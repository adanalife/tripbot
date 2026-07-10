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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/scoreboards"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/getsentry/sentry-go"
	"github.com/hako/durafmt"
	"gorm.io/gorm"
)

// leaderboardSize is how many rows the leaderboard commands show.
const leaderboardSize = 10

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
	n := len(a.helpMessages)
	// a.help() advances the index, so capture the displayed line's number first.
	pos := a.helpIndex + 1
	msg := fmt.Sprintf("%s (%d of %d)", a.help(), pos, n)
	a.Chat.Say(msg)
}

// commandsCmd lists a curated set of featured commands — filtered to the ones
// actually dispatchable on this App's platform, so a YouTube instance doesn't
// suggest commands that would silently no-op.
func (a *App) commandsCmd(_ context.Context, _ *users.User, _ []string) {
	featured := []string{
		"!location", "!guess", "!date", "!state",
		"!sunset", "!timewarp", "!miles", "!leaderboard", "!song",
	}
	avail := make([]string, 0, len(featured))
	for _, t := range featured {
		if _, ok := a.singleWordLookup[t]; ok {
			avail = append(avail, t)
		}
	}
	a.Chat.Say("You can try: " + strings.Join(avail, ", ") + ", and many other hidden commands!")
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
	if a.Sessions.CurrentMiles(ctx, *user) < 2.0 {
		msg += " I'm Tripbot, your adventure companion. Try using !commands to interact with me."
	}

	a.Chat.Say(msg)
	// update our record of last time it ran
	lastHelloTime = time.Now()
}

func (a *App) versionCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !version", "username", user.Username)

	// Cache the lookup — the file is baked at image build time, so its
	// contents don't change for the lifetime of the process.
	if currentVersion == "" {
		currentVersion = readBuildVersion(ctx)
	}

	a.Chat.Say("Current version is " + currentVersion)
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
	a.Chat.Say(msg)
}

func (a *App) followageCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !followage", "username", user.Username)

	// bare !followage = the caller; !followage @user looks up someone else
	username := user.Username
	other := len(params) > 0
	if other {
		username = helpers.StripAtSign(params[0])
	}

	followedAt, ok := a.Twitch.FollowedAt(username)
	if !ok {
		if other {
			a.Chat.Say(fmt.Sprintf("@%s isn't following the channel.", username))
		} else {
			a.Chat.Say("You're not following yet — hit that follow button!")
		}
		return
	}

	dur := durafmt.Parse(time.Since(followedAt)).LimitFirstN(2)
	if other {
		a.Chat.Say(fmt.Sprintf("@%s has been following for %s.", username, dur))
	} else {
		a.Chat.Say(fmt.Sprintf("@%s, you've been following for %s. Thanks!", username, dur))
	}
}

func (a *App) milesCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !miles", "username", user.Username)
	var username string
	var lifetimeMiles, monthlyMiles float32

	// check to see if an arg was provided
	if len(params) == 0 {
		username = user.Username
		lifetimeMiles = a.Sessions.CurrentMiles(ctx, *user)
		monthlyMiles = a.Sessions.CurrentMonthlyMiles(ctx, *user)
	} else {
		username = helpers.StripAtSign(params[0])
		u, err := a.Sessions.Find(ctx, username)

		// check to see if they are in our DB
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.Chat.Say("I don't know them, sorry!")
			return
		}
		if err != nil {
			slog.ErrorContext(ctx, "error finding user", "err", err, "username", username)
			a.Chat.Say("Couldn't look them up right now, try again in a bit")
			return
		}

		lifetimeMiles = a.Sessions.CurrentMiles(ctx, u)
		monthlyMiles = a.Sessions.CurrentMonthlyMiles(ctx, u)
	}

	// Floor the *displayed* monthly miles at 0.01 so a brand-new viewer never
	// sees "0.00mi", which reads as broken. This is display-only — the real
	// monthlyMiles value still drives the newcomer-hint logic below.
	displayMonthly := monthlyMiles
	if displayMonthly < 0.01 {
		displayMonthly = 0.01
	}

	msg := "@%s has %.2fmi this month"
	msg = fmt.Sprintf(msg, username, displayMonthly)

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

	a.Chat.Say(msg)
}

func (a *App) kilometresCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !kilometres", "username", user.Username)

	var username string
	var miles float32

	// check to see if an arg was provided (mirror milesCmd's other-user lookup)
	if len(params) == 0 {
		username = user.Username
		miles = a.Sessions.CurrentMiles(ctx, *user)
	} else {
		username = helpers.StripAtSign(params[0])
		u, err := a.Sessions.Find(ctx, username)

		// check to see if they are in our DB
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.Chat.Say("I don't know them, sorry!")
			return
		}
		if err != nil {
			slog.ErrorContext(ctx, "error finding user", "err", err, "username", username)
			a.Chat.Say("Couldn't look them up right now, try again in a bit")
			return
		}

		miles = a.Sessions.CurrentMiles(ctx, u)
	}

	km := miles * 1.609344
	msg := "@%s has %.2f kilometres."
	msg = fmt.Sprintf(msg, username, km)
	a.Chat.Say(msg)
}

func (a *App) sunsetCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !sunset", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}
	lat, lng, _ := vid.Location()
	a.Chat.Say(helpers.SunsetStr(vid.DateFilmed, lat, lng))
}

func (a *App) weatherCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !weather", "username", user.Username)
	if !a.Flags.Bool(ctx, weatherFlagKey, feature.EvalContext{
		Username: user.Username,
		Channel:  c.Conf.ChannelName,
		Env:      c.Conf.Environment,
	}) {
		slog.InfoContext(ctx, "!weather disabled by feature flag", "flag", weatherFlagKey, "username", user.Username)
		return
	}
	vid := a.Video.Current()
	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}
	lat, lng, _ := vid.Location()
	desc, err := a.Weather.Historical(ctx, vid.DateFilmed, lat, lng)
	if err != nil {
		slog.ErrorContext(ctx, "weather lookup failed", "err", err)
		a.Chat.Say("I couldn't fetch the weather for this spot, sorry!")
		return
	}
	a.Chat.Say(desc)
}

func (a *App) locationCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !location (or similar)", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		//TODO: write something like vid.FindClosest() that
		// chooses whether or not to use Next() vs Prev()
		vid = vid.Next(ctx)
	}
	// extract the coordinates
	lat, lng, err := vid.Location()
	// geocode the location
	address, _ := a.Geocoder.City(lat, lng)
	if err != nil {
		slog.ErrorContext(ctx, "geocoding error", "err", err)
	}
	// generate a google maps url — but only when we actually have coords.
	// A 0,0 fallback (the fallback video also had no usable GPS) would
	// otherwise emit a bogus maps.google.com/?q=0.00000,0.00000 link to chat.
	var msg string
	switch {
	case lat != 0 || lng != 0:
		msg = fmt.Sprintf("%s %s", address, helpers.GoogleMapsURL(lat, lng))
	case address != "":
		msg = address
	default:
		msg = "I couldn't pin down the exact spot, sorry!"
	}
	// record that they know the location now
	user.SetLastLocationTime()
	a.Chat.Say(msg)
}

func (a *App) monthlyMilesLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !leaderboard", "username", user.Username)

	// select users to show in leaderboard
	leaderboard := scoreboards.TopMilesRows(ctx, leaderboardSize)

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, scoreboards.CurrentMilesMonth()+" Miles", leaderboard)

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d miles this month: ", len(leaderboard))
	for i, leaderPair := range leaderboard {
		msg += fmt.Sprintf("%d. %s (%smi)", i+1, leaderPair[0], leaderPair[1])
		if i+1 != len(leaderboard) {
			msg += ", "
		}
	}
	a.Chat.Say(msg)
}

func (a *App) lifetimeMilesLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !totalleaderboard", "username", user.Username)

	// select users to show in leaderboard
	size := leaderboardSize
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
	a.Chat.Say(msg)
}

func (a *App) monthlyGuessLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !guessleaderboard", "username", user.Username)

	// select users to show in leaderboard (zero-scorers already filtered)
	intLeaderboard := scoreboards.TopGuessRows(ctx, leaderboardSize)

	// special message if no one has any correct guesses yet
	if len(intLeaderboard) == 0 {
		a.Chat.Say("No one is on that leaderboard yet!")
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
	a.Chat.Say(msg)
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
		a.Chat.Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtTime := realDate.Format("3:04pm MST")
		a.Chat.Say(fmt.Sprintf("This moment was %s", fmtTime))
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
		a.Chat.Say("I couldn't figure out current GPS coords, sorry!")
	} else {
		realDate := helpers.ActualDate(vid.DateFilmed, lat, lng)
		fmtDate := realDate.Format("Monday January 2, 2006")
		a.Chat.Say(fmt.Sprintf("This moment was %s", fmtDate))
	}
}

func (a *App) guessCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !guess", "username", user.Username)
	var msg string

	if len(params) == 0 {
		msg = "Try and guess what state we're in! For example: !guess CA"
		a.Chat.Say(msg)
		return
	}

	// don't let people guess if they already know the answer
	if !user.HasGuessCommandAvailable(ctx, lastTimewarpTime) {
		prettyDur := durafmt.ParseShort(user.GuessCooldownRemaining())
		msg = "I recently told you the answer! Try again in %s."
		msg = fmt.Sprintf(msg, prettyDur)
		a.Chat.Say(msg)
		return
	}

	// get the arg from the command
	guess := strings.Join(params, " ")

	// convert to short form if they used the full name
	// e.g. "Massachusetts" instead of "MA"
	if len(guess) == 2 {
		guess = helpers.StateAbbrevToState(guess)
	}

	// forgive close misspellings ("florisa" -> Florida); exact state names
	// are never touched and ambiguous typos stay as typed
	if corrected := fuzzyStateName(guess); corrected != "" {
		slog.InfoContext(ctx, "fuzzy-corrected state guess", "text", guess, "state", corrected)
		guess = corrected
	}

	vid := a.Video.Current()
	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}

	if strings.EqualFold(guess, vid.State) {
		msg = fmt.Sprintf("@%s got it! We're in %s", user.Username, vid.State)
		// increase their guess score
		user.AddToScore(ctx, guessScoreboard, 1.0)
		user.AddToScore(ctx, scoreboards.CurrentGuessScoreboard(), 1.0)
		// do a timewarp, crediting the guesser on the overlay
		a.timewarp(ctx, user.Username)
	} else {
		msg = "Try again! EarthDay"
	}
	a.Chat.Say(msg)
}

func (a *App) stateCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !state", "username", user.Username)
	vid := a.Video.Current()
	if vid.Flagged {
		a.Chat.Say("I couldn't figure out current GPS coords, using next closest...")
		vid = vid.Next(ctx)
	}
	msg := fmt.Sprintf("We're in %s", vid.State)
	// record that they know the location now
	user.SetLastLocationTime()
	a.Chat.Say(msg)
}

// TODO: maybe there could be a !cancel command or something
// reportReporter is the label a !report is attributed to in its downstream
// sinks (the Sentry error event and the Discord alert). YouTube viewers are
// anonymized because v1 punts YouTube identity entirely (see the youtubeCommands
// allowlist) — until real YouTube user support lands there is no persisted
// identity to stand behind, so the name is kept out of those durable/external
// sinks. Twitch reports keep the username. Note the transient 14-day Loki chat
// line still carries the name for every message; this only governs the report's
// longer-lived sinks.
func reportReporter(platform, username string) string {
	if platform == platformYouTube {
		return "a youtube viewer"
	}
	return username
}

func (a *App) reportCmd(ctx context.Context, user *users.User, params []string) {
	reporter := reportReporter(a.platform(), user.Username)
	slog.InfoContext(ctx, "ran !report", "username", reporter)
	message := strings.Join(params, " ")
	// Always log to slog (→ stderr + Sentry via the slog→Sentry handler)
	// as the durable audit trail.
	slog.ErrorContext(ctx, "!report", "err", fmt.Errorf("viewer report from %s: %s", reporter, message))
	// Fire-and-forget to Discord for real-time notification. Skipped
	// silently when DISCORD_ALERTS_WEBHOOK is unset (e.g. local dev) —
	// the slog/Sentry path still fires so nothing is lost.
	if webhook := c.Conf.DiscordAlertsWebhook; webhook != "" {
		if isDiscordWebhookURL(webhook) {
			go postReportToDiscord(webhook, reporter, message)
		} else {
			// A misconfigured secret (e.g. the SM placeholder string) would
			// otherwise log a "unsupported protocol scheme" ERROR on every
			// !report. Warn once per process and fall through to slog/Sentry.
			reportWebhookWarnOnce.Do(func() {
				slog.WarnContext(ctx, "DISCORD_ALERTS_WEBHOOK is not a Discord webhook URL; skipping Discord report")
			})
		}
	}
	a.Chat.Say("Thank you, I will look into this ASAP!")
}

// reportWebhookWarnOnce bounds the misconfigured-webhook warning to one line
// per process instead of one per !report invocation.
var reportWebhookWarnOnce sync.Once

// isDiscordWebhookURL reports whether s looks like a Discord webhook endpoint,
// guarding postReportToDiscord against placeholder/garbage secret values.
func isDiscordWebhookURL(s string) bool {
	return strings.HasPrefix(s, "https://discord.com/api/webhooks/")
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
	bonus := a.Sessions.BonusMiles(*user)
	msg := fmt.Sprintf("%s has earned %.4f bonus miles this session", user.Username, bonus)
	a.Chat.Say(msg)
}

func (a *App) secretInfoCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !secretinfo", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		return
	}
	vid := a.Video.Current()
	msg := fmt.Sprintf("currently playing: %s, playtime: %s", vid, a.Video.CurrentProgress())
	lat, lng, err := vid.Location()
	if err != nil {
		msg = fmt.Sprintf("%s, err: %s", msg, err)
	} else {
		msg = fmt.Sprintf("%s, lat: %f, lng: %f", msg, lat, lng)
	}
	slog.InfoContext(ctx, "secretinfo output", "text", msg)
	a.Chat.Say(msg)
}

// giveMilesCmd is the admin !givemiles <user> <amount> command: it applies a
// manual miles correction (amount may be negative) and logs a correction event
// so the rollup folds it into user_rollups.extra_miles. Admin-only for now
// (broadcaster); widen the gate to mods once a mod-status source exists.
func (a *App) giveMilesCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !givemiles", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		return
	}
	if len(params) < 2 {
		a.Chat.Say("usage: !givemiles <user> <amount>")
		return
	}
	target := helpers.StripAtSign(params[0])
	delta, err := strconv.ParseFloat(params[1], 32)
	if err != nil {
		a.Chat.Say("that amount isn't a number I understand")
		return
	}
	if _, err := a.Sessions.Find(ctx, target); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			a.Chat.Say("I don't know them, sorry!")
		} else {
			slog.ErrorContext(ctx, "error finding user", "err", err, "username", target)
			a.Chat.Say("Couldn't look them up right now, try again in a bit")
		}
		return
	}
	newTotal := a.Sessions.CorrectMiles(ctx, target, float32(delta))
	if err := events.Correction(ctx, target, delta); err != nil {
		slog.ErrorContext(ctx, "error creating correction event", "err", err)
	}
	a.Chat.Say(fmt.Sprintf("@%s now has %.2fmi", target, newTotal))
}

// refreshOverlaysCmd hard-reloads every OBS browser source (the onscreen
// corners, the next-frame cover, etc.) by respawning each source's CEF render
// process. Admin-only. This is the manual recovery for a crashed/frozen overlay
// — the hourly soft refresh can't revive a crashed CEF webpage.
func (a *App) refreshOverlaysCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !refreshoverlays", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		return
	}
	n, err := a.OBS.RefreshBrowserSources(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "overlay refresh failed", "err", err)
		a.Chat.Say("Couldn't refresh the overlays right now, try again in a bit")
		return
	}
	a.Chat.Say(fmt.Sprintf("Refreshed %d overlay(s).", n))
}

func (a *App) shutdownCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !shutdown", "username", user.Username)
	if !c.UserIsAdmin(user.Username) {
		a.Chat.Say("Nice try bucko")
		return
	}
	a.Chat.Say("Shutting down...")
	slog.InfoContext(ctx, "shutdown: currently playing", "video", a.Video.Current())
	if err := a.Cron.Stop(); err != nil {
		slog.ErrorContext(ctx, "cron shutdown failed during !shutdown", "err", err)
	}
	a.Sessions.Shutdown(ctx)
	err := database.Connection().Close()
	if err != nil {
		slog.ErrorContext(ctx, "DB close failed during shutdown", "err", err)
	}
	sentry.Flush(time.Second * 5)
	os.Exit(0)
}

// TODO: this will always be lower case, find out why
// middleCmd sets the text at the bottom-middle of the stream
func (a *App) middleCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !middle", "username", user.Username)
	// don't let strangers run this
	if !c.UserIsAdmin(user.Username) {
		return
	}

	// don't do anything if empty
	if len(params) == 0 {
		a.Chat.Say("What do you want to say?")
		return
	}

	// if the arg was "hide", hide the text from view
	if len(params) == 1 && strings.ToLower(params[0]) == "hide" {
		a.Chat.Say("Got it! Hiding the message.")
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
