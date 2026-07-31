package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/adanalife/tripbot/pkg/bootstrap"
	"github.com/adanalife/tripbot/pkg/chatbot"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/discord"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/eventsub"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/locationfeed"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/obs/audiowatchdog"
	"github.com/adanalife/tripbot/pkg/obs/beds"
	"github.com/adanalife/tripbot/pkg/obs/watchdog"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	playoutClient "github.com/adanalife/tripbot/pkg/playout-client"
	"github.com/adanalife/tripbot/pkg/rollups"
	"github.com/adanalife/tripbot/pkg/rotatorstore"
	"github.com/adanalife/tripbot/pkg/server"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gempir/go-twitch-irc/v4"
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

	// cfg is the process config, loaded once in main and threaded into every
	// constructor — nothing reads a package-level config global.
	cfg *c.TripbotConfig

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

	// beds owns which background-audio bed this platform's OBS is playing and
	// applies switches to it. Constructed in startBackgroundAudio and shared by
	// the console API (/api/audio) and the audio watchdog (album advance).
	beds *beds.Store

	// scheduler is the background cron scheduler, constructed in startCron and
	// shared by scheduleBackgroundJobs (job registration) and shutdown
	// (Shutdown). Also assigned onto t.app.Cron so the !shutdown command can
	// stop it.
	scheduler gocron.Scheduler

	// srv is the auth-links / console-API / metrics HTTP server, constructed in
	// NewTripbot. cmd installs the build version through it (SetVersion) and
	// starts it (Start). The rich admin panel lives in the standalone
	// tripbot-console; this server holds the OAuth bootstrap pages, the read-only
	// /api/* endpoints the console proxies, and /health + /metrics.
	srv *server.Server

	// player owns "what's currently playing" — the single process-wide
	// instance, constructed in NewTripbot. The 60s cron tick refreshes it
	// (GetCurrentlyPlaying); findInitialVideo + shutdown read it; it's
	// wrapped into the chatbot Video adapter (NewVideoAdapter) so commands read
	// the same state, and it publishes video.changed to NATS for the console.
	player *video.Player

	// sessions tracks who's currently in chat (the login map) + the
	// lifetime-miles leaderboard — the single process-wide instance,
	// constructed in NewTripbot. Cron jobs refresh it (UpdateSession /
	// UpdateLeaderboard); boot hydrates it (InitLeaderboard); shutdown
	// flushes it (Shutdown); assigned onto the chatbot App (Sessions adapter +
	// UserSessions) and into discord so they read the same state. One *Sessions
	// per chat provider is the multi-provider seam.
	sessions *users.Sessions

	// discordSession is set by startDiscord when the Discord bot is enabled
	// for this env; shutdown calls Stop on it to deregister the per-guild
	// slash commands. Nil when Discord stays gated off.
	discordSession *discord.Session

	// flagClient is the process-wide feature flag evaluator. Initialised to an
	// empty in-memory client so unknown keys evaluate to false during the brief
	// startup window before startFeatureFlags swaps in the Postgres-backed
	// client — same fail-closed contract as pkg/feature.
	flagClient feature.FlagClient

	// rotatorStore owns the console-edited corner-rotator copy in Postgres.
	// Retained past the /api/rotators wiring so the NATS on-connect callback can
	// republish every stored platform's copy, refilling the last-value cache
	// onscreens-server restores from. Nil until startRotatorEditing runs.
	rotatorStore *rotatorstore.Store

	// gateway is the HTTP client for the platform-gateway — the single Helix
	// caller. Non-nil on a Twitch instance (TWITCH_API_URL is
	// set); nil on a non-Twitch instance, which never reaches the Twitch Helix
	// paths (all gated behind platformIsTwitch). The shared client the
	// non-chatbot Helix callers (the OBS watchdog's live-check, the chat-send
	// path) route through.
	gateway *gateway.Client

	// locationFeed publishes the currently-playing clip's location, state, date,
	// weather, and sunset to the onscreens rotators on a timer — the values their
	// $variables resolve to, and on a stream where no command can reply the
	// passive stand-in for the !location / !date hints. Runs on every instance,
	// since an authored $variable can appear in any platform's copy.
	locationFeed *locationfeed.Emitter
}

// newGatewayClient builds the platform-gateway client when TWITCH_API_URL is
// set (a Twitch instance), else returns nil (a non-Twitch instance has no
// Twitch Helix surface). Stateless and side-effect free, so it's safe to
// construct at NewTripbot time.
func newGatewayClient(cfg *c.TripbotConfig) *gateway.Client {
	if cfg.TwitchAPIURL == "" {
		return nil
	}
	return gateway.New(cfg.TwitchAPIURL)
}

// NewTripbot constructs a Tripbot with default runtime state. Dependencies
// that need I/O or ordering (the IRC client, scheduler, Discord session,
// Postgres-backed flag client) are filled in by the boot-sequence methods.
func NewTripbot(version string, cfg *c.TripbotConfig) *Tripbot {
	t := &Tripbot{
		version: version,
		cfg:     cfg,
		app:     chatbot.New(cfg),
		srv:     server.New(cfg),
		player: video.NewPlayer(
			cfg,
			onscreensClient.New(natsclient.DefaultPublisher(), cfg.Environment, cfg.Platform),
			playoutClient.New(cfg.VlcServerHost, natsclient.DefaultPublisher(), cfg.Environment, cfg.Platform),
		),
		flagClient: feature.NewInMemoryClient(nil),
		gateway:    newGatewayClient(cfg),
	}
	// The audience source dispatches chatter refresh + the follower check to the
	// gateway (when the flag is on) or in-process; with no gateway wired it's the
	// plain in-process source. Reads t.gateway/t.flagClient lazily, so wiring it
	// here against the partially-built t is fine.
	t.sessions = users.New(t.cfg, gatewayChatterSource{t: t})
	// Feed the rotators what's playing, so their $variables resolve. Reuses the
	// chatbot's Geocoder and Weather adapters (the pkg/geo default is installed by
	// whichever Connect* path this platform takes).
	t.locationFeed = locationfeed.New(
		onscreensClient.New(natsclient.DefaultPublisher(), t.cfg.Environment, t.cfg.Platform),
		t.app.Geocoder,
		t.app.Weather,
	)
	return t
}

func main() {
	printBanner()
	NewTripbot(version, c.Load()).Run()
}

// printBanner prints the repo-root banner.txt if it's there. The file is a
// local-dev nicety and isn't copied into the container image, so an absent
// file is the ordinary production case and prints nothing — hence the ignored
// error.
func printBanner() {
	if art, err := os.ReadFile("banner.txt"); err == nil {
		fmt.Print(string(art))
	}
}

// platformIsTwitch reports whether this instance serves Twitch. Empty
// Platform is treated as Twitch, matching the chatbot registry's contract.
func (t *Tripbot) platformIsTwitch() bool {
	return t.cfg.Platform == "" || t.cfg.Platform == "twitch"
}

// Run performs the various steps to get the bot running. The spine —
// telemetry, HTTP server, player, sessions, cron, feature flags, NATS, the
// admin hub — is platform-neutral and runs on every instance; only the
// chat-transport bring-up swaps on STREAM_PLATFORM. Twitch-only steps (IRC
// token plumbing, EventSub, subscriber polling, the admin chat-send
// subscriber, Discord) are gated off non-Twitch instances.
func (t *Tripbot) Run() {
	slog.Info("tripbot starting", "version", t.version, "platform", t.cfg.Platform)
	// ctx is canceled on SIGINT/SIGTERM; every background goroutine hangs
	// off it and the HTTP server uses it to trigger its graceful drain.
	// When the blocking chat loop returns, t.shutdown runs the cleanup
	// sequence and the process exits 0 — there is no separate
	// signal-handler goroutine.
	ctx, flush := bootstrap.Start("tripbot", t.version, t.cfg)
	defer flush()
	t.srv.SetVersion(t.version)
	httpDone := t.startHttpServer(ctx)
	t.findInitialVideo()
	t.app.Video = chatbot.NewVideoAdapter(t.player)                         // commands read the same Player the cron refreshes
	t.app.Sessions = chatbot.NewSessionsAdapter(t.cfg.Platform, t.sessions) // command-time queries
	t.app.UserSessions = t.sessions                                         // inbound IRC handlers + access checks read the same session state
	t.sessions.InitLeaderboard(context.Background())
	t.startFeatureFlags(ctx)
	t.startRotatorEditing()
	if t.platformIsTwitch() {
		if t.gateway == nil {
			// There is no in-process Helix fallback, so audience polls, the
			// follower check, and broadcaster send have no backend here.
			// Real deploys always wire TWITCH_API_URL; this is local/CI.
			slog.WarnContext(ctx, "no TWITCH_API_URL: Twitch audience/follower/broadcaster-send features disabled (gateway not wired)")
		}
		t.loadTwitchToken(ctx) // must precede setUpTwitchClient — provides the IRC token
		t.setUpTwitchClient()  // required for the below
		t.updateSubscribers()
		t.getCurrentUsers()
		t.startEventSub(ctx)
	}
	// after setUpTwitchClient: the twitch.ReloadTokens job dereferences
	// t.irc, so cron registration waits until the client exists.
	t.startCron()
	t.startNATS(ctx)
	t.player.EmitCurrentVideo(ctx) // after startNATS: publishes the current video.changed for the standalone console
	if t.platformIsTwitch() {
		// after startNATS: the first auth.status snapshot for the standalone
		// console. The twitch.EmitAuthStatus cron job keeps it fresh.
		t.emitAuthStatus(ctx)
	}
	t.startOBSRefreshSubscriber(ctx) // after startNATS: per-platform (each instance owns its OBS)
	// Poll this instance's OBS WebSocket for streaming state + render/output
	// stats, stamping the series with the platform. These obs_* gauges feed
	// the stream-health dashboards and alerts.
	go obs.PollStreamingActive(ctx, t.cfg.Platform, 30*time.Second)
	t.startBackgroundAudio(ctx)         // every platform: owns its own OBS's bed
	t.startBackgroundAudioWatchdog(ctx) // recovers SomaFM outages; advances album tracks
	t.startStreamWatchdog(ctx)          // twitch + tiktok: recovers a stream the platform stopped showing
	if t.platformIsTwitch() {
		// chat.send subjects are per-env, not per-platform — both platform
		// instances would receive every admin send, so only the Twitch instance
		// (which owns the bot/broadcaster identities the command names)
		// subscribes.
		t.startChatSendSubscriber(ctx) // after startNATS + setUpTwitchClient: needs the conn and t.app.Chat
		t.startDiscord(ctx)            // Discord stays Twitch-side for v1
		t.connectToTwitch(ctx)         // blocks until shutdown
	} else {
		t.connectViaGateway(ctx, t.gatewayPlatform()) // blocks until shutdown
	}
	t.shutdown(httpDone)
}

// gatewayPlatform describes how one non-Twitch platform reaches its chat. All
// of them go through a platform-gateway service: outbound via its SendChat,
// inbound via its GET /v1/chat/inbound poll, so tripbot holds no credential for
// any of them. What differs is the URL, which outbound client gets installed,
// and whether the inbound poll runs at all.
type gatewayPlatform struct {
	name    string // platform slug, used in log messages
	envVar  string // config var holding the gateway URL; named in the error when unset
	apiURL  string // that var's value
	connect func() // installs the outbound chat client on the App

	// directions describes the platform's chat reach for the log line.
	// "inbound only" platforms have no post API — TikTok's webcast protocol is
	// observe-only, Instagram's Graph API can read live comments but not create
	// them — so their outbound client drops sends.
	directions string

	// reportsLiveness marks a platform whose chat poll is also its only
	// liveness signal, so the poll should write the live gauge. TikTok has no
	// broadcast-discovery tick, and the webcast room the gateway tracks for
	// chat is the same room viewers watch — so that poll is what catches a room
	// reaped out from under a healthy OBS push. Platforms with their own
	// discovery tick leave it off rather than have two writers fight over one
	// gauge.
	reportsLiveness bool

	// skipInbound turns the inbound poll off, leaving outbound and the
	// background jobs running. Only YouTube uses it: the poll is the expensive
	// YouTube Data API spend, and until the quota extension lands the instance
	// runs bot-less. inboundOffReason is logged in its place.
	skipInbound      bool
	inboundOffReason string
	inboundOffFix    string
}

// gatewayPlatform returns the descriptor for this instance's platform. An
// unrecognized platform falls through to youtube (empty PLATFORM never reaches
// here — platformIsTwitch claims it).
func (t *Tripbot) gatewayPlatform() gatewayPlatform {
	switch t.cfg.Platform {
	case "facebook":
		// The gateway owns the Page access token and the live-video resolution,
		// so outbound sends land as a Page comment on the live video.
		return gatewayPlatform{
			name: "facebook", envVar: "FACEBOOK_API_URL", apiURL: t.cfg.FacebookAPIURL,
			connect: t.app.ConnectFacebookViaGateway, directions: "inbound + outbound",
		}
	case "instagram":
		// The broadcast itself is started by a human — there is no API to go
		// live on Instagram — so the poller idles on rediscovery until one
		// appears.
		return gatewayPlatform{
			name: "instagram", envVar: "INSTAGRAM_API_URL", apiURL: t.cfg.InstagramAPIURL,
			connect: t.app.ConnectInstagramViaGateway, directions: "inbound only",
		}
	case "tiktok":
		return gatewayPlatform{
			name: "tiktok", envVar: "TIKTOK_API_URL", apiURL: t.cfg.TikTokAPIURL,
			connect: t.app.ConnectTikTokViaGateway, directions: "inbound only",
			reportsLiveness: true,
		}
	default:
		return gatewayPlatform{
			name: "youtube", envVar: "YOUTUBE_API_URL", apiURL: t.cfg.YouTubeAPIURL,
			connect: t.app.ConnectYouTubeViaGateway, directions: "inbound + outbound",
			skipInbound:      !t.cfg.YouTubeInboundEnabled,
			inboundOffReason: "youtube inbound chat disabled (bot-less mode); outbound + jobs only",
			inboundOffFix:    "set YOUTUBE_INBOUND_ENABLED=true to read chat",
		}
	}
}

// connectViaGateway brings up p's chat and blocks until shutdown.
//
// The gateway URL is required — without it there's no way to reach the platform
// — but a missing one is not fatal: the instance comes up Ready with everything
// else working and no chat, logging loudly. Same "stay up with limited
// functionality" contract as loadTwitchToken.
func (t *Tripbot) connectViaGateway(ctx context.Context, p gatewayPlatform) {
	if p.apiURL == "" {
		slog.ErrorContext(ctx, p.envVar+" unset; "+p.name+" chat disabled",
			"fix", "set "+p.envVar+" to the gateway-"+p.name+" service URL")
		<-ctx.Done()
		return
	}

	p.connect()

	if p.skipInbound {
		// Outbound posting (rotators) and the background jobs stay up, but no
		// command responds. The chatbot serves promo copy instead of command
		// ads (see enabledHelpMessages).
		slog.WarnContext(ctx, p.inboundOffReason, "gateway", p.apiURL, "fix", p.inboundOffFix)
	} else {
		poller := t.app.NewGatewayChatPoller(p.apiURL)
		if p.reportsLiveness {
			poller = poller.ReportsLiveness()
		}
		go poller.Run(ctx)
		slog.InfoContext(ctx, p.name+" chat via gateway ("+p.directions+")", "gateway", p.apiURL)
	}

	// nothing else to do on the main goroutine — the poller and HTTP server
	// run until shutdown begins.
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
	fc, err := feature.NewPostgresClient(ctx, database.GormDB(), featureFlagRefreshInterval, t.cfg.Platform)
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

// startRotatorEditing wires the console's /api/rotators surface: the Postgres
// store that owns the edited corner-rotator copy, and the NATS publisher that
// pushes a save to the platform's onscreens-server. Needs only the DB, so it
// runs before NATS is up; the stream-declare and republish happen on connect
// (see ensureRotatorCopyPublished).
//
// Without it the endpoints answer 503 and the overlays run the copy compiled
// into onscreens-server — copy editing must never be what keeps the bot down.
func (t *Tripbot) startRotatorEditing() {
	t.rotatorStore = rotatorstore.New(database.GormDB())
	t.srv.SetRotators(t.rotatorStore,
		onscreensClient.New(natsclient.DefaultPublisher(), t.cfg.Environment, t.cfg.Platform))
}

// ensureRotatorCopyPublished declares the rotator-copy last-value stream and
// republishes every stored platform's copy onto it. Runs from the NATS
// on-connect callback so it executes against a live server.
//
// The republish is the repair path for that cache: it rides a local-path PVC
// that `talosctl upgrade` wipes even with --preserve, so refilling it from the
// Postgres record of truth is what keeps a wipe from costing hand-authored copy.
// Best-effort per platform — a failure logs, and the next restart or console
// save covers it.
func (t *Tripbot) ensureRotatorCopyPublished(ctx context.Context) {
	if err := onscreensClient.EnsureRotatorConfigStream(ctx, natsclient.JetStream(), t.cfg.Environment); err != nil {
		slog.WarnContext(ctx, "rotator config stream setup failed; edits won't survive an onscreens restart",
			"err", err)
	}
	if t.rotatorStore == nil {
		return
	}
	platforms, err := t.rotatorStore.Platforms(ctx)
	if err != nil {
		slog.WarnContext(ctx, "couldn't list stored rotator copy to republish",
			"fix", "ensure migration 037_create_onscreens_rotators has run", "err", err)
		return
	}
	pub := onscreensClient.New(natsclient.DefaultPublisher(), t.cfg.Environment, t.cfg.Platform)
	for _, platform := range platforms {
		cfg, _, err := t.rotatorStore.GetOrDefault(ctx, platform)
		if err != nil {
			slog.WarnContext(ctx, "couldn't read stored rotator copy", "err", err, "platform", platform)
			continue
		}
		if err := pub.PublishRotatorConfig(ctx, platform, cfg); err != nil {
			slog.WarnContext(ctx, "couldn't republish rotator copy", "err", err, "platform", platform)
		}
	}
	if len(platforms) > 0 {
		slog.InfoContext(ctx, "republished stored rotator copy", "platforms", len(platforms))
	}
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
	natsclient.Connect(t.cfg.NatsURL, "tripbot", func(*nats.Conn) {
		if err := eventbus.EnsureStreams(ctx, natsclient.JetStream(), t.cfg.Environment); err != nil {
			slog.WarnContext(ctx, "jetstream stream setup failed; console will run without durable history",
				"err", err)
		}
		t.ensureRotatorCopyPublished(ctx)
	})
}

// authStatusInterval is how often the instance publishes its auth.status
// snapshot. Matches the in-process panel's 30s pollAuth cadence — token expiry
// moves on the order of minutes/hours, and the TRIPBOT_AUTH last-value stream
// means a freshly-connected console is at most one interval stale.
const authStatusInterval = 30 * time.Second

// emitAuthStatus publishes this instance's token state to
// tripbot.<env>.auth.status.twitch. The in-process admin hub ignores the
// subject (it polls token state directly); the standalone console is the
// consumer. Snapshots are assembled here — not in pkg/eventbus — so the
// eventbus stays free of pkg/twitch imports.
//
// Run once from Run, after NATS is up so the first snapshot isn't published
// into a connecting client, then on authStatusInterval as a cron job. Only the
// Twitch instance holds tokens: YouTube auth lives entirely on the
// platform-gateway (gateway-youtube owns the oauth_tokens youtube row), so a
// youtube instance has no token state to report and the job is Twitch-gated
// with the rest. (Surfacing YouTube auth status to the console is the gateway's
// job once it grows a NATS publisher — tracked separately.)
func (t *Tripbot) emitAuthStatus(ctx context.Context) {
	eventbus.EmitAuthStatus(ctx, t.cfg.Environment, "twitch", t.twitchAuthAccounts())
}

// twitchAuthAccounts converts the live Twitch token state (bot + broadcaster)
// into the eventbus wire shape.
func (t *Tripbot) twitchAuthAccounts() []eventbus.AuthAccount {
	statuses := mytwitch.TokenStatuses(t.cfg.BotUsername, t.cfg.ChannelName)
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

// startStreamWatchdog launches the goroutine that detects the silent
// disconnect — OBS reporting outputActive=true while the platform reports the
// channel offline — and forces recovery after several consecutive
// minute-spaced misalignments. First seen in prod on Twitch 2026-05-27, ~30h
// into an OBS session.
//
// Both live-checks route through the platform-gateway. That wiring lives here
// in cmd rather than in pkg/obs/watchdog, so the shared package takes no
// binary-specific dependency.
//
// Platforms whose broadcast is a set-and-forget ingest key have nothing to
// recover from outside OBS and get no watchdog.
func (t *Tripbot) startStreamWatchdog(ctx context.Context) {
	switch {
	case t.platformIsTwitch():
		t.startTwitchWatchdog(ctx)
	case t.cfg.Platform == "tiktok":
		t.startTikTokWatchdog(ctx)
	}
}

// startTwitchWatchdog recovers the half-open RTMP socket: Twitch's ingest
// closed the session without the FIN/RST reaching OBS, so a StopStream +
// StartStream opens a fresh connection.
func (t *Tripbot) startTwitchWatchdog(ctx context.Context) {
	deps := watchdog.DefaultWatchdogDeps()
	// A nil gateway is a misconfigured Twitch instance (TWITCH_API_URL unset) —
	// report the check as errored rather than force-restarting on a false
	// negative.
	deps.ChannelLive = func(ctx context.Context) (bool, error) {
		if t.gateway == nil {
			return false, errors.New("watchdog live-check: no gateway configured")
		}
		live, err := t.gateway.IsLive(ctx, t.cfg.ChannelName)
		if err == nil {
			instrumentation.TwitchChannelLive.Set(live)
			instrumentation.ChannelLive.Set(live, t.cfg.Platform)
		}
		return live, err
	}
	go watchdog.WatchSilentDisconnect(ctx, deps, 60*time.Second, 3, 10*time.Minute)
}

// startTikTokWatchdog recovers a reaped LIVE room. TikTok's failure is one
// layer above Twitch's: the Streamlabs-minted room is gone once a push gap
// outlives the relay target's idleTimeout, and reconnecting OBS's push into a
// dead room changes nothing — the room has to be re-minted through the gateway,
// which binds a fresh portrait relay target, and the push re-opened onto it.
//
// Slower to fire and slower to repeat than the Twitch watchdog: a re-mint
// costs a brand-new LIVE (viewers have to rejoin), so it waits out five
// consecutive misses rather than three, and holds a 30m cooldown — which the
// watchdog retires once a re-mint has held for five ticks, so the long timer
// only ever suppresses re-mints that aren't taking.
func (t *Tripbot) startTikTokWatchdog(ctx context.Context) {
	if t.cfg.TikTokAPIURL == "" {
		slog.WarnContext(ctx, "no TIKTOK_API_URL: silent-disconnect watchdog disabled (gateway not wired)")
		return
	}
	gw := gateway.New(t.cfg.TikTokAPIURL)
	deps := watchdog.DefaultWatchdogDeps()
	// No gauge write here: the inbound chat poll already owns
	// tripbot_channel_live for TikTok (gatewayPlatform.reportsLiveness), and two
	// writers on one gauge would fight.
	deps.ChannelLive = func(ctx context.Context) (bool, error) {
		return gw.IsLive(ctx, t.cfg.ChannelName)
	}
	deps.Restart = func(ctx context.Context) error {
		return remintTikTokEgress(ctx, gw, tiktokRemintGap, watchdog.RestartOBSOutput)
	}
	go watchdog.WatchSilentDisconnect(ctx, deps, 60*time.Second, 5, 30*time.Minute)
}

// tiktokRemintGap lets the old relay target unbind before the new room binds
// its replacement. Mirrors the settle pause in the OBS restart next door.
const tiktokRemintGap = 5 * time.Second

// remintTikTokEgress stops the gateway's egress, starts a fresh one after gap,
// then restarts the OBS output so the push lands on the target the new room
// bound.
//
// The bounce is the load-bearing step. A push already in flight is not moved
// onto a target that binds under it: the relay keeps accepting frames and
// forwards them at the target that was unbound, so the fresh room sits at
// "LIVE will begin shortly" with no source while the gateway reports it live —
// a state nothing else detects (2026-07-29). Only an RTMP session opened after
// the bind reaches the new room.
func remintTikTokEgress(ctx context.Context, gw *gateway.Client, gap time.Duration, restartOBS func(context.Context) error) error {
	if err := gw.StopEgress(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(gap):
	}
	if err := gw.StartEgress(ctx); err != nil {
		return err
	}
	return restartOBS(ctx)
}

// startBackgroundAudio constructs this instance's bed store and hands it to the
// console API. The seed bed mirrors the OBS entrypoint's per-platform default;
// Detect then reads the source's real settings so a tripbot restart reports
// what's actually playing rather than the default. Detect dials OBS, so it runs
// in the background — a slow or absent OBS must not hold up startup.
func (t *Tripbot) startBackgroundAudio(ctx context.Context) {
	seed := beds.CarHum
	if t.platformIsTwitch() {
		seed = beds.SomaFM
	}
	t.beds = beds.NewStore(beds.RealOBS{}, seed, "", t.cfg.Platform)
	t.srv.SetBeds(t.beds) // the console's /api/audio reads + switches through it
	t.app.Beds = t.beds   // and !audio, so chat and console report the same bed
	go t.beds.Detect(ctx)
}

// startBackgroundAudioWatchdog launches the volume-meter connection + the
// background-audio watchdog. It does two jobs, both tied to the single
// "Background Audio" source: it keeps audible music on the stream when SomaFM
// drops (swapping onto the local Car Hum bed and back once SomaFM recovers —
// first needed in prod on 2026-06-23, when a full SomaFM edge outage left the
// stream silent with no self-heal), and it advances the album to its next track
// when OBS reports the current one ended.
//
// Runs on every platform: any platform can select any bed, so neither job
// is Twitch-specific. On the car-hum bed it only records the audio gauges.
func (t *Tripbot) startBackgroundAudioWatchdog(ctx context.Context) {
	meter := audiowatchdog.NewVolumeMeter(audiowatchdog.BackgroundAudioInputName, 30*time.Second)
	go meter.Run(ctx)
	go audiowatchdog.Watch(ctx, audiowatchdog.DefaultDeps(meter, t.beds), audiowatchdog.DefaultConfig())
}

// startDiscord brings up the bot's Discord slash-command session when
// the env supplies the required config and the discord.bot_enabled feature
// flag is on. Every failure path here logs and returns so it can't block
// (or crash) tripbot startup — Discord is additive to the core IRC /
// EventSub paths.
func (t *Tripbot) startDiscord(ctx context.Context) {
	if ok, reason := discord.ShouldStart(t.cfg); !ok {
		slog.InfoContext(ctx, "discord disabled", "reason", reason)
		return
	}
	if !t.flagClient.Bool(ctx, discord.FlagKey, feature.EvalContext{Env: t.cfg.Environment}) {
		slog.InfoContext(ctx, "discord disabled by feature flag", "flag", discord.FlagKey)
		return
	}
	session, err := discord.New(t.cfg, t.sessions)
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
			"login_as", t.cfg.ChannelName)
		return
	}
	if mytwitch.ChannelID() == "" && t.gateway != nil {
		// The gateway owns Helix, so nothing populates channelID in-process.
		// Resolve it via the gateway's /v1/users/{login} so EventSub gets a
		// BroadcasterUserID. Non-fatal — falls through to the skip below on error.
		if id, err := t.gateway.UserID(ctx, t.cfg.ChannelName); err != nil {
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
		for {
			if ctx.Err() != nil {
				return
			}
			err := eventsub.Run(ctx, eventsub.Config{
				ClientID:          t.cfg.TwitchClientID,
				BroadcasterToken:  token,
				BroadcasterUserID: mytwitch.ChannelID(),
			}, eventsub.Handlers{
				OnFollow:      t.app.AnnounceNewFollower,
				OnSubscribe:   t.app.AnnounceSubscriber,
				OnUnsubscribe: t.app.RecordUnsubscribe,
				OnGift:        t.app.AnnounceGiftSub,
				OnResub:       t.app.AnnounceResub,
			})
			if err == nil || errors.Is(err, context.Canceled) {
				return
			}
			if errors.Is(err, eventsub.ErrUnauthorized) {
				// The token is loaded once above, so every redial would repeat
				// this rejection — roughly three a minute, since Twitch drops a
				// subscription-less session after ~10s. Stop and say so; the
				// recovery is a broadcaster re-consent and a restart.
				slog.ErrorContext(ctx, "eventsub disabled: broadcaster token rejected — re-consent via the platform-gateway flow (surfaced in tripbot-console), then restart", "err", err)
				return
			}
			// Twitch closing the socket outright surfaces here instead of as a
			// session_reconnect frame the library handles itself, so Run has to
			// be redialed or follower/sub announcements stay dead until the pod
			// restarts. Run resubscribes on the new session's welcome.
			slog.WarnContext(ctx, "eventsub run terminated; reconnecting", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()
}

// startHttpServer starts a webserver, which is used for admin tools and
// receiving webhooks, in a goroutine. The passed context is honored by the
// server for graceful shutdown — when it's canceled, the server stops
// accepting new connections and drains in-flight requests up to its
// shutdown timeout. The returned channel closes once that drain completes;
// t.shutdown waits on it so the process doesn't exit mid-drain.
func (t *Tripbot) startHttpServer(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		t.srv.Start(ctx)
		close(done)
	}()
	return done
}

// findInitialVideo determines the video that is currently playing. Run it
// early, otherwise it stays unset until the first cron job runs.
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
	s, err := gocron.NewScheduler()
	if err != nil {
		slog.Error("error creating background scheduler", "err", err)
		os.Exit(1)
	}
	t.scheduler = s
	slog.Info("starting cron")
	t.scheduler.Start()
	// let !shutdown stop the same scheduler instance (gocron.Scheduler
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
	if err := mytwitch.LoadFromDB(t.cfg.BotUsername, t.cfg.ChannelName); err != nil {
		slog.WarnContext(ctx, "no usable Twitch token at boot; starting without a chat connection and polling",
			"login_as", t.cfg.BotUsername,
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
			if err := mytwitch.LoadFromDB(t.cfg.BotUsername, t.cfg.ChannelName); err != nil {
				if time.Since(lastLogged) >= logEvery {
					slog.WarnContext(ctx, "still waiting for Twitch token (re-auth via the platform-gateway consent flow, surfaced in tripbot-console)",
						"login_as", t.cfg.BotUsername, "err", err)
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
// setUpTwitchClient installs the static app credentials and builds the Twitch
// IRC client, wiring the App's inbound adapters to it.
//
// The credentials are required, and fatal when absent: unlike a missing gateway
// URL — where the instance stays up serving everything but that platform's chat
// — there is no useful Twitch instance without them. Nothing outside this
// twitch-only path needs them, which is why the check lives here rather than in
// config.Load or a package init.
// missingTwitchCredentials names the static Twitch app credentials that aren't
// set, in a stable order. TWITCH_AUTH_TOKEN is deliberately absent: the IRC
// token lives in the oauth_tokens table and is loaded via LoadFromDB.
func missingTwitchCredentials(cfg *c.TripbotConfig) []string {
	var missing []string
	for _, cred := range []struct{ name, value string }{
		{"TWITCH_CLIENT_ID", cfg.TwitchClientID},
		{"TWITCH_CLIENT_SECRET", cfg.TwitchClientSecret},
	} {
		if cred.value == "" {
			missing = append(missing, cred.name)
		}
	}
	return missing
}

func (t *Tripbot) setUpTwitchClient() {
	for _, name := range missingTwitchCredentials(t.cfg) {
		log.Fatalf("You must set %s", name)
	}
	mytwitch.SetCredentials(t.cfg.TwitchClientID, t.cfg.TwitchClientSecret)

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

// connectToTwitch joins Twitch chat and listens until ctx cancels.
func (t *Tripbot) connectToTwitch(ctx context.Context) {
	t.irc.Join(t.cfg.ChannelName)
	slog.Info("joined channel", "channel", t.cfg.ChannelName, "url", fmt.Sprintf("https://twitch.tv/%s", t.cfg.ChannelName))

	// Mark the bot connected to chat once the IRC connection is established.
	// This drives the tripbot_twitch_connected gauge — it does NOT gate
	// /health/ready, which stays 200 so the pod keeps serving /auth/* and the
	// console-facing /api/* endpoints even while the bot is offline.
	t.irc.OnConnect(func() {
		slog.Info("connected to Twitch chat")
		instrumentation.TwitchConnection.Set(true)
	})

	// Disconnect the IRC client when shutdown begins so the blocking
	// Connect below returns and the loop can exit.
	go func() {
		<-ctx.Done()
		if err := t.irc.Disconnect(); err != nil {
			slog.Debug("irc disconnect on shutdown", "err", err)
		}
	}()

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		slog.Info("initializing connection to Twitch")
		// Connect blocks while connected and returns when the connection
		// drops; mark not-in-chat so the gauge reflects the gap until the next
		// OnConnect fires.
		err := t.irc.Connect()
		instrumentation.TwitchConnection.Set(false)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("unable to connect to twitch", "err", err)
			if errors.Is(err, twitch.ErrLoginAuthenticationFailed) {
				// The IRC client's token was rejected. Re-read the bot row from
				// oauth_tokens — the platform-gateway keeps it fresh, so a token
				// it just rotated (or one auth-bootstrap wrote) is picked up
				// without a restart. Then sync whatever's now in memory into the
				// IRC client for the next Connect attempt.
				if err := mytwitch.LoadFromDB(t.cfg.BotUsername, t.cfg.ChannelName); err != nil {
					slog.Warn("IRC auth failed; re-reading oauth_tokens failed", "err", err, "login_as", t.cfg.BotUsername)
				}
				if tok := mytwitch.IRCAuthToken(); tok != "" {
					t.irc.SetIRCToken(tok)
				} else {
					// No usable token in the DB yet (e.g. the row is unseeded).
					// Re-auth runs through the platform-gateway consent flow now
					// (surfaced in tripbot-console); the gateway writes the row.
					slog.Error("IRC auth failed and no valid token in oauth_tokens; re-auth via the platform-gateway consent flow (surfaced in tripbot-console)", "login_as", t.cfg.BotUsername)
				}
			}
			// retry after a minute, or bail if shutdown starts meanwhile
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute):
			}
		}
	}
}

// shutdown runs the cleanup sequence once the blocking chat loop returns:
// stop the cron scheduler (no new ticks), stop Discord, flush session
// state to the still-open DB, wait for the HTTP drain, then close the DB.
// Sentry and telemetry flush afterwards, in bootstrap's deferred flush.
func (t *Tripbot) shutdown(httpDone <-chan struct{}) {
	slog.Warn("shutting down")
	//TODO: print different message if CurrentlyPlaying is ""
	slog.Info("last played video", "file", t.player.Current().File())
	// Shutdown cancels in-flight job contexts, so any ctx-aware work in those
	// jobs unwinds rather than running to completion. Cron jobs here are short
	// idempotent ticks that retry on the next interval, so losing an in-flight
	// execution is fine. Nil until startCron runs, so a signal arriving during
	// boot doesn't panic the cleanup path.
	if t.scheduler != nil {
		slog.Info("stopping cron")
		if err := t.scheduler.Shutdown(); err != nil {
			slog.Error("error shutting down gocron scheduler", "err", err)
		}
	}
	if t.discordSession != nil {
		if err := t.discordSession.Stop(); err != nil {
			slog.Error("discord stop failed", "err", err)
		}
	}
	t.sessions.Shutdown(context.Background())
	<-httpDone
	if err := database.Close(); err != nil {
		slog.Error("error closing DB connection", "err", err)
	}
}

// scheduleBackgroundJobs schedules the various background jobs.
// Lives in this package (not pkg/background) to avoid circular deps with
// the job-target packages.
func (t *Tripbot) scheduleBackgroundJobs() {
	// platform-neutral jobs: every instance plays video and posts the
	// periodic help message.
	t.addJob(60*time.Second, "video.GetCurrentlyPlaying", t.player.GetCurrentlyPlaying)
	t.addJob(2*time.Hour+57*time.Minute+30*time.Second, "chatbot.Chatter", t.app.Chatter)
	// Refresh the rotators' clip-data feed every minute. Re-publishing (not just
	// on video change) also recovers a restarted onscreens-server within a tick;
	// the geocode and weather lookups are throttled inside Emit.
	t.addJob(60*time.Second, "video.LocationFeed", func(ctx context.Context) {
		t.locationFeed.Emit(ctx, t.player.Current())
	})

	// YouTube instances (bot-less or full): discover the current broadcast's
	// videoId on a slow ticker and publish it for the console, which links to and
	// embeds the broadcast directly. Needed because an unlisted broadcast's
	// channel/handle "/live" redirect only resolves a public stream. One quota
	// unit per poll — negligible even against prod's constrained quota — and it
	// runs regardless of YOUTUBE_INBOUND_ENABLED (discovery is not the chat read).
	// WithStartImmediately so a fresh console sees the link without a full
	// interval's wait; the last-value cache then retains it.
	if !t.platformIsTwitch() && t.cfg.YouTubeAPIURL != "" {
		ytGateway := gateway.New(t.cfg.YouTubeAPIURL)
		t.addJob(2*time.Minute, "youtube.BroadcastDiscovery", func(ctx context.Context) {
			b, err := ytGateway.ActiveBroadcast(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "youtube broadcast discovery failed", "err", err)
				return
			}
			eventbus.EmitYoutubeBroadcast(ctx, t.cfg.Environment, b.VideoID, b.Privacy, b.Live)
			// This tick is YouTube's liveness source rather than the inbound chat
			// poll: it runs whether or not chat is enabled, and it reports an
			// active broadcast as live even when that broadcast has no live chat.
			instrumentation.ChannelLive.Set(b.Live, t.cfg.Platform)
		}, gocron.WithStartAt(gocron.WithStartImmediately()))
	}

	// The facebook analog of the youtube ticker above: snapshot the Page's
	// current broadcast (video id + public/unpublished privacy) so the
	// console can badge and link an unpublished rehearsal, which the Page
	// timeline never shows.
	if !t.platformIsTwitch() && t.cfg.FacebookAPIURL != "" {
		fbGateway := gateway.New(t.cfg.FacebookAPIURL)
		t.addJob(2*time.Minute, "facebook.BroadcastDiscovery", func(ctx context.Context) {
			b, err := fbGateway.ActiveBroadcast(ctx)
			if err != nil {
				slog.ErrorContext(ctx, "facebook broadcast discovery failed", "err", err)
				return
			}
			eventbus.EmitFacebookBroadcast(ctx, t.cfg.Environment, b.VideoID, b.BroadcastID, b.PermalinkURL, b.Privacy, b.Live)
			instrumentation.ChannelLive.Set(b.Live, t.cfg.Platform)
		}, gocron.WithStartAt(gocron.WithStartImmediately()))
	}

	if !t.platformIsTwitch() {
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
	t.addJob(5*time.Minute, "rollups.Reconcile", func(ctx context.Context) { rollups.Reconcile(ctx, t.cfg) },
		gocron.WithSingletonMode(gocron.LimitModeReschedule))
	t.addJob(5*time.Minute, "chatbot.ShowRotatingLeaderboard", t.app.ShowRotatingLeaderboard)
	t.addJob(5*time.Minute, "users.PrintCurrentSession", t.sessions.PrintCurrentSession)
	t.addJob(5*time.Minute, "twitch.GetSubscribers", t.refreshSubscribers)
	t.addJob(5*time.Minute, "twitch.GetFollowerCount", t.refreshFollowerCount)
	// Run publishes the first snapshot once NATS is connected; this keeps it
	// fresh. Registered here rather than as its own goroutine so a panic in the
	// token read is recovered and each publish gets a span and duration metric.
	t.addJob(authStatusInterval, "twitch.EmitAuthStatus", t.emitAuthStatus)
	// The platform-gateway owns token refresh now; tripbot only reads the rows
	// it keeps fresh. Re-read on a timer so the in-memory tokens track the
	// gateway's rotations — the IRC PASS line on reconnect and the token-expiry
	// gauge (both fed by LoadFromDB) — without tripbot ever refreshing itself.
	t.addJob(5*time.Minute, "twitch.ReloadTokens", func(ctx context.Context) {
		if err := mytwitch.LoadFromDB(t.cfg.BotUsername, t.cfg.ChannelName); err != nil {
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
//
// name reaches gocron itself via WithName, not just the span and the error log,
// so the scheduler can be asked what it holds — which is how the platform gates
// in scheduleBackgroundJobs are tested without running a single tick.
func (t *Tripbot) addJob(interval time.Duration, name string, fn func(context.Context), opts ...gocron.JobOption) {
	_, err := t.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(tracedJob(name, fn)),
		append([]gocron.JobOption{gocron.WithName(name)}, opts...)...,
	)
	if err != nil {
		slog.Error("error adding background job: "+name, "err", err)
	}
}
