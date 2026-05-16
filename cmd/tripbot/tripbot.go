package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	"github.com/adanalife/tripbot/pkg/helpers"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	_ "github.com/dimiro1/banner/autoload"
	"github.com/gempir/go-twitch-irc/v4"
	"github.com/getsentry/sentry-go"
	"github.com/logrusorgru/aurora/v3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var cronTracer = otel.Tracer("github.com/adanalife/tripbot/cmd/tripbot/cron")

// tracedJob wraps a cron callback in a span so each tick shows up as its
// own trace. Slow or failing jobs become visible in Grafana traces without
// having to thread context through the underlying functions (the cron
// library only accepts func()).
func tracedJob(name string, fn func()) func() {
	return func() {
		_, span := cronTracer.Start(context.Background(), "cron."+name,
			trace.WithAttributes(attribute.String("cron.job", name)))
		defer span.End()
		fn()
	}
}

// tracedJobCtx is the ctx-aware variant of tracedJob. The span's ctx is
// threaded into fn so DB queries (otelsql) and outbound HTTP (otelhttp)
// nest under cron.<name> in Tempo, giving each cron tick a tree of children
// rather than orphan spans.
func tracedJobCtx(name string, fn func(context.Context)) func() {
	return func() {
		ctx, span := cronTracer.Start(context.Background(), "cron."+name,
			trace.WithAttributes(attribute.String("cron.job", name)))
		defer span.End()
		fn(ctx)
	}
}

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

var client *twitch.Client

var telemetryShutdown telemetry.ShutdownFunc

// main performs the various steps to get the bot running
func main() {
	log.Println(aurora.Cyan(fmt.Sprintf("tripbot version %s", version)))
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
	startOBSPolling()
	loadTwitchToken()   // must precede chatbot.Initialize — provides the IRC token
	setUpTwitchClient() // required for the below
	updateSubscribers()
	getCurrentUsers()
	updateWebhookSubscriptions()
	connectToTwitch()
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
	shutdown, err := telemetry.Init(context.Background(), "tripbot", version)
	if err != nil {
		// telemetry init failure shouldn't crash the bot — log and continue.
		log.Println(aurora.Yellow(fmt.Sprintf("telemetry init: %v", err)))
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
	video.GetCurrentlyPlaying()
	v := video.CurrentlyPlaying
	_, err := video.LoadOrCreate(v.String())
	if err != nil {
		terrors.Log(err, "error loading initial video, is there a video playing?")
	}
}

// startCron starts the background workers
func startCron() {
	// start cron and attach cronjobs
	background.StartCron()
	scheduleBackgroundJobs()
}

// startOBSPolling starts the background goroutine that polls OBS WebSocket
// for streaming state and updates the obs_streaming_active gauge.
func startOBSPolling() {
	go obs.PollStreamingActive(context.Background(), 30*time.Second)
}

// loadTwitchToken pulls the bot's OAuth row from the oauth_tokens table.
// Refuses to start if the row is missing — pointing at the bootstrap CLI
// is louder than running IRC-less.
func loadTwitchToken() {
	if err := mytwitch.LoadFromDB(); err != nil {
		if errors.Is(err, mytwitch.ErrNoToken) {
			log.Printf("refusing to start: no oauth_tokens row for %q — tripbot will not run without a Twitch IRC connection", c.Conf.BotUsername)
			log.Fatal("to fix: run `task tripbot:auth:bootstrap` from the infra directory")
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
	mytwitch.GetSubscribers()
}

// getCurrentUsers gets the users watching the stream
func getCurrentUsers() {
	// fetch initial session
	users.UpdateSession(context.Background())
	users.PrintCurrentSession()
}

//updateWebhookSubscriptions makes sure webhooks are being sent to the bot
func updateWebhookSubscriptions() {
	// create webhook subscriptions
	mytwitch.UpdateWebhookSubscriptions()
}

// connectToTwitch joins Twitch chat and starts listening
func connectToTwitch() {
	client.Join(c.Conf.ChannelName)
	log.Println("Joined channel", c.Conf.ChannelName)
	log.Printf("URL: %s", aurora.Blue(fmt.Sprintf("https://twitch.tv/%s", c.Conf.ChannelName)).Underline())

	// actually connect to Twitch
	// wrapped in a loop in case twitch goes down
	for {
		log.Println(aurora.Magenta("Initializing connection to Twitch"))
		err := client.Connect()
		if err != nil {
			terrors.Log(err, "unable to connect to twitch")
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

	log.Println(aurora.Red("caught CTRL-C"))
	// anything below this probably won't be executed
	// try and use !shutdown instead
	//TODO: print different message if CurrentlyPlaying is ""
	log.Printf("Last played video: %s", aurora.Yellow(video.CurrentlyPlaying.File()))
	users.Shutdown(context.Background())
	err := database.Connection().Close()
	if err != nil {
		terrors.Log(err, "error closing DB connection")
	}
	background.StopCron()
	sentry.Flush(time.Second * 5)
	if telemetryShutdown != nil {
		flushCtx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		if err := telemetryShutdown(flushCtx); err != nil {
			log.Printf("telemetry shutdown: %v", err)
		}
		cancel()
	}
	os.Exit(1)
}

// scheduleBackgroundJobs schedules the various background jobs
// the reason we put this is in this package is because adding this to background
// would cause circular dependencies
func scheduleBackgroundJobs() {
	var err error

	// schedule these functions
	err = background.Cron.AddFunc("@every 60s", tracedJob("video.GetCurrentlyPlaying", video.GetCurrentlyPlaying))
	err = background.Cron.AddFunc("@every 61s", tracedJobCtx("users.UpdateSession", users.UpdateSession))
	err = background.Cron.AddFunc("@every 62s", tracedJob("users.UpdateLeaderboard", users.UpdateLeaderboard))
	err = background.Cron.AddFunc("@every 5m", tracedJobCtx("onscreens.ShowGuessLeaderboard", onscreensClient.ShowGuessLeaderboard))
	err = background.Cron.AddFunc("@every 5m", tracedJob("users.PrintCurrentSession", users.PrintCurrentSession))
	err = background.Cron.AddFunc("@every 5m", tracedJob("twitch.GetSubscribers", mytwitch.GetSubscribers))
	err = background.Cron.AddFunc("@every 5m", tracedJob("twitch.GetFollowerCount", mytwitch.GetFollowerCount))
	err = background.Cron.AddFunc("@every 1h", tracedJob("twitch.RefreshUserAccessToken", func() {
		mytwitch.RefreshUserAccessToken()
		// Keep the IRC client's stored token in sync with the rotated credentials.
		// go-twitch-irc captures the token at construction; without this, any
		// reconnect after the first rotation replays the original boot-time token.
		if tok := mytwitch.IRCAuthToken(); tok != "" {
			client.SetIRCToken(tok)
		}
	}))
	err = background.Cron.AddFunc("@every 2h57m30s", tracedJob("chatbot.Chatter", chatbot.Chatter))
	err = background.Cron.AddFunc("@every 12h", tracedJob("twitch.UpdateWebhookSubscriptions", mytwitch.UpdateWebhookSubscriptions))
	if err != nil {
		terrors.Log(err, "error adding at least one background job!")
	}
}
