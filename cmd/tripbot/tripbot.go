package main

import (
	"context"
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
	"github.com/adanalife/tripbot/pkg/server"
	"github.com/adanalife/tripbot/pkg/telemetry"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
	_ "github.com/dimiro1/banner/autoload"
	"github.com/gempir/go-twitch-irc/v2"
	"github.com/getsentry/sentry-go"
	"github.com/go-co-op/gocron/v2"
	"github.com/logrusorgru/aurora"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var cronTracer = otel.Tracer("github.com/adanalife/tripbot/cmd/tripbot/cron")

// tracedJob wraps a cron callback in a span so each tick shows up as its
// own trace. Slow or failing jobs become visible in Grafana traces, and
// the span ctx is threaded into the underlying function so child spans
// (otelsql DB queries, otelhttp client calls) auto-link to the cron parent.
func tracedJob(name string, fn func(context.Context)) func(context.Context) {
	return func(ctx context.Context) {
		ctx, span := cronTracer.Start(ctx, "cron."+name,
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
	listenForShutdown()
	initializeTelemetry()
	initializeErrorLogger()
	server.SetVersion(version)
	startHttpServer()
	findInitialVideo()
	users.InitLeaderboard()
	startCron()
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
	terrors.Initialize(c.Conf)
}

// startHttpServer starts a webserver, which is
// used for admin tools and receiving webhooks
func startHttpServer() {
	// start the HTTP server
	go server.Start()
}

// findInitialVideo will determin the vido that is currently-playing
// we want to run this early, otherwise it will be unset until the first cron job runs
func findInitialVideo() {
	video.GetCurrentlyPlaying(context.Background())
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

//updateWebhookSubscriptions makes sure webhooks are being sent to the bot
func updateWebhookSubscriptions() {
	// create webhook subscriptions
	mytwitch.UpdateWebhookSubscriptions(context.Background())
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
	// anything below this probably wont be executed
	// try and use !shutdown instead
	//TODO: print different message if CurrentlyPlaying is ""
	log.Printf("Last played video: %s", aurora.Yellow(video.CurrentlyPlaying.File()))
	users.Shutdown()
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
	addJob := func(d time.Duration, name string, fn func(context.Context)) {
		_, err := background.Scheduler.NewJob(
			gocron.DurationJob(d),
			gocron.NewTask(tracedJob(name, fn)),
			gocron.WithName(name),
		)
		if err != nil {
			terrors.Log(err, "error adding background job: "+name)
		}
	}

	// schedule these functions
	addJob(60*time.Second, "video.GetCurrentlyPlaying", video.GetCurrentlyPlaying)
	addJob(61*time.Second, "users.UpdateSession", users.UpdateSession)
	addJob(62*time.Second, "users.UpdateLeaderboard", users.UpdateLeaderboard)
	addJob(5*time.Minute, "onscreens.ShowGuessLeaderboard", onscreensClient.ShowGuessLeaderboard)
	addJob(5*time.Minute, "users.PrintCurrentSession", users.PrintCurrentSession)
	addJob(5*time.Minute, "twitch.GetSubscribers", mytwitch.GetSubscribers)
	addJob(1*time.Hour, "twitch.RefreshUserAccessToken", mytwitch.RefreshUserAccessToken)
	addJob(2*time.Hour+57*time.Minute+30*time.Second, "chatbot.Chatter", chatbot.Chatter)
	addJob(12*time.Hour, "twitch.UpdateWebhookSubscriptions", mytwitch.UpdateWebhookSubscriptions)
	if !helpers.RunningOnWindows() {
		addJob(12*time.Hour, "twitch.SetStreamTags", mytwitch.SetStreamTags)
	}
}
