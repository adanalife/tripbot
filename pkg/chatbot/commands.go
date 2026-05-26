package chatbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/scoreboards"

	"github.com/adanalife/tripbot/pkg/background"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/helpers"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/getsentry/sentry-go"
	"github.com/hako/durafmt"
	"gorm.io/gorm"
)

// lastHelloTime is used to rate-limit the hello command
var lastHelloTime time.Time = time.Now()

var currentVersion string

// versionFilePath is the build-time-baked version file path. Released
// container images write the tag here (see infra/docker/*/Dockerfile);
// outside a container the file won't exist and versionCmd falls back to
// "dev". Overridable in tests.
var versionFilePath = "/etc/tripbot/version"

// this is the scoreboard name used for counting correct guesses
const guessScoreboard = "guess_state_total"

//TODO: incorrect guess scoreboard?

func (a *App) helpCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !help", "username", user.Username)
	msg := fmt.Sprintf("%s (%d of %d)", help(), helpIndex+1, len(c.HelpMessages))
	a.IRC.Say(msg)
}

func (a *App) helloCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "user said hello", "username", user.Username)

	// check if it was just a one-word hello
	if len(params) > 0 {
		return
	}

	// check if we said hi too recently
	if time.Now().Sub(lastHelloTime) < 20*time.Second {
		return
	}

	// say a random greeting back, with random punctuation
	greetings := []string{"Hello", "Hey", "Hi"}
	punctuation := []string{"!", ".", ".", "."}
	msg := greetings[rand.Intn(len(greetings))]
	msg += punctuation[rand.Intn(len(punctuation))]

	// give a little help message if the user is new
	if user.CurrentMiles(ctx) < 2.0 {
		msg += " I'm Tripbot, your adventure companion. Try using !commands to interact with me."
	}

	a.IRC.Say(msg)
	// update our record of last time it ran
	lastHelloTime = time.Now()
}

func (a *App) flagCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !flag", "username", user.Username)
	a.Onscreens.ShowFlag(ctx, 10 * time.Second)
}

func (a *App) versionCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !version", "username", user.Username)

	// Cache the lookup — the file is baked at image build time, so its
	// contents don't change for the lifetime of the process.
	if currentVersion == "" {
		currentVersion = readBuildVersion(ctx)
	}

	a.IRC.Say("Current version is " + currentVersion)
}

// readBuildVersion reads the build-time-baked tag from versionFilePath
// (written by the release Dockerfiles). When the file is missing or
// empty — i.e. local `go run` outside a container — returns "dev" to
// match the ldflag default used by the /version HTTP handler.
func readBuildVersion(ctx context.Context) string {
	raw, err := os.ReadFile(versionFilePath)
	if err != nil {
		slog.DebugContext(ctx, "version file not present, falling back to dev", "err", err, "file", versionFilePath)
		return "dev"
	}
	v := strings.TrimSpace(string(raw))
	if v == "" {
		return "dev"
	}
	return v
}

func (a *App) uptimeCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !uptime", "username", user.Username)
	dur := time.Now().Sub(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	a.IRC.Say(msg)
}

func (a *App) followageCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !followage", "username", user.Username)

	// bare !followage = the caller; !followage @user looks up someone else
	username := user.Username
	other := len(params) > 0
	if other {
		username = helpers.StripAtSign(params[0])
	}

	followedAt, ok := mytwitch.FollowedAt(username)
	if !ok {
		if other {
			a.IRC.Say(fmt.Sprintf("@%s isn't following the channel.", username))
		} else {
			a.IRC.Say("You're not following yet — hit that follow button!")
		}
		return
	}

	dur := durafmt.Parse(time.Since(followedAt)).LimitFirstN(2)
	if other {
		a.IRC.Say(fmt.Sprintf("@%s has been following for %s.", username, dur))
	} else {
		a.IRC.Say(fmt.Sprintf("@%s, you've been following for %s. Thanks!", username, dur))
	}
}

func (a *App) milesCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !miles", "username", user.Username)
	var username string
	var lifetimeMiles, monthlyMiles float32

	// check to see if an arg was provided
	if len(params) == 0 {
		username = user.Username
		lifetimeMiles = user.CurrentMiles(ctx)
		monthlyMiles = user.CurrentMonthlyMiles(ctx)
	} else {
		username = helpers.StripAtSign(params[0])
		u := a.Sessions.Find(ctx, username)

		// check to see if they are in our DB
		if u.ID == 0 {
			a.IRC.Say("I don't know them, sorry!")
			return
		}

		lifetimeMiles = u.CurrentMiles(ctx)
		monthlyMiles = u.CurrentMonthlyMiles(ctx)
	}

	msg := "@%s has %.2fmi this month"
	msg = fmt.Sprintf(msg, username, monthlyMiles)

	// add total miles if they have been around for more than one month
	if lifetimeMiles > monthlyMiles {
		msg += " (%vmi total)."
		msg = fmt.Sprintf(msg, math.Round(float64(lifetimeMiles)))
	} else {
		msg += "."

		// add helpful messages for new folks
		if len(params) == 0 {
			if monthlyMiles < 0.2 {
				msg += " You'll earn more miles the longer you watch the stream."
			}
			if monthlyMiles == 0.0 {
				msg += " (Sometimes it takes a bit for me to notice you. You should be good now!)"
			}
		}
	}

	a.IRC.Say(msg)
}

func (a *App) kilometresCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !kilometres", "username", user.Username)
	km := user.CurrentMiles(ctx) * 1.609344
	msg := "@%s has %.2f kilometres."
	msg = fmt.Sprintf(msg, user.Username, km)
	a.IRC.Say(msg)
}

func (a *App) sunsetCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !sunset", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.IRC.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}
	lat, lng, _ := vid.Location()
	a.IRC.Say(helpers.SunsetStr(vid.DateFilmed, lat, lng))
}

func (a *App) locationCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !location (or similar)", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.IRC.Say("I couldn't figure out current GPS coords, using next closest...")
		//TODO: write something like vid.FindClosest() that
		// chooses whether or not to use Next() vs Prev()
		vid = vid.Next(ctx)
	}
	// extract the coordinates
	lat, lng, err := vid.Location()
	// geocode the location
	address, _ := helpers.CityFromCoords(lat, lng)
	if err != nil {
		slog.ErrorContext(ctx, "geocoding error", "err", err)
	}
	// generate a google maps url
	url := helpers.GoogleMapsURL(lat, lng)
	msg := fmt.Sprintf("%s %s", address, url)
	// record that they know the location now
	user.SetLastLocationTime()
	a.IRC.Say(msg)
}

func (a *App) monthlyMilesLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !leaderboard", "username", user.Username)

	// select users to show in leaderboard
	size := 10
	leaderboard := scoreboards.TopUsers(ctx, scoreboards.CurrentMilesScoreboard(), size)
	if size > len(leaderboard) {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, "Monthly Miles", leaderboard)

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d miles this month: ", size)
	for i, leaderPair := range leaderboard {
		msg += fmt.Sprintf("%d. %s (%smi)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != len(leaderboard) {
			msg += ", "
		}
	}
	a.IRC.Say(msg)
}

func (a *App) lifetimeMilesLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !totalleaderboard", "username", user.Username)

	// select users to show in leaderboard
	size := 10
	lifetime := a.Sessions.LifetimeLeaderboard()
	if size > len(lifetime) {
		size = len(lifetime)
	}
	leaderboard := lifetime[:size]

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, "Total Miles", leaderboard)

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d lifetime miles: ", size)
	for i, leaderPair := range leaderboard {
		msg += fmt.Sprintf("%d. %s (%smi)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != len(leaderboard) {
			msg += ", "
		}
	}
	a.IRC.Say(msg)
}

func (a *App) monthlyGuessLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !guessleaderboard", "username", user.Username)

	// select users to show in leaderboard
	size := 10
	leaderboard := scoreboards.TopUsers(ctx, scoreboards.CurrentGuessScoreboard(), size)

	// truncate the leaderboard if necessary
	if size > len(leaderboard) {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	// Filter zero-scorers (AddToScoreByName uses FirstOrCreate, so every
	// user who's ever guessed has a row — many at 0 early in the month).
	var intLeaderboard [][]string
	for _, leaderPair := range leaderboard {
		// guesses are ints not floats, so remove the decimal place
		intVersion := strings.Split(leaderPair[1], ".")[0]
		if intVersion == "0" || intVersion == "" {
			continue
		}
		intLeaderboard = append(intLeaderboard, []string{leaderPair[0], intVersion})
	}

	// special message if no one has any correct guesses yet
	if len(intLeaderboard) == 0 {
		a.IRC.Say("No one is on that leaderboard yet!")
		return
	}

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, "Correct Guesses This Month", intLeaderboard)

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d correct guesses this month: ", len(intLeaderboard))
	for i, leaderPair := range intLeaderboard {
		msg += fmt.Sprintf("%d. %s (%s)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != len(intLeaderboard) {
			msg += ", "
		}
	}
	a.IRC.Say(msg)
}

func (a *App) timeCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !time", "username", user.Username)
	var err error
	var lat, lng float64
	vid := a.Video.Current()
	if vid.Flagged {
		lat, lng, err = vid.Next(ctx).Location()
	} else {
		lat, lng, err = vid.Location()
	}
	if err != nil {
		a.IRC.Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtTime := realDate.Format("3:04pm MST")
		a.IRC.Say(fmt.Sprintf("This moment was %s", fmtTime))
	}
}

func (a *App) dateCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !date", "username", user.Username)
	var err error
	var lat, lng float64
	vid := a.Video.Current()
	if vid.Flagged {
		lat, lng, err = vid.Next(ctx).Location()
	} else {
		lat, lng, err = vid.Location()
	}
	if err != nil {
		a.IRC.Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtDate := realDate.Format("Monday January 2, 2006")
		a.IRC.Say(fmt.Sprintf("This moment was %s", fmtDate))
	}
}

//TODO: refactor to use golang '...' syntax
func (a *App) guessCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !guess", "username", user.Username)
	var msg string

	if len(params) == 0 {
		msg = "Try and guess what state we're in! For example: !guess CA"
		a.IRC.Say(msg)
		return
	}

	// don't let people guess if they already know the answer
	if !user.HasGuessCommandAvailable(ctx, lastTimewarpTime) {
		prettyDur := durafmt.ParseShort(user.GuessCooldownRemaining())
		msg = "I recently told you the answer! Try again in %s."
		msg = fmt.Sprintf(msg, prettyDur)
		a.IRC.Say(msg)
		return
	}

	// get the arg from the command
	guess := strings.Join(params, " ")

	// convert to short form if they used the full name
	// e.g. "Massachusetts" instead of "MA"
	if len(guess) == 2 {
		guess = helpers.StateAbbrevToState(guess)
	}

	vid := a.Video.Current()
	if vid.Flagged {
		a.IRC.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}

	if strings.ToLower(guess) == strings.ToLower(vid.State) {
		msg = fmt.Sprintf("@%s got it! We're in %s", user.Username, vid.State)
		// show the flag for the state
		a.Onscreens.ShowFlag(ctx, 10 * time.Second)
		// increase their guess score
		user.AddToScore(ctx, guessScoreboard, 1.0)
		user.AddToScore(ctx, scoreboards.CurrentGuessScoreboard(), 1.0)
		// do a timewarp
		a.timewarp(ctx)
	} else {
		msg = "Try again! EarthDay"
	}
	a.IRC.Say(msg)
}

func (a *App) stateCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !state", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.IRC.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}
	msg := fmt.Sprintf("We're in %s", vid.State)
	// show the flag for the state
	a.Onscreens.ShowFlag(ctx, 10 * time.Second)
	// record that they know the location now
	user.SetLastLocationTime()
	a.IRC.Say(msg)
}

//TODO: maybe there could be a !cancel command or something
//TODO: use fancy golang ... syntax?
func (a *App) reportCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !report", "username", user.Username)
	message := strings.Join(params, " ")
	// Always log to slog (→ stderr + Sentry via the slog→Sentry handler)
	// as the durable audit trail.
	slog.ErrorContext(ctx, "!report", "err", fmt.Errorf("viewer report from %s: %s", user.Username, message))
	// Fire-and-forget to Discord for real-time notification. Skipped
	// silently when DISCORD_ALERTS_WEBHOOK is unset (e.g. local dev) —
	// the slog/Sentry path still fires so nothing is lost.
	if webhook := c.Conf.DiscordAlertsWebhook; webhook != "" {
		go postReportToDiscord(webhook, user.Username, message)
	}
	a.IRC.Say("Thank you, I will look into this ASAP!")
}

// postReportToDiscord POSTs a viewer report to a Discord webhook.
// Runs in a goroutine off the chat-handler path so chat doesn't block on
// Discord latency; uses a fresh ctx with a 5s timeout because the
// caller's ctx is already detached.
func postReportToDiscord(webhookURL, username, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload, err := json.Marshal(map[string]string{
		"content": fmt.Sprintf("**!report** from @%s: %s", username, message),
	})
	if err != nil {
		slog.ErrorContext(ctx, "discord webhook payload marshal", "err", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
	if err != nil {
		slog.ErrorContext(ctx, "discord webhook request build", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.ErrorContext(ctx, "discord webhook POST", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.ErrorContext(ctx, "discord webhook non-2xx", "status", resp.StatusCode)
	}
}

func (a *App) bonusMilesCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !bonusmiles", "username", user.Username)
	bonus := user.BonusMiles()
	msg := fmt.Sprintf("%s has earned %.4f bonus miles this session", user.Username, bonus)
	a.IRC.Say(msg)
}

func (a *App) secretInfoCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !secretinfo", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		return
	}
	vid := a.Video.Current()
	msg := fmt.Sprintf("currently playing: %s, playtime: %s", vid, video.CurrentProgress())
	lat, lng, err := vid.Location()
	if err != nil {
		msg = fmt.Sprintf("%s, err: %s", msg, err)
	} else {
		msg = fmt.Sprintf("%s, lat: %f, lng: %f", msg, lat, lng)
	}
	slog.InfoContext(ctx, "secretinfo output", "text", msg)
	a.IRC.Say(msg)
}

func (a *App) shutdownCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !shutdown", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		a.IRC.Say("Nice try bucko")
		return
	}
	a.IRC.Say("Shutting down...")
	slog.InfoContext(ctx, "shutdown: currently playing", "video", a.Video.Current())
	background.StopCron()
	a.Sessions.Shutdown(ctx)
	err := database.Connection().Close()
	if err != nil {
		slog.ErrorContext(ctx, "DB close failed during shutdown", "err", err)
	}
	sentry.Flush(time.Second * 5)
	os.Exit(0)
}

//TODO: this will always be lower case, find out why
// middleCmd sets the text at the bottom-middle of the stream
func (a *App) middleCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !middle", "username", user.Username)
	// don't let strangers run this
	if !c.UserIsAdmin(user.Username) {
		return
	}

	// don't do anything if empty
	if len(params) == 0 {
		a.IRC.Say("What do you want to say?")
		return
	}

	// if the arg was "hide", hide the text from view
	if len(params) == 1 && strings.ToLower(params[0]) == "hide" {
		a.IRC.Say("Got it! Hiding the message.")
		a.Onscreens.HideMiddleText(ctx)
		return
	}

	// use the params as the text
	text := strings.Join(params, " ")

	slog.InfoContext(ctx, "setting middle text", "text", text)

	a.Onscreens.ShowMiddleText(ctx, text)
}

func (a *App) makeBotCmd(ctx context.Context, user *users.User, params []string) {
	a.setBotFlag(ctx, user, params, true, "!makebot")
}

func (a *App) unBotCmd(ctx context.Context, user *users.User, params []string) {
	a.setBotFlag(ctx, user, params, false, "!unbot")
}

// setBotFlag is the shared body of !makebot and !unbot. Admin-only, silent
// in chat, logs the outcome for ops visibility.
func (a *App) setBotFlag(ctx context.Context, user *users.User, params []string, isBot bool, trigger string) {
	slog.InfoContext(ctx, "ran "+trigger, "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		return
	}
	if len(params) == 0 {
		slog.WarnContext(ctx, trigger+" called with no target", "username", user.Username)
		return
	}
	target := strings.ToLower(strings.TrimPrefix(params[0], "@"))
	if err := a.Sessions.SetBot(ctx, target, isBot); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.WarnContext(ctx, trigger+": target user not found", "target", target)
			return
		}
		slog.ErrorContext(ctx, trigger+" failed", "target", target, "err", err)
		return
	}
	slog.InfoContext(ctx, trigger+": flipped is_bot", "target", target, "is_bot", isBot)
}
