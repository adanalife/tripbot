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
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/eventsub"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	"github.com/adanalife/tripbot/pkg/server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
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

var client *twitch.Client

var telemetryShutdown telemetry.ShutdownFunc

// main performs the various steps to get the bot running
func main() {
	slog.Info("tripbot starting", "version", version)
	createRandomSeed()
	// shutdownCtx is canceled on SIGINT/SIGTERM; the HTTP server uses it
	// to trigger a graceful shutdown so in-flight requests aren't cut.
	// listenForShutdown's gracefulShutdown goroutine handles the rest of
	// the app cleanup off the same signals.
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	listenForShutdown()
	initializeTelemetry()
	initializeErrorLogger()
	server.SetVersion(version)
	startHttpServer(shutdownCtx)
	findInitialVideo()
	users.InitLeaderboard(context.Background())
	startCron()
	loadTwitchToken()   // must precede chatbot.Initialize — provides the IRC token
	setUpTwitchClient() // required for the below
	updateSubscribers()
	getCurrentUsers()
	startEventSub(shutdownCtx)
	connectToTwitch()
}

// startEventSub kicks off the EventSub WebSocket listener in a goroutine
// so real-time follow/subscribe events fire chat shouts without a 5min
// polling delay. Skipped (logged, not fatal) when the broadcaster row
// isn't loaded — the bot still runs without real-time alerts.
func startEventSub(ctx context.Context) {
	token := mytwitch.BroadcasterUserAccessToken()
	if token == "" {
		slog.WarnContext(ctx, "skipping eventsub: no broadcaster oauth_tokens row; bootstrap with `task tripbot:auth:bootstrap:broadcaster`")
		return
	}
	if mytwitch.ChannelID == "" {
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
			BroadcasterUserID: mytwitch.ChannelID,
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
func listenForShutdown() {
	helpers.WritePidFile(c.Conf.TripbotPidFile)
	// start the graceful shutdown listener
	go gracefulShutdown()
}

// initializeTelemetry brings up OpenTelemetry providers (traces, metrics,
// logs). No-ops cleanly if OTEL_SDK_DISABLED is set or no OTLP endpoint
// is configured — see pkg/telemetry.
func initializeTelemetry() {
	ctx := context.Background()
	shutdown, err := telemetry.Init(ctx, "tripbot", version)
	if err != nil {
		// telemetry init failure shouldn't crash the bot — log and continue.
		slog.WarnContext(ctx, "telemetry init failed", "err", err)
	}
	telemetryShutdown = shutdown
}

// initializeErrorLogger makes sure the logger is configured
func initializeErrorLogger() {
	terrors.Initialize(c.Conf, version)
}

// startHttpServer starts a webserver, which is
// used for admin tools and receiving webhooks. The passed context is
// honored by the server for graceful shutdown — when it's canceled,
// the server stops accepting new connections and drains in-flight
// requests up to its shutdown timeout.
func startHttpServer(ctx context.Context) {
	// start the HTTP server
	go server.Start(ctx)
}

// findInitialVideo will determine the vido that is currently-playing
// we want to run this early, otherwise it will be unset until the first cron job runs
func findInitialVideo() {
	video.GetCurrentlyPlaying(context.Background())
	v := video.CurrentlyPlaying()
	_, err := video.LoadOrCreate(context.Background(), v.String())
	if err != nil {
		slog.Error("error loading initial video, is there a video playing?", "err", err)
	}
}

// startCron starts the background workers
func startCron() {
	// start cron and attach cronjobs
	background.StartCron()
	scheduleBackgroundJobs()
}

// loadTwitchToken pulls the bot's OAuth row from the oauth_tokens table.
// Refuses to start if the row is missing — pointing at the bootstrap CLI
// is louder than running IRC-less.
func loadTwitchToken() {
	if err := mytwitch.LoadFromDB(); err != nil {
		if errors.Is(err, mytwitch.ErrNoToken) {
			slog.Error("refusing to start: no oauth_tokens row for bot", "bot_username", c.Conf.BotUsername, "fix", "task tripbot:auth:bootstrap")
			os.Exit(1)
		}
		terrors.Fatal(err, "failed to load Twitch token from DB")
	}
}

// setUpTwitchClient sets up the Twitch client,
// used by many bot features
func setUpTwitchClient() {
	// set up the Twitch client
	client = chatbot.Initialize()
}

// updateSubscribers gets the list of current subscribers
func updateSubscribers() {
	// update subscribers list
	mytwitch.GetSubscribers(context.Background())
}

// getCurrentUsers gets the users watching the stream
func getCurrentUsers() {
	// fetch initial session
	users.UpdateSession(context.Background())
	users.PrintCurrentSession(context.Background())
}

// connectToTwitch joins Twitch chat and starts listening
func connectToTwitch() {
	client.Join(c.Conf.ChannelName)
	slog.Info("joined channel", "channel", c.Conf.ChannelName, "url", fmt.Sprintf("https://twitch.tv/%s", c.Conf.ChannelName))

	// Flip readiness on once the IRC connection is established so the
	// readiness probe (/health/ready) reports live. Until then — and after
	// any disconnect below — the pod stays up but not-ready.
	client.OnConnect(func() {
		slog.Info("connected to Twitch chat")
		server.SetReady(true)
	})

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		slog.Info("initializing connection to Twitch")
		// Connect blocks while connected and returns when the connection
		// drops; mark not-ready so the probe reflects the gap until the
		// next OnConnect fires.
		err := client.Connect()
		server.SetReady(false)
		if err != nil {
			slog.Error("unable to connect to twitch", "err", err)
			if errors.Is(err, twitch.ErrLoginAuthenticationFailed) {
				// The IRC client holds a stale token. Sync it with the
				// in-memory token (kept fresh by the hourly refresh cron)
				// so the next Connect attempt uses the current credentials.
				if tok := mytwitch.IRCAuthToken(); tok != "" {
					client.SetIRCToken(tok)
				}
			}
			time.Sleep(time.Minute)
		}
	}
}

// gracefulShutdown catches CTRL-C and cleans up
func gracefulShutdown() {
	ctrlC := make(chan os.Signal, 1)
	signal.Notify(ctrlC, os.Interrupt, syscall.SIGTERM)

	// wait for signal
	<-ctrlC

	slog.Warn("caught CTRL-C, shutting down")
	// anything below this probably won't be executed
	// try and use !shutdown instead
	//TODO: print different message if CurrentlyPlaying is ""
	slog.Info("last played video", "file", video.CurrentlyPlaying().File())
	users.Shutdown(context.Background())
	err := database.Connection().Close()
	if err != nil {
		slog.Error("error closing DB connection", "err", err)
	}
	background.StopCron()
	sentry.Flush(time.Second * 5)
	if telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := telemetryShutdown(flushCtx); err != nil {
			slog.ErrorContext(flushCtx, "telemetry shutdown failed", "err", err)
		}
		cancel()
	}
	os.Exit(1)
}

// scheduleBackgroundJobs schedules the various background jobs.
// Lives in this package (not pkg/background) to avoid circular deps with
// the job-target packages.
func scheduleBackgroundJobs() {
	onscreensCli := onscreensClient.New(c.Conf.OnscreensServerHost)
	addJob(60*time.Second, "video.GetCurrentlyPlaying", video.GetCurrentlyPlaying)
	addJob(61*time.Second, "users.UpdateSession", users.UpdateSession)
	addJob(62*time.Second, "users.UpdateLeaderboard", users.UpdateLeaderboard)
	addJob(5*time.Minute, "onscreens.ShowGuessLeaderboard", onscreensCli.ShowGuessLeaderboard)
	addJob(5*time.Minute, "users.PrintCurrentSession", users.PrintCurrentSession)
	addJob(5*time.Minute, "twitch.GetSubscribers", mytwitch.GetSubscribers)
	addJob(5*time.Minute, "twitch.GetFollowerCount", mytwitch.GetFollowerCount)
	addJob(1*time.Hour, "twitch.RefreshUserAccessToken", func(ctx context.Context) {
		mytwitch.RefreshUserAccessToken(ctx)
		// Keep the IRC client's stored token in sync with the rotated credentials.
		// go-twitch-irc captures the token at construction; without this, any
		// reconnect after the first rotation replays the original boot-time token.
		if tok := mytwitch.IRCAuthToken(); tok != "" {
			client.SetIRCToken(tok)
		}
	})
	addJob(2*time.Hour+57*time.Minute+30*time.Second, "chatbot.Chatter", chatbot.Chatter)
}

// addJob registers a gocron job at the given interval, wrapping fn with
// tracedJob so each tick opens a span and centralising the error logging.
func addJob(interval time.Duration, name string, fn func(context.Context)) {
	_, err := background.Scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(tracedJob(name, fn)),
	)
	if err != nil {
		slog.Error("error adding background job: "+name, "err", err)
	}
}
