package chatbot

import (
	"context"
	"log/slog"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
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
var Uptime time.Time

// App holds injectable dependencies for the chatbot. cmd/tripbot constructs the
// live one with New(); tests instantiate it directly with fakes.
type App struct {
	// Platform names the streaming platform this App serves ("twitch" /
	// "youtube"). It gates which commands are indexed for dispatch
	// (indexCommands): Twitch runs the full registry, YouTube runs the v1
	// allowlist. Empty is treated as Twitch. Set from c.Conf.Platform in New().
	Platform string
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
	// Chat sends chat output (Say, Whisper) to the streaming platform. Tests
	// inject a recordingChat to assert on messages; production uses the
	// twitchChat adapter (wrapped in the console mirror) that ConnectIRC wires
	// to this App's own *twitch.Client. The provider-neutral seam: a YouTube /
	// TikTok ChatClient drops in here without touching command code.
	Chat ChatClient
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
	// Weather returns historical conditions at a point for !weather. Tests
	// inject noopWeather; production uses realWeather, which queries the
	// keyless Open-Meteo archive API.
	Weather Weather
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

	// helpMessages is the platform-filtered subset of c.HelpMessages — the
	// rotating !help / Chatter lines, minus any whose command isn't enabled on
	// this App's platform (so a YouTube instance doesn't advertise !miles etc.).
	// helpIndex walks it; randomized at indexCommands() so each restart starts
	// on a different line.
	helpMessages []string
	helpIndex    int
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
		Platform: c.Conf.Platform,
		// DB stays nil; commands use a.db() which falls back to database.GormDB().
		Onscreens:  realOnscreens{c: onscreensClient.New(natsclient.DefaultPublisher(), c.Conf.Environment)},
		VLC:        realVLC{c: vlcClient.New(c.Conf.VlcServerHost, natsclient.DefaultPublisher(), c.Conf.Environment)},
		Video:      realVideo{},
		Chat:       disconnectedChat{},
		Sessions:   realSessions{},
		NowPlaying: newRealNowPlaying(),
		Flags:      feature.NewInMemoryClient(nil),
		NATS:       realNATS{},
		Cron:       noopCron{},
		Geocoder:   realGeocoder{},
		Weather:    realWeather{},
		Twitch:     realTwitch{},
	}
	a.indexCommands()
	return a
}

const followerMsg = "Right now only followers of the channel can run unlimited commands :)"
const subscriberMsg = "You must be a subscriber to run that command :)"

// followerGatingEnabled toggles the RequiresFollow access check in
// checkAccess. Disabled for launch so first-time viewers aren't told to
// follow before they can try commands. Flip back to true to re-enable.
var followerGatingEnabled = false

// ConnectIRC builds the Twitch IRC client, wires this App's inbound adapters to
// it, points this App's outbound Chat at it, and returns it. Also does the
// process-wide geocoder + Twitch-API warmup. The returned client is connected
// by the caller (cmd/tripbot).
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
	client := twitch.NewClient(c.Conf.BotUsername, mytwitch.IRCAuthToken())

	// attach this App's Twitch inbound adapters
	client.OnUserJoinMessage(a.onTwitchJoin)
	client.OnUserPartMessage(a.onTwitchPart)
	// client.OnUserNoticeMessage(...)
	client.OnWhisperMessage(a.onTwitchWhisper)
	client.OnPrivateMessage(a.onTwitchMessage)

	// point this App's outbound chat at the Twitch client, wrapped in the
	// provider-neutral console mirror so the bot's own output reaches the admin
	// live console. A second provider (YouTube/…) wires its own ChatClient here.
	a.Chat = consoleMirror{
		inner: twitchChat{
			client:        client,
			channelName:   c.Conf.ChannelName,
			outputChannel: c.Conf.OutputChannel,
			botUsername:   c.Conf.BotUsername,
		},
		env:         c.Conf.Environment,
		platform:    c.Conf.Platform,
		botUsername: c.Conf.BotUsername,
	}

	return client
}

// Chatter is designed to post a randomized message on a timer.
// Right now it just posts random "help messages."
// ctx is forward-compat plumbing — a.Chat.Say doesn't take ctx yet, so it's
// not propagated into the chat write.
func (a *App) Chatter(_ context.Context) {
	// the "/me " twitch emote prefix adds some color on Twitch; youtubeChat.Say
	// strips it (it would render as literal text on YouTube).
	a.Chat.Say("/me " + a.help())
}

// help returns the next rotating help message for this App's platform and
// advances the index. Empty when no help messages are enabled (guards against
// a divide-by-zero on the modulo).
func (a *App) help() string {
	if len(a.helpMessages) == 0 {
		return ""
	}
	text := a.helpMessages[a.helpIndex]
	a.helpIndex = (a.helpIndex + 1) % len(a.helpMessages)
	return text
}
