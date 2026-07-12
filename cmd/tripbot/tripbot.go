package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adanalife/tripbot/pkg/achievements"
	"github.com/adanalife/tripbot/pkg/background"
	"github.com/adanalife/tripbot/pkg/chatbot"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/discord"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/eventsub"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/locationfeed"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs/audiowatchdog"
	"github.com/adanalife/tripbot/pkg/obs/watchdog"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	"github.com/adanalife/tripbot/pkg/rollups"
	"github.com/adanalife/tripbot/pkg/server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
	_ "github.com/dimiro1/banner/autoload"
	"github.com/gempir/go-twitch-irc/v4"
	"github.com/getsentry/sentry-go"
	"github.com/go-co-op/gocron/v2"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"runtime/debug"
)

var cronTracer = otel.Tracer("github.com/adanalife/tripbot/cmd/tripbot/cron")

// tracedJob wraps a cron callback in a span so each tick shows up as its
// own trace, records run-count / duration / last-run-timestamp metrics
// per job, and recovers panics so a single failing job doesn't kill the
// scheduler goroutine. The scheduler's job ctx is the span's parent and
// is threaded into fn, so DB queries (otelsql) and outbound HTTP
// (otelhttp) nest under cron.<name> in Tempo as children of the cron tick.
func tracedJob(name string, fn func(context.Context)) func(context.Context) {
	return func(ctx context.Context) {
		start := time.Now()
		ctx, span := cronTracer.Start(ctx, "cron."+name,
			trace.WithAttributes(attribute.String("cron.job", name)))
		defer span.End()
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "cron panic recovered",
					"job", name,
					"err", fmt.Sprintf("%v", r),
					"stack", string(debug.Stack()),
				)
				instrumentation.Cron.Panic(name)
				span.SetStatus(codes.Error, fmt.Sprintf("panic: %v", r))
			}
			instrumentation.Cron.Observe(name, time.Since(start).Seconds(), time.Now().Unix())
		}()
		fn(ctx)
	}
}

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

// Tripbot holds the bot process's runtime dependencies and wiring. The boot
// sequence (Run) and graceful shutdown are methods on it, so startup ordering
// is explicit and the deps are fields rather than package-level globals.
type Tripbot struct {
	version string

	// app is the chatbot App that owns the command registry and runs chat
	// commands + inbound handlers. Constructed in NewTripbot; setUpTwitchClient
	// wires its Twitch adapters to the IRC client (ConnectIRC), and eventsub /
	// cron register its methods. cmd owns this App; the package holds no
	// singleton.
	app *chatbot.App

	// irc is the go-twitch-irc client, constructed by setUpTwitchClient
	// (app.ConnectIRC) and shared by connectToTwitch, pollForTwitchToken
	// and the token-refresh cron job (SetIRCToken).
	irc *twitch.Client

	// scheduler is the background cron scheduler, constructed in startCron and
	// shared by scheduleBackgroundJobs (job registration) and gracefulShutdown
	// (Stop). Also assigned onto t.app.Cron so the !shutdown command can stop
	// it.
	scheduler *background.Scheduler

	// srv is the auth-links / console-API / metrics HTTP server, constructed in
	// NewTripbot. cmd installs the build version through it (SetVersion) and
	// starts it (Start). The rich admin panel moved to the standalone
	// tripbot-console; what's left is the OAuth bootstrap pages, the read-only
	// /api/* endpoints the console proxies, and /health + /metrics.
	srv *server.Server

	// player owns "what's currently playing" — the single process-wide
	// instance, constructed in NewTripbot. The 60s cron tick refreshes it
	// (GetCurrentlyPlaying); findInitialVideo + gracefulShutdown read it; it's
	// wrapped into the chatbot Video adapter (NewVideoAdapter) so commands read
	// the same state, and it publishes video.changed to NATS for the console.
	player *video.Player

	// sessions tracks who's currently in chat (the login map) + the
	// lifetime-miles leaderboard — the single process-wide instance,
	// constructed in NewTripbot. Cron jobs refresh it (UpdateSession /
	// UpdateLeaderboard); boot hydrates it (InitLeaderboard); gracefulShutdown
	// flushes it (Shutdown); assigned onto the chatbot App (Sessions adapter +
	// UserSessions) and into discord so they read the same state. One *Sessions
	// per chat provider is the multi-provider seam.
	sessions *users.Sessions

	telemetryShutdown telemetry.ShutdownFunc

	// discordSession is set by startDiscord when the Discord bot is enabled
	// for this env; gracefulShutdown calls Stop on it to deregister the
	// per-guild slash commands. Nil when Discord stays gated off.
	discordSession *discord.Session

	// flagClient is the process-wide feature flag evaluator. Initialised to an
	// empty in-memory client so unknown keys evaluate to false during the brief
	// startup window before startFeatureFlags swaps in the Postgres-backed
	// client — same fail-closed contract as pkg/feature.
	flagClient feature.FlagClient

	// gateway is the HTTP client for the platform-gateway — the single Helix
	// caller. Non-nil on a Twitch instance (TWITCH_API_URL is
	// set); nil on a non-Twitch instance, which never reaches the Twitch Helix
	// paths (all gated behind platformIsTwitch). The shared client the
	// non-chatbot Helix callers (the OBS watchdog's live-check, the chat-send
	// path) route through.
	gateway *gateway.Client

	// locationFeed publishes the currently-playing clip's location + date to the
	// onscreens rotators on a timer. Non-nil only on a bot-less YouTube instance
	// (youtube + inbound chat disabled), where it surfaces the info the
	// !location / !date / !state commands would return; nil disables the feed
	// and its background job.
	locationFeed *locationfeed.Emitter
}

// newGatewayClient builds the platform-gateway client when TWITCH_API_URL is
// set (a Twitch instance), else returns nil (a non-Twitch instance has no
// Twitch Helix surface). Stateless and side-effect free, so it's safe to
// construct at NewTripbot time.
func newGatewayClient() *gateway.Client {
	if c.Conf.TwitchAPIURL == "" {
		return nil
	}
	return gateway.New(c.Conf.TwitchAPIURL)
}

// NewTripbot constructs a Tripbot with default runtime state. Dependencies
// that need I/O or ordering (the IRC client, scheduler, Discord session,
// Postgres-backed flag client) are filled in by the boot-sequence methods.
func NewTripbot(version string) *Tripbot {
	t := &Tripbot{
		version: version,
		app:     chatbot.New(),
		srv:     server.New(),
		player: video.NewPlayer(
			onscreensClient.New(natsclient.DefaultPublisher(), c.Conf.Environment, c.Conf.Platform),
			vlcClient.New(c.Conf.VlcServerHost, natsclient.DefaultPublisher(), c.Conf.Environment),
		),
		flagClient: feature.NewInMemoryClient(nil),
		gateway:    newGatewayClient(),
	}
	// The audience source dispatches chatter refresh + the follower check to the
	// gateway (when the flag is on) or in-process; with no gateway wired it's the
	// plain in-process source. Reads t.gateway/t.flagClient lazily, so wiring it
	// here against the partially-built t is fine.
	t.sessions = users.New(gatewayChatterSource{t: t})
	// On a bot-less YouTube instance (no inbound chat → no commands), feed the
	// rotators the clip's location/date in place of command hints. Gated by the
	// same flag as the rest of the bot-less presentation; reuses the chatbot's
	// Geocoder (pkg/geo default, set up in ConnectYouTubeViaGateway).
	if c.Conf.Platform == "youtube" && !c.Conf.YouTubeInboundEnabled {
		t.locationFeed = locationfeed.New(
			onscreensClient.New(natsclient.DefaultPublisher(), c.Conf.Environment, c.Conf.Platform),
			t.app.Geocoder,
		)
	}
	return t
}

func main() {
	NewTripbot(version).Run()
}

// platformIsTwitch reports whether this instance serves Twitch. Empty
// Platform is treated as Twitch, matching the chatbot registry's contract.
func platformIsTwitch() bool {
	return c.Conf.Platform == "" || c.Conf.Platform == "twitch"
}

// Run performs the various steps to get the bot running. The spine —
// telemetry, HTTP server, player, sessions, cron, feature flags, NATS, the
// admin hub — is platform-neutral and runs on every instance; only the
// chat-transport bring-up swaps on STREAM_PLATFORM. Twitch-only steps (IRC
// token plumbing, EventSub, subscriber polling, the admin chat-send
// subscriber, Discord, the OBS↔Twitch watchdog) are gated off non-Twitch
// instances.
func (t *Tripbot) Run() {
	slog.Info("tripbot starting", "version", t.version, "platform", c.Conf.Platform)
	// shutdownCtx is canceled on SIGINT/SIGTERM; the HTTP server uses it
	// to trigger a graceful shutdown so in-flight requests aren't cut.
	// listenForShutdown's gracefulShutdown goroutine handles the rest of
	// the app cleanup off the same signals.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	t.listenForShutdown()
	t.initializeTelemetry()
	t.initializeErrorLogger()
	t.srv.SetVersion(t.version)
	t.startHttpServer(shutdownCtx)
	t.findInitialVideo()
	t.app.Video = chatbot.NewVideoAdapter(t.player)         // commands read the same Player the cron refreshes
	t.app.Sessions = chatbot.NewSessionsAdapter(t.sessions) // command-time queries
	t.app.UserSessions = t.sessions                         // inbound IRC handlers + access checks read the same session state
	if platformIsTwitch() {
		// Award viewer achievements on each clip transition. Twitch-only:
		// it's the instance with presence tracking, and achievements key off
		// who's in chat. Set after findInitialVideo so a restart doesn't
		// re-award for the clip already on screen.
		t.player.OnChange = func(ctx context.Context, v video.Video) {
			for _, msg := range achievements.HandleVideoChange(ctx, v, t.sessions.LoggedInHumans()) {
				t.app.Chat.Say(msg)
			}
		}
	}
	t.sessions.InitLeaderboard(context.Background())
	t.startCron()
	t.startFeatureFlags(shutdownCtx)
	if platformIsTwitch() {
		if t.gateway == nil {
			// No in-process Helix fallback since the cutover, so audience polls,
			// the follower check, and broadcaster send have no backend here.
			// Real deploys always wire TWITCH_API_URL; this is local/CI.
			slog.WarnContext(shutdownCtx, "no TWITCH_API_URL: Twitch audience/follower/broadcaster-send features disabled (gateway not wired)")
		}
		t.loadTwitchToken(shutdownCtx) // must precede setUpTwitchClient — provides the IRC token
		t.setUpTwitchClient()          // required for the below
		t.updateSubscribers()
		t.getCurrentUsers()
		t.startEventSub(shutdownCtx)
	}
	t.startNATS(shutdownCtx)
	t.player.EmitCurrentVideo(shutdownCtx)   // after startNATS: publishes the current video.changed for the standalone console
	t.startAuthStatusEmitter(shutdownCtx)    // after startNATS: publishes auth.status snapshots for the standalone console
	t.startOBSRefreshSubscriber(shutdownCtx) // after startNATS: per-platform (each instance owns its OBS), so before the YouTube early-return
	if !platformIsTwitch() {
		if c.Conf.Platform == "facebook" {
			t.connectToFacebook(shutdownCtx)
		} else {
			t.connectToYouTube(shutdownCtx)
		}
		return
	}
	// chat.send subjects are per-env, not per-platform — both platform
	// instances would receive every admin send, so only the Twitch instance
	// (which owns the bot/broadcaster identities the command names)
	// subscribes.
	t.startChatSendSubscriber(shutdownCtx)       // after startNATS + setUpTwitchClient: needs the conn and t.app.Chat
	t.startDiscord(shutdownCtx)                  // Discord stays Twitch-side for v1
	t.startSilentDisconnectWatchdog(shutdownCtx) // watches the OBS→Twitch stream specifically
	t.startBackgroundAudioWatchdog(shutdownCtx)  // keeps audible music on-stream when SomaFM drops
	t.connectToTwitch()
}

// connectToYouTube wires a PLATFORM=youtube instance's chat — both directions —
// through gateway-youtube, then blocks until shutdown. YouTube auth + runtime
// moved entirely onto the platform-gateway, so tripbot holds no YouTube token:
// outbound sends go via gateway-youtube's SendChat, inbound chat via its
// GET /v1/chat/inbound poll.
//
// YOUTUBE_API_URL is required — the in-process YouTube client is gone, so
// without the gateway URL there's no way to reach YouTube. A misconfigured
// instance comes up Ready with everything else working but no YouTube chat,
// logging loudly (the same "stay up with limited functionality" contract as
// loadTwitchToken).
func (t *Tripbot) connectToYouTube(ctx context.Context) {
	if c.Conf.YouTubeAPIURL == "" {
		slog.ErrorContext(ctx, "YOUTUBE_API_URL unset; youtube chat disabled",
			"fix", "set YOUTUBE_API_URL to the gateway-youtube service URL")
		<-ctx.Done()
		return
	}

	t.app.ConnectYouTubeViaGateway()
	if c.Conf.YouTubeInboundEnabled {
		go t.app.NewGatewayChatPoller(c.Conf.YouTubeAPIURL).Run(ctx)
		slog.InfoContext(ctx, "youtube chat via gateway (inbound + outbound)", "gateway", c.Conf.YouTubeAPIURL)
	} else {
		// Bot-less mode: outbound posting (rotators) + background jobs stay up,
		// but the inbound poll — the expensive YouTube Data API spend — is off,
		// so no command responds. The chatbot serves promo copy instead of
		// command ads (see enabledHelpMessages). Flip YOUTUBE_INBOUND_ENABLED
		// to true once the quota extension lands.
		slog.WarnContext(ctx, "youtube inbound chat disabled (bot-less mode); outbound + jobs only",
			"gateway", c.Conf.YouTubeAPIURL, "fix", "set YOUTUBE_INBOUND_ENABLED=true to read chat")
	}

	// nothing else to do on the main goroutine — the poller and HTTP server
	// run until the signal handler shuts the process down.
	<-ctx.Done()
}

// connectToFacebook wires a PLATFORM=facebook instance's chat — both
// directions — through gateway-facebook, then blocks until shutdown. The
// gateway owns the Page access token and the live-video resolution, so
// tripbot holds no Facebook credential: outbound sends go via
// gateway-facebook's SendChat (a Page comment on the live video), inbound
// chat via its GET /v1/chat/inbound poll.
//
// FACEBOOK_API_URL is required — without the gateway URL there's no way to
// reach Facebook. A misconfigured instance comes up Ready with everything
// else working but no Facebook chat, logging loudly (the same "stay up with
// limited functionality" contract as connectToYouTube).
func (t *Tripbot) connectToFacebook(ctx context.Context) {
	if c.Conf.FacebookAPIURL == "" {
		slog.ErrorContext(ctx, "FACEBOOK_API_URL unset; facebook chat disabled",
			"fix", "set FACEBOOK_API_URL to the gateway-facebook service URL")
		<-ctx.Done()
		return
	}

	t.app.ConnectFacebookViaGateway()
	go t.app.NewGatewayChatPoller(c.Conf.FacebookAPIURL).Run(ctx)
	slog.InfoContext(ctx, "facebook chat via gateway (inbound + outbound)", "gateway", c.Conf.FacebookAPIURL)

	// nothing else to do on the main goroutine — the poller and HTTP server
	// run until the signal handler shuts the process down.
	<-ctx.Done()
}

// featureFlagRefreshInterval is how often the Postgres-backed flag client
// re-reads the feature_flags table. 30s is chat-acceptable lag for
// dark-launches and kill-switches; revisit if a use case wants instant.
const featureFlagRefreshInterval = 30 * time.Second

// startFeatureFlags brings up the Postgres-backed feature flag client and
// installs it into the chatbot package. Non-fatal: a startup failure (DB
// hiccup, missing migration) logs loudly and leaves chatbot's package-level
// empty in-memory client in place — every flag evaluates to its default
// (false) until the next restart loads cleanly. Mirrors the loadTwitchToken
// "stay up with limited functionality" pattern.
func (t *Tripbot) startFeatureFlags(ctx context.Context) {
	fc, err := feature.NewPostgresClient(ctx, database.GormDB(), featureFlagRefreshInterval, c.Conf.Platform)
	if err != nil {
		slog.WarnContext(ctx, "feature flag client init failed; flags will default to off",
			"fix", "ensure migration 013_create_feature_flags has run",
			"err", err)
		return
	}
	t.flagClient = fc
	t.app.Flags = fc   // command-time flag gating reads the same Postgres-backed client
	t.srv.SetFlags(fc) // the console's /api/flags endpoints read/toggle the same client
	go fc.Start(ctx)
}

// startNATS connects to the in-cluster NATS broker and declares the JetStream
// streams that back the standalone tripbot-console's durable history. Optional —
// when NATS_URL is empty the connection is skipped and publishes no-op silently.
//
// EnsureStreams declares the JetStream streams the standalone tripbot-console
// consumes (chat + video history), so they exist before the publishers emit.
// It runs in the on-connect callback so it executes against a live server
// even when the first dial loses the boot race and the client connects late.
// It no-ops when JetStream is unavailable (a server without JetStream) —
// publishes then fall back to live-only core subjects, so a stream-declare
// failure must not be fatal.
func (t *Tripbot) startNATS(ctx context.Context) {
	natsclient.Connect(c.Conf.NatsURL, "tripbot", func(*nats.Conn) {
		if err := eventbus.EnsureStreams(ctx, natsclient.JetStream(), c.Conf.Environment); err != nil {
			slog.WarnContext(ctx, "jetstream stream setup failed; console will run without durable history",
				"err", err)
		}
	})
}

// authStatusInterval is how often the instance publishes its auth.status
// snapshot. Matches the in-process panel's 30s pollAuth cadence — token expiry
// moves on the order of minutes/hours, and the TRIPBOT_AUTH last-value stream
// means a freshly-connected console is at most one interval stale.
const authStatusInterval = 30 * time.Second

// startAuthStatusEmitter publishes this instance's token state to
// tripbot.<env>.auth.status.twitch on start and every authStatusInterval.
// The in-process admin hub ignores the subject (it polls token state directly);
// the standalone console is the consumer. Snapshots are assembled here — not in
// pkg/eventbus — so the eventbus stays free of pkg/twitch imports.
//
// Only the Twitch instance holds tokens now: YouTube auth moved entirely onto
// the platform-gateway (gateway-youtube owns the oauth_tokens youtube row), so a
// youtube instance has no token state to report and skips this. (Surfacing
// YouTube auth status to the console is the gateway's job once it grows a NATS
// publisher — tracked separately.)
func (t *Tripbot) startAuthStatusEmitter(ctx context.Context) {
	if !platformIsTwitch() {
		return
	}
	go func() {
		eventbus.EmitAuthStatus(ctx, c.Conf.Environment, "twitch", twitchAuthAccounts())
		tick := time.NewTicker(authStatusInterval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				eventbus.EmitAuthStatus(ctx, c.Conf.Environment, "twitch", twitchAuthAccounts())
			}
		}
	}()
}

// twitchAuthAccounts converts the live Twitch token state (bot + broadcaster)
// into the eventbus wire shape.
func twitchAuthAccounts() []eventbus.AuthAccount {
	statuses := mytwitch.TokenStatuses()
	accounts := make([]eventbus.AuthAccount, 0, len(statuses))
	for _, s := range statuses {
		expiresAt := ""
		if !s.ExpiresAt.IsZero() {
			expiresAt = s.ExpiresAt.UTC().Format(time.RFC3339Nano)
		}
		accounts = append(accounts, eventbus.AuthAccount{
			Account:   s.Account,
			LoginAs:   s.LoginAs,
			ExpiresAt: expiresAt,
			Reason:    s.Reason,
		})
	}
	return accounts
}

// startSilentDisconnectWatchdog launches the goroutine that detects the
// half-open RTMP state where OBS reports outputActive=true but Twitch's
// API shows the channel offline, and force-restarts the stream after
// 3 consecutive minute-spaced misalignments. First seen in prod on
// 2026-05-27, ~30h into an OBS session.
func (t *Tripbot) startSilentDisconnectWatchdog(ctx context.Context) {
	deps := watchdog.DefaultWatchdogDeps()
	// The live-check routes through the platform-gateway (the single Helix
	// caller). This wiring lives here in cmd (not in pkg/obs/watchdog) per
	// package-boundary-init-discipline. A nil gateway is a misconfigured Twitch
	// instance (TWITCH_API_URL unset) — report the check as errored rather than
	// force-restarting on a false negative.
	deps.TwitchLive = func(ctx context.Context) (bool, error) {
		if t.gateway == nil {
			return false, errors.New("watchdog live-check: no gateway configured")
		}
		live, err := t.gateway.IsLive(ctx, c.Conf.ChannelName)
		if err == nil {
			instrumentation.TwitchChannelLive.Set(live)
		}
		return live, err
	}
	go watchdog.WatchSilentDisconnect(ctx, deps, 60*time.Second, 3, 10*time.Minute)
}

// startBackgroundAudioWatchdog launches the volume-meter connection + the
// background-audio watchdog that keeps audible music on the Twitch stream when
// SomaFM drops — swapping the "Groove Salad Classic" source onto the local Car
// Hum bed when it goes silent and back when SomaFM recovers. Twitch-only:
// SomaFM is the Twitch music bed; YouTube already runs the always-available
// local Car Hum. First seen needed in prod on 2026-06-23, when a full SomaFM
// edge outage left the stream silent with no self-heal.
func (t *Tripbot) startBackgroundAudioWatchdog(ctx context.Context) {
	meter := audiowatchdog.NewVolumeMeter(audiowatchdog.BackgroundAudioInputName, 30*time.Second)
	go meter.Run(ctx)
	go audiowatchdog.Watch(ctx, audiowatchdog.DefaultDeps(meter), audiowatchdog.DefaultConfig())
}

// startDiscord brings up the bot's Discord slash-command session when
// the env supplies the required config and the discord.bot_enabled feature
// flag is on. Every failure path here logs and returns so it can't block
// (or crash) tripbot startup — Discord is additive to the core IRC /
// EventSub paths.
func (t *Tripbot) startDiscord(ctx context.Context) {
	if ok, reason := discord.ShouldStart(c.Conf); !ok {
		slog.InfoContext(ctx, "discord disabled", "reason", reason)
		return
	}
	if !t.flagClient.Bool(ctx, discord.FlagKey, feature.EvalContext{Env: c.Conf.Environment}) {
		slog.InfoContext(ctx, "discord disabled by feature flag", "flag", discord.FlagKey)
		return
	}
	session, err := discord.New(c.Conf.DiscordBotToken, c.Conf.DiscordGuildID, t.sessions)
	if err != nil {
		slog.ErrorContext(ctx, "discord init failed", "err", err)
		return
	}
	if err := session.Start(ctx); err != nil {
		slog.ErrorContext(ctx, "discord start failed", "err", err)
		return
	}
	t.discordSession = session
}

// startEventSub kicks off the EventSub WebSocket listener in a goroutine
// so real-time follow/subscribe events fire chat shouts without a 5min
// polling delay. Skipped (logged, not fatal) when the broadcaster row
// isn't loaded — the bot still runs without real-time alerts.
func (t *Tripbot) startEventSub(ctx context.Context) {
	token := mytwitch.BroadcasterUserAccessToken()
	if token == "" {
		slog.WarnContext(ctx, "skipping eventsub: no broadcaster oauth_tokens row; re-auth via the platform-gateway consent flow (surfaced in tripbot-console)",
			"login_as", c.Conf.ChannelName)
		return
	}
	if mytwitch.ChannelID() == "" && t.gateway != nil {
		// The gateway owns Helix, so nothing populates channelID in-process any
		// more. Resolve it via the gateway's /v1/users/{login} so EventSub gets a
		// BroadcasterUserID. Non-fatal — falls through to the skip below on error.
		if id, err := t.gateway.UserID(ctx, c.Conf.ChannelName); err != nil {
			slog.ErrorContext(ctx, "eventsub: resolving channel id via gateway failed", "err", err)
		} else {
			mytwitch.SetChannelID(id)
		}
	}
	if mytwitch.ChannelID() == "" {
		slog.WarnContext(ctx, "skipping eventsub: ChannelID not yet resolved")
		return
	}
	go func() {
		err := eventsub.Run(ctx, eventsub.Config{
			ClientID:          mytwitch.ClientID,
			BroadcasterToken:  token,
			BroadcasterUserID: mytwitch.ChannelID(),
		}, eventsub.Handlers{
			OnFollow:      t.app.AnnounceNewFollower,
			OnSubscribe:   t.app.AnnounceSubscriber,
			OnUnsubscribe: t.app.RecordUnsubscribe,
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.ErrorContext(ctx, "eventsub run terminated", "err", err)
		}
	}()
}

// listenForShutdown creates a background job that listens for a graceful shutdown request
func (t *Tripbot) listenForShutdown() {
	helpers.WritePidFile(c.Conf.TripbotPidFile)
	// start the graceful shutdown listener
	go t.gracefulShutdown()
}

// initializeTelemetry brings up OpenTelemetry providers (traces, metrics,
// logs). No-ops cleanly if OTEL_SDK_DISABLED is set or no OTLP endpoint
// is configured — see pkg/telemetry.
func (t *Tripbot) initializeTelemetry() {
	ctx := context.Background()
	shutdown, err := telemetry.Init(ctx, "tripbot", t.version)
	if err != nil {
		// telemetry init failure shouldn't crash the bot — log and continue.
		slog.WarnContext(ctx, "telemetry init failed", "err", err)
	}
	t.telemetryShutdown = shutdown
}

// initializeErrorLogger makes sure the logger is configured
func (t *Tripbot) initializeErrorLogger() {
	terrors.Initialize(c.Conf, t.version)
}

// startHttpServer starts a webserver, which is
// used for admin tools and receiving webhooks. The passed context is
// honored by the server for graceful shutdown — when it's canceled,
// the server stops accepting new connections and drains in-flight
// requests up to its shutdown timeout.
func (t *Tripbot) startHttpServer(ctx context.Context) {
	// start the HTTP server
	go t.srv.Start(ctx)
}

// findInitialVideo will determine the vido that is currently-playing
// we want to run this early, otherwise it will be unset until the first cron job runs
func (t *Tripbot) findInitialVideo() {
	t.player.GetCurrentlyPlaying(context.Background())
	v := t.player.Current()
	_, err := video.LoadOrCreate(context.Background(), v.String())
	if err != nil {
		slog.Error("error loading initial video, is there a video playing?", "err", err)
	}
}

// startCron starts the background workers
func (t *Tripbot) startCron() {
	s, err := background.New()
	if err != nil {
		slog.Error("error creating background scheduler", "err", err)
		os.Exit(1)
	}
	t.scheduler = s
	t.scheduler.Start()
	// let !shutdown stop the same scheduler instance (*background.Scheduler
	// satisfies chatbot.Cron directly)
	t.app.Cron = t.scheduler
	t.scheduleBackgroundJobs()
}

// loadTwitchToken pulls the bot's OAuth row from the oauth_tokens table.
// Non-fatal: when the row is missing (e.g. auth-bootstrap hasn't run yet
// against a freshly-restored DB) or the DB is briefly unreachable, the bot
// comes up with limited functionality and polls in the background until the
// token lands — rather than crashlooping (a crashlooping pod can also race a
// concurrent DB restore's migrate init). The pod stays Ready throughout
// (readiness doesn't gate on Twitch) so the auth-links landing page + /auth/init are
// reachable to re-auth; "not in chat" is surfaced via the
// tripbot_twitch_connected gauge instead.
func (t *Tripbot) loadTwitchToken(ctx context.Context) {
	if err := mytwitch.LoadFromDB(); err != nil {
		slog.WarnContext(ctx, "no usable Twitch token at boot; starting without a chat connection and polling",
			"login_as", c.Conf.BotUsername,
			"fix", "re-auth via the platform-gateway consent flow (surfaced in tripbot-console)",
			"err", err)
		go t.pollForTwitchToken(ctx)
	}
}

// pollForTwitchToken retries LoadFromDB until the bot's oauth_tokens row is
// available, then syncs the freshly-loaded IRC token into the client so
// connectToTwitch's reconnect loop authenticates on its next attempt. Started
// only when the token was missing at boot; stops on shutdown.
func (t *Tripbot) pollForTwitchToken(ctx context.Context) {
	// Check often so the token is picked up promptly once it lands, but log
	// the "still waiting" warning at a much slower cadence — boot already
	// logged the reauth link once, so re-surfacing it every 15s is just noise.
	const (
		interval = 15 * time.Second
		logEvery = 15 * time.Minute
	)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Suppress the first poll-failure log: loadTwitchToken just logged the
	// same warning at boot. The next one waits a full logEvery.
	lastLogged := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := mytwitch.LoadFromDB(); err != nil {
				if time.Since(lastLogged) >= logEvery {
					slog.WarnContext(ctx, "still waiting for Twitch token (re-auth via the platform-gateway consent flow, surfaced in tripbot-console)",
						"login_as", c.Conf.BotUsername, "err", err)
					lastLogged = time.Now()
				}
				continue
			}
			slog.InfoContext(ctx, "Twitch token loaded; bot will connect on next attempt")
			// Push the freshly-loaded token into the (already-constructed)
			// IRC client so the connect loop's next try uses it instead of
			// the empty token captured at ConnectIRC.
			if tok := mytwitch.IRCAuthToken(); tok != "" && t.irc != nil {
				t.irc.SetIRCToken(tok)
			}
			return
		}
	}
}

// setUpTwitchClient sets up the Twitch client,
// used by many bot features
func (t *Tripbot) setUpTwitchClient() {
	// build the Twitch IRC client and wire the App's inbound adapters to it
	t.irc = t.app.ConnectIRC()
}

// updateSubscribers gets the list of current subscribers (gateway-or-in-process
// per the runtime flag — see refreshSubscribers).
func (t *Tripbot) updateSubscribers() {
	t.refreshSubscribers(context.Background())
}

// getCurrentUsers gets the users watching the stream
func (t *Tripbot) getCurrentUsers() {
	// fetch initial session
	t.sessions.UpdateSession(context.Background())
	t.sessions.PrintCurrentSession(context.Background())
}

// connectToTwitch joins Twitch chat and starts listening
func (t *Tripbot) connectToTwitch() {
	t.irc.Join(c.Conf.ChannelName)
	slog.Info("joined channel", "channel", c.Conf.ChannelName, "url", fmt.Sprintf("https://twitch.tv/%s", c.Conf.ChannelName))

	// Mark the bot connected to chat once the IRC connection is established.
	// This drives the tripbot_twitch_connected gauge — it does NOT gate
	// /health/ready, which stays 200 so the pod keeps serving /auth/* and the
	// console-facing /api/* endpoints even while the bot is offline.
	t.irc.OnConnect(func() {
		slog.Info("connected to Twitch chat")
		instrumentation.TwitchConnection.Set(true)
	})

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		slog.Info("initializing connection to Twitch")
		// Connect blocks while connected and returns when the connection
		// drops; mark not-in-chat so the gauge reflects the gap until the next
		// OnConnect fires.
		err := t.irc.Connect()
		instrumentation.TwitchConnection.Set(false)
		if err != nil {
			slog.Error("unable to connect to twitch", "err", err)
			if errors.Is(err, twitch.ErrLoginAuthenticationFailed) {
				// The IRC client's token was rejected. Re-read the bot row from
				// oauth_tokens — the platform-gateway keeps it fresh, so a token
				// it just rotated (or one auth-bootstrap wrote) is picked up
				// without a restart. Then sync whatever's now in memory into the
				// IRC client for the next Connect attempt.
				if err := mytwitch.LoadFromDB(); err != nil {
					slog.Warn("IRC auth failed; re-reading oauth_tokens failed", "err", err, "login_as", c.Conf.BotUsername)
				}
				if tok := mytwitch.IRCAuthToken(); tok != "" {
					t.irc.SetIRCToken(tok)
				} else {
					// No usable token in the DB yet (e.g. the row is unseeded).
					// Re-auth runs through the platform-gateway consent flow now
					// (surfaced in tripbot-console); the gateway writes the row.
					slog.Error("IRC auth failed and no valid token in oauth_tokens; re-auth via the platform-gateway consent flow (surfaced in tripbot-console)", "login_as", c.Conf.BotUsername)
				}
			}
			time.Sleep(time.Minute)
		}
	}
}

// gracefulShutdown catches CTRL-C and cleans up
func (t *Tripbot) gracefulShutdown() {
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt, syscall.SIGTERM)

	// wait for signal
	<-ctrlC

	slog.Warn("caught CTRL-C, shutting down")
	//TODO: print different message if CurrentlyPlaying is ""
	slog.Info("last played video", "file", t.player.Current().File())
	if t.discordSession != nil {
		if err := t.discordSession.Stop(); err != nil {
			slog.Error("discord stop failed", "err", err)
		}
	}
	t.sessions.Shutdown(context.Background())
	err := database.Close()
	if err != nil {
		slog.Error("error closing DB connection", "err", err)
	}
	if err := t.scheduler.Stop(); err != nil {
		slog.Error("error shutting down gocron scheduler", "err", err)
	}
	sentry.Flush(time.Second * 5)
	if t.telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := t.telemetryShutdown(flushCtx); err != nil {
			slog.ErrorContext(flushCtx, "telemetry shutdown failed", "err", err)
		}
		cancel()
	}
	os.Exit(1)
}

// scheduleBackgroundJobs schedules the various background jobs.
// Lives in this package (not pkg/background) to avoid circular deps with
// the job-target packages.
func (t *Tripbot) scheduleBackgroundJobs() {
	// platform-neutral jobs: every instance plays video and posts the
	// periodic help message.
	t.addJob(60*time.Second, "video.GetCurrentlyPlaying", t.player.GetCurrentlyPlaying)
	t.addJob(2*time.Hour+57*time.Minute+30*time.Second, "chatbot.Chatter", t.app.Chatter)
	// Bot-less YouTube only: refresh the rotators' location/date feed every
	// minute. Re-publishing (not just on video change) also recovers a restarted
	// onscreens-server within a tick; the city geocode is throttled inside Emit.
	if t.locationFeed != nil {
		t.addJob(60*time.Second, "video.LocationFeed", func(ctx context.Context) {
			t.locationFeed.Emit(ctx, t.player.Current())
		})
	}

	// YouTube instances (bot-less or full): discover the current broadcast's
	// videoId on a slow ticker and publish it for the console, which links to and
	// embeds the broadcast directly. Needed because an unlisted broadcast's
	// channel/handle "/live" redirect only resolves a public stream. One quota
	// unit per poll — negligible even against prod's constrained quota — and it
	// runs regardless of YOUTUBE_INBOUND_ENABLED (discovery is not the chat read).
	// WithStartImmediately so a fresh console sees the link without a full
	// interval's wait; the last-value cache then retains it.
	if !platformIsTwitch() && c.Conf.YouTubeAPIURL != "" {
		ytGateway := gateway.New(c.Conf.YouTubeAPIURL)
		t.addJob(2*time.Minute, "youtube.BroadcastDiscovery", func(ctx context.Context) {
			b, err := ytGateway.ActiveBroadcast(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "youtube broadcast discovery failed", "err", err)
				return
			}
			eventbus.EmitYoutubeBroadcast(ctx, c.Conf.Environment, b.VideoID, b.Privacy, b.Live)
		}, gocron.WithStartAt(gocron.WithStartImmediately()))
	}

	if !platformIsTwitch() {
		// Twitch-sourced jobs stay off non-Twitch instances: session/presence
		// tracking reads Twitch chatters (YouTube presence is punted in v1),
		// the leaderboards back excluded commands, the subscriber /
		// follower polls hit Helix, and the token-refresh job dereferences the
		// IRC client this instance never constructs.
		return
	}
	t.addJob(61*time.Second, "users.UpdateSession", t.sessions.UpdateSession)
	t.addJob(62*time.Second, "users.UpdateLeaderboard", t.sessions.UpdateLeaderboard)
	// Derived-state reconciler over the events table (all platforms' events,
	// but only one instance should run it — the twitch gate above covers that).
	// Singleton mode + the reconciler's own row lock make overlap harmless.
	t.addJob(5*time.Minute, "rollups.Reconcile", rollups.Reconcile,
		gocron.WithSingletonMode(gocron.LimitModeReschedule))
	t.addJob(5*time.Minute, "chatbot.ShowRotatingLeaderboard", t.app.ShowRotatingLeaderboard)
	t.addJob(5*time.Minute, "users.PrintCurrentSession", t.sessions.PrintCurrentSession)
	t.addJob(5*time.Minute, "twitch.GetSubscribers", t.refreshSubscribers)
	t.addJob(5*time.Minute, "twitch.GetFollowerCount", t.refreshFollowerCount)
	// The platform-gateway owns token refresh now; tripbot only reads the rows
	// it keeps fresh. Re-read on a timer so the in-memory tokens track the
	// gateway's rotations — the IRC PASS line on reconnect and the token-expiry
	// gauge (both fed by LoadFromDB) — without tripbot ever refreshing itself.
	t.addJob(5*time.Minute, "twitch.ReloadTokens", func(ctx context.Context) {
		if err := mytwitch.LoadFromDB(); err != nil {
			slog.WarnContext(ctx, "periodic oauth_tokens reload failed", "err", err)
		}
		// Keep the IRC client's stored token in sync with the rotated credentials.
		// go-twitch-irc captures the token at construction; without this, any
		// reconnect after the first rotation replays the original boot-time token.
		if tok := mytwitch.IRCAuthToken(); tok != "" {
			t.irc.SetIRCToken(tok)
		}
	})
}

// addJob registers a gocron job at the given interval, wrapping fn with
// tracedJob so each tick opens a span and centralising the error logging.
// Extra gocron.JobOptions (e.g. WithStartAt for an immediate first run) are
// appended verbatim; existing callers pass none.
func (t *Tripbot) addJob(interval time.Duration, name string, fn func(context.Context), opts ...gocron.JobOption) {
	_, err := t.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(tracedJob(name, fn)),
		opts...,
	)
	if err != nil {
		slog.Error("error adding background job: "+name, "err", err)
	}
}
