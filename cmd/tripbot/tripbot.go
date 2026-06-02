package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adanalife/tripbot/pkg/background"
	"github.com/adanalife/tripbot/pkg/chatbot"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/discord"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/eventsub"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/natsclient"
	"github.com/adanalife/tripbot/pkg/obs/watchdog"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
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
// is explicit and the deps that used to be package-level globals are fields.
type Tripbot struct {
	version string

	// irc is the go-twitch-irc client, constructed by setUpTwitchClient
	// (chatbot.Initialize) and shared by connectToTwitch, pollForTwitchToken
	// and the token-refresh cron job (SetIRCToken).
	irc *twitch.Client

	// scheduler is the background cron scheduler, constructed in startCron and
	// shared by scheduleBackgroundJobs (job registration) and gracefulShutdown
	// (Stop). Also installed into chatbot via chatbot.SetScheduler so !shutdown
	// can stop it.
	scheduler *background.Scheduler

	// srv is the admin-panel / auth / metrics HTTP server, constructed in
	// NewTripbot. cmd installs runtime state through it (SetVersion,
	// SetFlagClient, SetTwitchConnected) and starts it (Start, StartEventHub);
	// the panel's handlers are methods on this instance.
	srv *server.Server

	// player owns "what's currently playing" — the single process-wide
	// instance, constructed in NewTripbot. The 60s cron tick refreshes it
	// (GetCurrentlyPlaying); findInitialVideo + gracefulShutdown read it; it's
	// installed into chatbot (SetVideoPlayer) so commands read the same state,
	// and it publishes video.changed to NATS for the admin panel.
	player *video.Player

	// sessions tracks who's currently in chat (the login map) + the
	// lifetime-miles leaderboard — the single process-wide instance,
	// constructed in NewTripbot. Cron jobs refresh it (UpdateSession /
	// UpdateLeaderboard); boot hydrates it (InitLeaderboard); gracefulShutdown
	// flushes it (Shutdown); installed into chatbot (SetSessions) + discord so
	// they read the same state. One *Sessions per chat provider is the
	// multi-provider seam.
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
}

// NewTripbot constructs a Tripbot with default runtime state. Dependencies
// that need I/O or ordering (the IRC client, scheduler, Discord session,
// Postgres-backed flag client) are filled in by the boot-sequence methods.
func NewTripbot(version string) *Tripbot {
	return &Tripbot{
		version: version,
		srv:     server.New(),
		player: video.NewPlayer(
			onscreensClient.New(c.Conf.OnscreensServerHost, natsclient.DefaultPublisher(), c.Conf.Environment),
			vlcClient.New(c.Conf.VlcServerHost),
		),
		sessions:   users.NewDefault(),
		flagClient: feature.NewInMemoryClient(nil),
	}
}

func main() {
	NewTripbot(version).Run()
}

// Run performs the various steps to get the bot running.
func (t *Tripbot) Run() {
	slog.Info("tripbot starting", "version", t.version)
	createRandomSeed()
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
	chatbot.SetVideoPlayer(t.player) // commands read the same Player the cron refreshes
	chatbot.SetSessions(t.sessions)  // commands + IRC handlers read the same session state
	t.sessions.InitLeaderboard(context.Background())
	t.startCron()
	t.startFeatureFlags(shutdownCtx)
	t.loadTwitchToken(shutdownCtx)           // must precede chatbot.Initialize — provides the IRC token
	t.refreshTokensIfNearExpiry(shutdownCtx) // closes the restart-desync gap with the hourly cron
	t.setUpTwitchClient()                    // required for the below
	t.updateSubscribers()
	t.getCurrentUsers()
	t.startEventSub(shutdownCtx)
	t.startNATS()
	t.srv.StartEventHub(shutdownCtx)       // after startNATS: the hub subscribes to the live NATS conn
	t.player.EmitCurrentVideo(shutdownCtx) // after the hub subscribes: seed its now-playing cache (no NATS replay)
	t.startDiscord(shutdownCtx)
	t.startSilentDisconnectWatchdog(shutdownCtx)
	t.connectToTwitch()
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
	fc, err := feature.NewPostgresClient(ctx, database.GormDB(), featureFlagRefreshInterval)
	if err != nil {
		slog.WarnContext(ctx, "feature flag client init failed; flags will default to off",
			"fix", "ensure migration 013_create_feature_flags has run",
			"err", err)
		return
	}
	t.flagClient = fc
	chatbot.SetFlagClient(fc)
	t.srv.SetFlagClient(fc)
	go fc.Start(ctx)
}

// startNATS connects to the in-cluster NATS broker (phase 1 of the
// pubsub migration). Optional — when NATS_URL is empty the connection
// is skipped and publishes no-op silently; chatbot.realOnscreens.
// ShowMiddleText still mirrors to NATS but the publish becomes a nil
// check, leaving HTTP as the sole transport.
func (t *Tripbot) startNATS() {
	natsclient.Connect(c.Conf.NatsURL, "tripbot")
}

// startSilentDisconnectWatchdog launches the goroutine that detects the
// half-open RTMP state where OBS reports outputActive=true but Twitch's
// API shows the channel offline, and force-restarts the stream after
// 3 consecutive minute-spaced misalignments. First seen in prod on
// 2026-05-27, ~30h into an OBS session.
func (t *Tripbot) startSilentDisconnectWatchdog(ctx context.Context) {
	go watchdog.WatchSilentDisconnect(ctx, watchdog.DefaultWatchdogDeps(), 60*time.Second, 3, 10*time.Minute)
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
		slog.WarnContext(ctx, "skipping eventsub: no broadcaster oauth_tokens row; bootstrap with `task tripbot:auth:bootstrap:broadcaster`",
			"login_as", c.Conf.ChannelName,
			"reauth_url", mytwitch.AuthInitURL("broadcaster"))
		return
	}
	if mytwitch.ChannelID() == "" {
		// getChannelID is lazy on first call; calling GetSubscribers /
		// GetFollowerCount typically populates it. updateSubscribers()
		// above already ran, so this is belt-and-suspenders.
		slog.WarnContext(ctx, "skipping eventsub: ChannelID not yet resolved")
		return
	}
	go func() {
		err := eventsub.Run(ctx, eventsub.Config{
			ClientID:          mytwitch.ClientID,
			BroadcasterToken:  token,
			BroadcasterUserID: mytwitch.ChannelID(),
		}, eventsub.Handlers{
			OnFollow:    chatbot.AnnounceNewFollower,
			OnSubscribe: chatbot.AnnounceSubscriber,
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.ErrorContext(ctx, "eventsub run terminated", "err", err)
		}
	}()
}

// createRandomSeed ensures that random numbers will be random
func createRandomSeed() {
	// create a brand new random seed
	rand.Seed(time.Now().UnixNano())
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
	// let !shutdown stop the same scheduler instance
	chatbot.SetScheduler(t.scheduler)
	t.scheduleBackgroundJobs()
}

// loadTwitchToken pulls the bot's OAuth row from the oauth_tokens table.
// Non-fatal: when the row is missing (e.g. auth-bootstrap hasn't run yet
// against a freshly-restored DB) or the DB is briefly unreachable, the bot
// comes up with limited functionality and polls in the background until the
// token lands — rather than crashlooping. A crashing pod after a wipe also
// raced the DB restore's migrate init (see the 2026-05-20 convergence-wipe
// notes); staying up avoids that. The pod stays Ready throughout (readiness
// no longer gates on Twitch) so the admin panel + /auth/init are reachable
// to re-auth; "not in chat" is surfaced via the admin panel + the
// tripbot_twitch_connected gauge instead.
func (t *Tripbot) loadTwitchToken(ctx context.Context) {
	if err := mytwitch.LoadFromDB(); err != nil {
		slog.WarnContext(ctx, "no usable Twitch token at boot; starting without a chat connection and polling",
			"login_as", c.Conf.BotUsername,
			"fix", "task tripbot:auth:bootstrap",
			"reauth_url", mytwitch.AuthInitURL("bot"),
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
					slog.WarnContext(ctx, "still waiting for Twitch token",
						"login_as", c.Conf.BotUsername,
						"reauth_url", mytwitch.AuthInitURL("bot"), "err", err)
					lastLogged = time.Now()
				}
				continue
			}
			slog.InfoContext(ctx, "Twitch token loaded; bot will connect on next attempt")
			// Push the freshly-loaded token into the (already-constructed)
			// IRC client so the connect loop's next try uses it instead of
			// the empty token captured at chatbot.Initialize.
			if tok := mytwitch.IRCAuthToken(); tok != "" && t.irc != nil {
				t.irc.SetIRCToken(tok)
			}
			return
		}
	}
}

// refreshTokensIfNearExpiry runs the same refresh that the hourly cron does,
// once, synchronously, at startup. gocron's DurationJob fires its first tick
// one full interval after Scheduler.Start(), so a pod restart that lands
// within the refresh window (or after expiry) would otherwise leave the
// in-memory token stale until the cron catches up — up to an hour. refreshOne
// early-returns when the stored token is healthy, so this is a no-op in the
// common case.
func (t *Tripbot) refreshTokensIfNearExpiry(ctx context.Context) {
	mytwitch.RefreshUserAccessToken(ctx)
}

// setUpTwitchClient sets up the Twitch client,
// used by many bot features
func (t *Tripbot) setUpTwitchClient() {
	// set up the Twitch client
	t.irc = chatbot.Initialize()
}

// updateSubscribers gets the list of current subscribers
func (t *Tripbot) updateSubscribers() {
	// update subscribers list
	mytwitch.GetSubscribers(context.Background())
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
	// This drives the admin-panel status row + the tripbot_twitch_connected
	// gauge — it does NOT gate /health/ready, which stays 200 so the pod keeps
	// serving the admin panel + /auth/* even while the bot is offline.
	t.irc.OnConnect(func() {
		slog.Info("connected to Twitch chat")
		t.srv.SetTwitchConnected(true)
	})

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		slog.Info("initializing connection to Twitch")
		// Connect blocks while connected and returns when the connection
		// drops; mark not-in-chat so the admin panel + gauge reflect the gap
		// until the next OnConnect fires.
		err := t.irc.Connect()
		t.srv.SetTwitchConnected(false)
		if err != nil {
			slog.Error("unable to connect to twitch", "err", err)
			if errors.Is(err, twitch.ErrLoginAuthenticationFailed) {
				// The IRC client's token was rejected. Re-establish the bot
				// token from the DB (forced refresh, then re-read the row) so a
				// token just written by auth-bootstrap is picked up without a
				// restart — the common case after a DB restore carries a stale
				// row. Then sync whatever's now in memory into the IRC client
				// for the next Connect attempt.
				mytwitch.Reauth(context.Background(), "bot")
				if tok := mytwitch.IRCAuthToken(); tok != "" {
					t.irc.SetIRCToken(tok)
				} else {
					// Reauth couldn't produce a token (refresh_token revoked and
					// no fresh row in the DB yet). Surface the re-bootstrap link
					// so re-auth is a click; the admin panel shows it too.
					slog.Error("IRC auth failed and no valid token after reauth; re-bootstrap needed", "login_as", c.Conf.BotUsername, "reauth_url", mytwitch.AuthInitURL("bot"))
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
	// anything below this probably won't be executed
	// try and use !shutdown instead
	//TODO: print different message if CurrentlyPlaying is ""
	slog.Info("last played video", "file", t.player.Current().File())
	if t.discordSession != nil {
		if err := t.discordSession.Stop(); err != nil {
			slog.Error("discord stop failed", "err", err)
		}
	}
	t.sessions.Shutdown(context.Background())
	err := database.Connection().Close()
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
	onscreensCli := onscreensClient.New(c.Conf.OnscreensServerHost, natsclient.DefaultPublisher(), c.Conf.Environment)
	t.addJob(60*time.Second, "video.GetCurrentlyPlaying", t.player.GetCurrentlyPlaying)
	t.addJob(61*time.Second, "users.UpdateSession", t.sessions.UpdateSession)
	t.addJob(62*time.Second, "users.UpdateLeaderboard", t.sessions.UpdateLeaderboard)
	t.addJob(5*time.Minute, "onscreens.ShowGuessLeaderboard", onscreensCli.ShowGuessLeaderboard)
	t.addJob(5*time.Minute, "users.PrintCurrentSession", t.sessions.PrintCurrentSession)
	t.addJob(5*time.Minute, "twitch.GetSubscribers", mytwitch.GetSubscribers)
	t.addJob(5*time.Minute, "twitch.GetFollowerCount", mytwitch.GetFollowerCount)
	t.addJob(1*time.Hour, "twitch.RefreshUserAccessToken", func(ctx context.Context) {
		mytwitch.RefreshUserAccessToken(ctx)
		// Keep the IRC client's stored token in sync with the rotated credentials.
		// go-twitch-irc captures the token at construction; without this, any
		// reconnect after the first rotation replays the original boot-time token.
		if tok := mytwitch.IRCAuthToken(); tok != "" {
			t.irc.SetIRCToken(tok)
		}
	})
	t.addJob(2*time.Hour+57*time.Minute+30*time.Second, "chatbot.Chatter", chatbot.Chatter)
}

// addJob registers a gocron job at the given interval, wrapping fn with
// tracedJob so each tick opens a span and centralising the error logging.
func (t *Tripbot) addJob(interval time.Duration, name string, fn func(context.Context)) {
	_, err := t.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(tracedJob(name, fn)),
	)
	if err != nil {
		slog.Error("error adding background job: "+name, "err", err)
	}
}
