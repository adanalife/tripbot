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

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/getsentry/sentry-go"
	"github.com/hako/durafmt"
	"gorm.io/gorm"
)

// leaderboardSize is how many rows the leaderboard commands list in chat.
const leaderboardSize = 10

// onscreenRows is how many of those rows go on the overlay. Chat and the
// overlay are read differently — a chat message scrolls past and can be
// scanned, while the overlay sits on the broadcast — so the onscreen copy is
// the short one.
const onscreenRows = 5

// guessrMonthlyRows is the one board that keeps a full ten onscreen: it is a
// month's running total, so the names below fifth place are still worth
// reading. Bounded by maxLeaderboardRows in pkg/onscreens-server, which is what
// actually fits on the overlay.
const guessrMonthlyRows = 10

// overlayRows trims a board to what goes onscreen, leaving the caller's slice
// intact for the chat message it also builds.
func overlayRows(rows [][]string, size int) [][]string {
	return rows[:min(len(rows), size)]
}

// rankedList renders a board as the numbered run of names a leaderboard
// command says in chat: "1. alice (12.0mi), 1. bob (12.0mi), 3. carol (9.0mi)".
// Tied rows share the better place, so two viewers level on miles are both
// first and nobody is arbitrarily ahead. unit is appended to each score.
func rankedList(rows [][]string, unit string) string {
	ranks := scoreboards.Ranks(rows)
	parts := make([]string, 0, len(rows))
	for i, row := range rows {
		parts = append(parts, fmt.Sprintf("%d. %s (%s%s)", ranks[i], row[0], row[1], unit))
	}
	return strings.Join(parts, ", ")
}

// targetUsername turns a chat-mention param ("@DanaMerrick") into the canonical
// username used everywhere else: @-less and lowercase. Params reach handlers
// with their original casing, and usernames are stored lowercase, so every
// command that looks up another viewer routes its param through here.
func targetUsername(param string) string {
	return strings.ToLower(helpers.StripAtSign(param))
}

// lastHelloTime is used to rate-limit the hello command
var lastHelloTime time.Time = time.Now()

var currentVersion string

// versionFilePath is the build-time-baked version file path. Released
// container images write the tag here (see infra/docker/*/Dockerfile);
// outside a container the file won't exist and versionCmd falls back to
// "dev". Overridable in tests.
var versionFilePath = "/etc/tripbot/version"

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
		"!sunset", "!timewarp", "!miles", "!leaderboard", "!guessr", "!song",
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
	if time.Since(lastHelloTime) < 20*time.Second {
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
	dur := time.Since(Uptime)
	msg := fmt.Sprintf("I have been running for %s", durafmt.Parse(dur))
	a.Chat.Say(msg)
}

func (a *App) followageCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !followage", "username", user.Username)

	// bare !followage = the caller; !followage @user looks up someone else
	username := user.Username
	other := len(params) > 0
	if other {
		username = targetUsername(params[0])
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

// formatLifetimeMiles renders the lifetime total for a !miles reply. Whole
// miles read best, so the total is rounded — except when the rounded total
// would be smaller than the two-decimal monthly figure sitting next to it in
// the same sentence, which looks like a bug to viewers. In that case the total
// keeps two decimals to match the month, and never renders below it.
func formatLifetimeMiles(lifetimeMiles, displayMonthly float32) string {
	rounded := math.Round(float64(lifetimeMiles))

	// what the monthly figure actually renders as, at two decimals
	monthlyShown := math.Round(float64(displayMonthly)*100) / 100
	if rounded >= monthlyShown {
		return fmt.Sprintf("%v", rounded)
	}

	total := float64(lifetimeMiles)
	if total < monthlyShown {
		total = monthlyShown
	}
	return fmt.Sprintf("%.2f", total)
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
		username = targetUsername(params[0])
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
		msg += " (%smi total)."
		msg = fmt.Sprintf(msg, formatLifetimeMiles(lifetimeMiles, displayMonthly))
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
		username = targetUsername(params[0])
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
	s, ok := a.currentSpot(ctx)
	if !ok {
		return
	}
	a.Chat.Say(helpers.SunsetStr(s.vid.DateFilmed, s.at.Lat, s.at.Lng))
}

func (a *App) weatherCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !weather", "username", user.Username)
	s, ok := a.currentSpot(ctx)
	if !ok {
		return
	}
	desc, err := a.Weather.Historical(ctx, s.vid.DateFilmed, s.at.Lat, s.at.Lng)
	if err != nil {
		slog.ErrorContext(ctx, "weather lookup failed", "err", err)
		a.Chat.Say("I couldn't fetch the weather for this spot, sorry!")
		return
	}
	a.Chat.Say(desc)
}

func (a *App) locationCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !location (or similar)", "username", user.Username)
	s, ok := a.currentSpot(ctx)
	if !ok {
		return
	}
	address := a.place(ctx, s)
	// generate a google maps url — but only when we actually have coords.
	// A 0,0 fallback (the fallback video also had no usable GPS) would
	// otherwise emit a bogus maps.google.com/?q=0.00000,0.00000 link to chat.
	var msg string
	switch {
	case s.at.Lat != 0 || s.at.Lng != 0:
		msg = fmt.Sprintf("%s %s", address, helpers.GoogleMapsURL(s.at.Lat, s.at.Lng))
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
	leaderboard := a.Scoreboards.TopMiles(ctx, leaderboardSize)

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, a.Scoreboards.MilesMonth()+" Miles", overlayRows(leaderboard, onscreenRows))

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d miles this month: ", len(leaderboard)) + rankedList(leaderboard, "mi")
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
	a.Onscreens.ShowLeaderboard(ctx, "Total Miles", overlayRows(leaderboard, onscreenRows))

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d lifetime miles: ", size) + rankedList(leaderboard, "mi")
	a.Chat.Say(msg)
}

func (a *App) monthlyGuessLeaderboardCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !guessleaderboard", "username", user.Username)

	// select users to show in leaderboard (zero-scorers already filtered)
	intLeaderboard := a.Scoreboards.TopGuesses(ctx, leaderboardSize)

	// special message if no one has any correct guesses yet
	if len(intLeaderboard) == 0 {
		a.Chat.Say("No one is on that leaderboard yet!")
		return
	}

	// display leaderboard on screen
	a.Onscreens.ShowLeaderboard(ctx, "Correct Guesses This Month", overlayRows(intLeaderboard, onscreenRows))

	// build a message to send to chat
	msg := fmt.Sprintf("Top %d correct guesses this month: ", len(intLeaderboard)) + rankedList(intLeaderboard, "")
	a.Chat.Say(msg)
}

func (a *App) timeCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !time", "username", user.Username)
	var err error
	var lat, lng float64
	vid := a.Video.Current()
	if vid.Flagged {
		var next video.Video
		next, err = vid.NextUnflagged(ctx)
		if err == nil {
			lat, lng, err = next.Location()
		}
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
		var next video.Video
		next, err = vid.NextUnflagged(ctx)
		if err == nil {
			lat, lng, err = next.Location()
		}
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
		// The cooldown is per-command state rather than a dispatcher gate, so
		// this refusal has to be recorded from inside the handler. It's the one
		// refusal a rollup can compare against a *successful* guess by the same
		// viewer, which is what makes the guess history readable.
		a.recordRefusal(ctx, events.CommandRefusal{
			Username: user.Username,
			Command:  "!guess",
			Args:     strings.Join(params, " "),
			Reason:   events.RefusedCooldown,
		})
		return
	}

	// get the arg from the command
	guess := strings.Join(params, " ")

	// expand a two-letter abbreviation to the full name the DB stores
	// ("MA" -> "Massachusetts"). A pair that isn't a state code is left as
	// typed; blanking it would compare "" against the video's state.
	if len(guess) == 2 {
		if full := helpers.StateAbbrevToState(guess); full != "" {
			guess = full
		}
	}

	// forgive close misspellings ("florisa" -> Florida); exact state names
	// are never touched and ambiguous typos stay as typed
	if corrected := fuzzyStateName(guess); corrected != "" {
		slog.InfoContext(ctx, "fuzzy-corrected state guess", "text", guess, "state", corrected)
		guess = corrected
	}

	s, ok := a.currentSpot(ctx)
	if !ok {
		return
	}
	state := a.state(ctx, s)

	// A video whose geocode came back empty (no Maps key, or ZERO_RESULTS)
	// isn't flagged, so it reaches here with no state to guess at. There's no
	// right answer to credit, and matching against "" would credit anyone
	// whose guess normalized to empty.
	if state == "" {
		a.Chat.Say("I don't know what state this is, sorry!")
		return
	}

	if strings.EqualFold(guess, state) {
		msg = fmt.Sprintf("@%s got it! We're in %s", user.Username, state)
		// increase their guess score
		a.Scoreboards.CreditGuess(ctx, user)
		// do a timewarp, crediting the guesser on the overlay
		a.timewarp(ctx, user.Username)
	} else {
		msg = "Try again! EarthDay"
	}
	a.Chat.Say(msg)
}

func (a *App) stateCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !state", "username", user.Username)
	s, ok := a.currentSpot(ctx)
	if !ok {
		return
	}
	msg := fmt.Sprintf("We're in %s", a.state(ctx, s))
	// record that they know the location now
	user.SetLastLocationTime()
	a.Chat.Say(msg)
}

// anonymizedReportPlatforms maps a platform to the anonymized label used for
// its viewers in a !report's durable/external sinks (the Sentry error event and
// the Discord alert). Membership is the capability the reporter label keys off,
// not a hardcoded name check. Only YouTube anonymizes for now — its privacy
// policy was strict about not recording a viewer's identity in such sinks. A
// platform absent from the map — Twitch, the other platforms, and any
// unrecognized one — keeps the real username: name-by-default is the intended
// behavior, and anonymization is the per-platform exception a privacy policy
// imposes (add a platform here if its policy comes to require it).
var anonymizedReportPlatforms = map[string]string{
	platformYouTube: "a youtube viewer",
}

// reportReporter is the label a !report is attributed to in its downstream
// sinks. It defaults to the viewer's real username; a platform whose privacy
// policy forbids recording viewer identities (see anonymizedReportPlatforms)
// gets an anonymized label instead. Note the transient 14-day Loki chat line
// still carries the name for every message; this only governs the report's
// longer-lived sinks.
func reportReporter(platform, username string) string {
	if label, ok := anonymizedReportPlatforms[platform]; ok {
		return label
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
	if webhook := a.Cfg.DiscordAlertsWebhook; webhook != "" {
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

// achievementsCmd lists the caller's earned achievements, newest first.
func (a *App) achievementsCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !achievements", "username", user.Username)
	var rows []struct{ Title string }
	if err := database.GormDB().WithContext(ctx).
		Raw(`SELECT title FROM achievements WHERE platform = ? AND username = ? ORDER BY earned_at DESC`,
			a.platform(), user.Username).Scan(&rows).Error; err != nil {
		slog.ErrorContext(ctx, "achievements lookup failed", "err", err, "username", user.Username)
		return
	}
	if len(rows) == 0 {
		a.Chat.Say(fmt.Sprintf("@%s has no achievements yet — keep watching!", user.Username))
		return
	}
	shown := rows
	extra := ""
	if len(rows) > 5 {
		shown = rows[:5]
		extra = fmt.Sprintf(", +%d more", len(rows)-5)
	}
	titles := make([]string, len(shown))
	for i, r := range shown {
		titles[i] = r.Title
	}
	a.Chat.Say(fmt.Sprintf("@%s has %d 🏆: %s%s", user.Username, len(rows), strings.Join(titles, ", "), extra))
}

func (a *App) bonusMilesCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !bonusmiles", "username", user.Username)
	bonus := a.Sessions.BonusMiles(*user)
	msg := fmt.Sprintf("%s has earned %.4f bonus miles this session", user.Username, bonus)
	a.Chat.Say(msg)
}

func (a *App) secretInfoCmd(ctx context.Context, user *users.User, _ []string) {
	slog.InfoContext(ctx, "ran !secretinfo", "username", user.Username)
	if !a.Cfg.UserIsAdmin(user.Username) {
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
	if !a.Cfg.UserIsAdmin(user.Username) {
		return
	}
	if len(params) < 2 {
		a.Chat.Say("usage: !givemiles <user> <amount>")
		return
	}
	target := targetUsername(params[0])
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
	if err := a.Events.Correction(ctx, target, delta); err != nil {
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
	if !a.Cfg.UserIsAdmin(user.Username) {
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
	if !a.Cfg.UserIsAdmin(user.Username) {
		a.Chat.Say("Nice try bucko")
		return
	}
	a.Chat.Say("Shutting down...")
	slog.InfoContext(ctx, "shutdown: currently playing", "video", a.Video.Current())
	if err := a.Cron.Shutdown(); err != nil {
		slog.ErrorContext(ctx, "cron shutdown failed during !shutdown", "err", err)
	}
	a.Sessions.Shutdown(ctx)
	err := database.Close()
	if err != nil {
		slog.ErrorContext(ctx, "DB close failed during shutdown", "err", err)
	}
	sentry.Flush(time.Second * 5)
	os.Exit(0)
}

// middleCmd sets the text at the bottom-middle of the stream
func (a *App) middleCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !middle", "username", user.Username)
	// don't let strangers run this
	if !a.Cfg.UserIsAdmin(user.Username) {
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
	if !a.Cfg.UserIsAdmin(user.Username) {
		return
	}
	if len(params) == 0 {
		slog.WarnContext(ctx, trigger+" called with no target", "username", user.Username)
		return
	}
	target := targetUsername(params[0])
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
