package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	mylog "github.com/adanalife/tripbot/pkg/chatbot/log"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/geo"
	"github.com/adanalife/tripbot/pkg/natsclient"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
	"github.com/gempir/go-twitch-irc/v4"
	"gorm.io/gorm"
)

var googleMapsAPIKey string
var client *twitch.Client
var Uptime time.Time

// App holds injectable dependencies for the chatbot. cmd/tripbot constructs the
// live one with New(); tests instantiate it directly with fakes.
type App struct {
	// DB is the GORM handle used by commands that need to read or write the
	// database. nil in tests that don't exercise the DB; otherwise either the
	// real database.GormDB() or a sqlmock-backed gorm.DB.
	DB *gorm.DB
	// Onscreens drives the OBS browser-source overlays for chat-triggered
	// effects (leaderboards, flags, middle-text). Tests inject a no-op fake.
	Onscreens Onscreens
	// VLC drives playback operations (timewarp, jump, skip, back). Tests
	// inject a no-op fake; production uses the realVLC adapter.
	VLC VLC
	// Video reads / refreshes the currently-playing dashcam video. Tests
	// inject a no-op fake; production uses the realVideo adapter.
	Video Video
	// IRC sends chat output (Say, Whisper). Tests inject a recordingIRC
	// to assert on chat messages; production uses the realIRC adapter
	// which delegates to the package-level twitch client.
	IRC IRC
	// Sessions wraps the user-lookup / lifetime-leaderboard / shutdown
	// surface of pkg/users for command-time queries. Tests inject a
	// recordingSessions to assert lookups and stage results; production
	// uses the realSessions adapter built by NewSessionsAdapter.
	Sessions Sessions
	// UserSessions is the concrete process-wide session state the inbound IRC
	// handlers (HandleMessage / Join / Part) and dispatch's access check use
	// directly — the login/logout lifecycle + follower/subscriber + login-count
	// reads that are intentionally off the narrow Sessions interface. cmd/tripbot
	// assigns the same *users.Sessions it wraps into Sessions. nil in tests and
	// the brief startup window before cmd assigns it.
	UserSessions *users.Sessions
	// NowPlaying reports the currently-playing track on the stream's
	// background audio source. Tests inject a fake; production uses
	// realNowPlaying which polls SomaFM.
	NowPlaying NowPlaying
	// Flags evaluates feature flag values for command-time gating. Tests
	// inject noopFlags{} (every key false); New() defaults it to an empty
	// in-memory client (same fail-closed contract) for the startup window
	// before cmd/tripbot assigns the Postgres-backed client once the DB
	// connection is up.
	Flags feature.FlagClient
	// NATS is the fire-and-forget pubsub surface. Tests inject a
	// recordingNATS to assert on publishes; production uses realNATS
	// which delegates to the pkg/natsclient singleton (no-op when
	// NATS_URL is empty).
	NATS NATS
	// Cron stops the background scheduler during !shutdown. Defaults to
	// noopCron{} (set in New(), also what tests use); cmd/tripbot assigns the
	// constructed *background.Scheduler — which satisfies Cron directly — once
	// cron has started.
	Cron Cron
	// Geocoder turns GPS coords into a place name for !location. Tests inject
	// a recordingGeocoder / noopGeocoder; production uses realGeocoder which
	// delegates to the pkg/geo default configured in ConnectIRC.
	Geocoder Geocoder
	// Twitch is the command-time Twitch Helix surface (follow lookups today).
	// Tests inject a recordingTwitch; production uses realTwitch which
	// delegates to the pkg/twitch client. The future swap point for an
	// out-of-process Helix/auth service.
	Twitch Twitch

	// commands is this App's command registry (built by buildRegistry);
	// singleWordLookup / multiWordLookup index it by trigger + alias for
	// dispatch. Built by indexCommands(), called from New() at construction.
	// Replaces the former package-level globals so the registry travels with
	// the App.
	commands         []Command
	singleWordLookup map[string]*Command
	multiWordLookup  map[string]*Command
}

// db returns the DB handle the App should use. Prefers an explicit a.DB
// (which tests set to a sqlmock-backed gorm.DB), otherwise falls back to the
// process-wide singleton. Lazy so package init never touches the DB.
func (a *App) db() *gorm.DB {
	if a.DB != nil {
		return a.DB
	}
	return database.GormDB()
}

// New constructs an App wired with the production (realX) dependency adapters,
// with its command registry built and indexed. cmd/tripbot builds the live App
// with this and owns it; nothing in the package holds a singleton. Construction
// touches no network or DB — the realX adapters are lazy.
func New() *App {
	a := &App{
		// DB stays nil; commands use a.db() which falls back to database.GormDB().
		Onscreens:  realOnscreens{c: onscreensClient.New(c.Conf.OnscreensServerHost, natsclient.DefaultPublisher(), c.Conf.Environment)},
		VLC:        realVLC{c: vlcClient.New(c.Conf.VlcServerHost)},
		Video:      realVideo{},
		IRC:        realIRC{},
		Sessions:   realSessions{},
		NowPlaying: newRealNowPlaying(),
		Flags:      feature.NewInMemoryClient(nil),
		NATS:       realNATS{},
		Cron:       noopCron{},
		Geocoder:   realGeocoder{},
		Twitch:     realTwitch{},
	}
	a.indexCommands()
	return a
}

// used to determine which help message to display
// randomized so it starts with a new one every restart
var helpIndex = rand.Intn(len(c.HelpMessages))

const followerMsg = "Right now only followers of the channel can run unlimited commands :)"
const subscriberMsg = "You must be a subscriber to run that command :)"

// followerGatingEnabled toggles the RequiresFollow access check in
// checkAccess. Disabled for launch so first-time viewers aren't told to
// follow before they can try commands. Flip back to true to re-enable.
var followerGatingEnabled = false

// ConnectIRC builds the Twitch IRC client, wires this App's inbound adapters to
// it, and returns it. Also does the process-wide geocoder + Twitch-API warmup.
// The returned client is connected by the caller (cmd/tripbot).
func (a *App) ConnectIRC() *twitch.Client {
	Uptime = time.Now()

	// set up the process-wide geocoder (coords -> places). realGeocoder and
	// pkg/video both route through this default.
	geo.SetDefault(geo.New(c.Conf.GoogleMapsAPIKey))

	// initialize the twitch API client. Non-fatal: if Twitch is unreachable
	// at boot, log and continue so the process stays up (readiness reports
	// not-ready until the IRC connection lands). mytwitch.Client() doesn't
	// cache on failure, so callers retry once Twitch is back.
	if _, err := mytwitch.Client(); err != nil {
		slog.Error("twitch API client unavailable at startup; continuing", "err", err)
	}

	// The IRC token comes from the DB-backed oauth_tokens row populated by
	// cmd/auth-bootstrap; cmd/tripbot calls mytwitch.LoadFromDB before this.
	client = twitch.NewClient(c.Conf.BotUsername, mytwitch.IRCAuthToken())

	// attach this App's Twitch inbound adapters
	client.OnUserJoinMessage(a.onTwitchJoin)
	client.OnUserPartMessage(a.onTwitchPart)
	// client.OnUserNoticeMessage(...)
	client.OnWhisperMessage(a.onTwitchWhisper)
	client.OnPrivateMessage(a.onTwitchMessage)

	return client
}

// Say will make a post in chat
func Say(msg string) {
	// include the message in the log
	mylog.ChatMsg(c.Conf.BotUsername, msg)
	// mirror the bot's own output onto the event bus so it shows in the admin
	// live console — Twitch doesn't echo our sent messages back via
	// PrivateMessage, so without this the console would miss everything the bot
	// says. Fire-and-forget; no-op when NATS is unconfigured.
	eventbus.EmitChatMessage(context.Background(), c.Conf.Environment, c.Conf.BotUsername, msg)
	// figure out what channel to speak to
	speakTo := c.Conf.ChannelName
	if c.Conf.OutputChannel != "" {
		speakTo = c.Conf.OutputChannel
	}
	// say the message to chat
	client.Say(speakTo, msg)
}

// Whisper will whisper a message to a user
// Note: go-twitch-irc v4 removed the Whisper() send method; we replicate the
// v2 behavior by sending the raw IRC /w command via PRIVMSG on the bot's own channel.
func Whisper(username, msg string) {
	//TODO: include whispers in log
	// include the message in the log
	// mylog.ChatMsg(c.Conf.BotUsername, msg)
	slog.Info("sending whisper", "to", username, "text", msg)
	// say the message to chat
	client.Say(c.Conf.BotUsername, fmt.Sprintf("/w %s %s", username, msg))
}

// Chatter is designed to post a randomized message on a timer.
// Right now it just posts random "help messages."
// ctx is forward-compat plumbing — a.IRC.Say doesn't take ctx yet, so it's
// not propagated into the IRC write.
func (a *App) Chatter(_ context.Context) {
	// use twitch emote feature to add some color
	a.IRC.Say("/me " + help())
}

func help() string {
	text := c.HelpMessages[helpIndex]
	// bump the index
	helpIndex = (helpIndex + 1) % len(c.HelpMessages)
	return text
}
