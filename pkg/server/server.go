package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Server holds the web server's runtime state: its config, the build version
// tag the /version endpoint reports, the feature-flag client the console's
// /api/flags endpoints read/toggle, and the store + publisher behind
// /api/rotators. cmd/tripbot constructs one via New and installs the version,
// user flags and playhead before Start runs. The flag client, bed store and
// rotator store + publisher come up later in its boot, while the server is
// already answering — so those four are guarded by lateMu and read through
// their accessors (flagClient, bedStore, rotatorDeps).
type Server struct {
	cfg        *c.TripbotConfig
	versionTag string
	userFlags  UserFlagger
	playhead   Playhead

	lateMu     sync.RWMutex
	flags      feature.FlagClient
	beds       BedStore
	rotators   RotatorStore
	rotatorPub RotatorPublisher
}

// flagClient is the installed feature-flag client, nil until SetFlags runs.
func (s *Server) flagClient() feature.FlagClient {
	s.lateMu.RLock()
	defer s.lateMu.RUnlock()
	return s.flags
}

// bedStore is the installed background-audio store, nil until SetBeds runs.
func (s *Server) bedStore() BedStore {
	s.lateMu.RLock()
	defer s.lateMu.RUnlock()
	return s.beds
}

// rotatorDeps is the installed rotator store and publisher, nil until
// SetRotators runs.
func (s *Server) rotatorDeps() (RotatorStore, RotatorPublisher) {
	s.lateMu.RLock()
	defer s.lateMu.RUnlock()
	return s.rotators, s.rotatorPub
}

// New constructs a Server with the default "dev" version tag (overridden by
// SetVersion at startup).
func New(cfg *c.TripbotConfig) *Server {
	return &Server{cfg: cfg, versionTag: "dev"}
}

// shutdownTimeout is how long Shutdown waits for in-flight requests to
// finish before forcing connections closed. 15s is the typical sweet spot:
// long enough that healthy requests complete, short enough that a stuck
// handler doesn't block process exit indefinitely.
const shutdownTimeout = 15 * time.Second

// maxRequestBody caps every request body. The largest legitimate one is a
// rotator config at its validation limits (TestMaxRequestBodyFitsRotatorConfig);
// 1 MiB holds that even when every rune is an emoji the console's JSON encoder
// escapes as a 12-byte surrogate pair, and stops a caller streaming an
// unbounded body into a decode.
const maxRequestBody = 1 << 20

// Start starts the web server, returning the listener error if it fails to
// come up. When ctx is canceled (e.g. SIGINT/SIGTERM via
// signal.NotifyContext) the server stops accepting new connections, waits up
// to shutdownTimeout for in-flight requests to complete, and returns nil.
// Whether a listener failure should end the process is the binary's call, not
// this package's — cmd/onscreens-server makes the same choice with its own
// server.
func (s *Server) Start(ctx context.Context) error {
	slog.InfoContext(ctx, "starting web server", "port", s.cfg.TripbotServerPort)

	r := mux.NewRouter()

	// healthcheck endpoints
	hp := r.PathPrefix("/health").Methods("GET", "HEAD").Subrouter()
	hp.Handle("/live", httpmw.Tagged("/health/live", httpmw.LivenessHandler()))
	// /ready runs no checks: tripbot's HTTP surface (the console-facing /api/*
	// endpoints, /metrics, /version) doesn't depend on the Twitch connection, so
	// the pod must stay routable even when the bot is offline. Chat-connection is
	// surfaced via the tripbot_twitch_connected gauge.
	hp.Handle("/ready", httpmw.Tagged("/health/ready", httpmw.ReadinessHandler()))
	// /deps reports the same verdict readiness deliberately doesn't gate on:
	// whether Postgres and NATS are actually usable right now. Non-gating, so
	// a wedged dep shows up in the console's status table instead of removing
	// the pod that would let anyone look at it.
	hp.Handle("/deps", httpmw.Tagged("/health/deps", httpmw.ReadinessHandler(s.depChecks()...)))

	// version endpoint — returns build metadata as JSON
	r.Handle("/version", httpmw.Tagged("/version", s.versionHandler)).Methods("GET", "HEAD")

	// static assets
	r.Handle("/favicon.ico", httpmw.Tagged("/favicon.ico", faviconHandler)).Methods("GET")

	// prometheus metrics endpoint
	r.Path("/metrics").Handler(httpmw.Tagged("/metrics", promhttp.Handler().ServeHTTP))

	// console-facing read-only endpoints. The standalone tripbot-console holds
	// no DB/Twitch access of its own and proxies these over the in-namespace
	// Service.
	//
	// read-only JSON profile for the console's user popover.
	r.Handle("/api/user/{username}", httpmw.Tagged("/api/user/{username}", s.userProfileAPIHandler)).Methods("GET")
	// the write behind the presence report: flag an account as a bot or keep
	// it off the leaderboard.
	r.Handle("/api/user/{username}/flags", httpmw.Tagged("/api/user/{username}/flags", s.userFlagsHandler)).Methods("POST")
	// every starred account, so the console can pick friends out of its lists.
	r.Handle("/api/users/starred", httpmw.Tagged("/api/users/starred", starredUsersHandler)).Methods("GET")
	// read-only JSON list of the logins currently in chat, for the console's
	// currently-active-chatters panel.
	r.Handle("/api/chatters", httpmw.Tagged("/api/chatters", chattersHandler)).Methods("GET")
	// read-only JSON of the current golang-migrate schema version, so the console
	// can surface which migration the env's DB is on.
	r.Handle("/api/db/migration", httpmw.Tagged("/api/db/migration", migrationVersionAPIHandler)).Methods("GET")
	// the full dashcam route as JSON, for the console's map overlay.
	r.Handle("/admin/map/corpus", httpmw.Tagged("/admin/map/corpus", mapCorpusHandler)).Methods("GET")
	// per-corpus row counts by pipeline stage, for the console's pipeline page.
	r.Handle("/admin/corpus/stats", httpmw.Tagged("/admin/corpus/stats", corpusStatsHandler)).Methods("GET")
	// the recent breadcrumbs of each platform, so a fresh console seeds its map
	// trail from the database rather than from what the video stream can replay.
	r.Handle("/admin/map/recent", httpmw.Tagged("/admin/map/recent", mapRecentHandler)).Methods("GET")
	// this instance's playhead and the clip's track, so the console's pin
	// follows the road for the whole clip.
	r.Handle("/api/map/playhead", httpmw.Tagged("/api/map/playhead", s.mapPlayheadHandler)).Methods("GET")
	// read-only JSON of the feature-flag snapshot, and a write to flip a flag's
	// global default — the console's feature-flag panel. Internal-only like the
	// rest of /api (no Ingress; reached over the in-namespace Service).
	r.Handle("/api/flags", httpmw.Tagged("/api/flags", s.flagsHandler)).Methods("GET")
	r.Handle("/api/flags/{key}", httpmw.Tagged("/api/flags/{key}", s.flagToggleHandler)).Methods("POST")
	// audit trail for the standalone console: it reports each successful admin
	// mutation here, and the report lands in the permanent events log as a
	// console_action event.
	r.Handle("/api/events/console-action", httpmw.Tagged("/api/events/console-action", s.consoleActionHandler)).Methods("POST")

	// read-only JSON aggregates over the append-only analytics tables (events,
	// video_plays, viewer_samples), for the console's insights panels.
	r.Handle("/api/insights/commands", httpmw.Tagged("/api/insights/commands", commandInsightsHandler)).Methods("GET")
	r.Handle("/api/insights/guesses", httpmw.Tagged("/api/insights/guesses", guessInsightsHandler)).Methods("GET")
	r.Handle("/api/insights/footage", httpmw.Tagged("/api/insights/footage", footageInsightsHandler)).Methods("GET")
	r.Handle("/api/insights/regions", httpmw.Tagged("/api/insights/regions", regionInsightsHandler)).Methods("GET")
	r.Handle("/api/insights/viewers", httpmw.Tagged("/api/insights/viewers", viewerSeriesHandler)).Methods("GET")
	r.Handle("/api/insights/sessions", httpmw.Tagged("/api/insights/sessions", sessionInsightsHandler)).Methods("GET")
	r.Handle("/api/insights/presence", httpmw.Tagged("/api/insights/presence", presenceInsightsHandler)).Methods("GET")
	// the chat log, paged newest-first, for the console's /history page.
	r.Handle("/api/chat/history", httpmw.Tagged("/api/chat/history", chatHistoryHandler)).Methods("GET")
	// one clip's record, for the console's video page.
	r.Handle("/api/videos/{id}", httpmw.Tagged("/api/videos/{id}", videoHandler)).Methods("GET")
	// the operator's marks on a moment of a clip: a star, or a bad GPS point.
	r.Handle("/api/videos/{id}/moments", httpmw.Tagged("/api/videos/{id}/moments", videoMomentsHandler)).Methods("POST")
	r.Handle("/api/sources/{stem}", httpmw.Tagged("/api/sources/{stem}", sourceHandler)).Methods("GET")
	// the footage calendar: every filmed day, and one day's clips.
	r.Handle("/api/footage/days", httpmw.Tagged("/api/footage/days", footageDaysHandler)).Methods("GET")
	r.Handle("/api/footage/days/{day}", httpmw.Tagged("/api/footage/days/{day}", footageDayHandler)).Methods("GET")
	// every season-2 piece auto-trim or a reviewer dropped whole.
	r.Handle("/api/footage/dropped", httpmw.Tagged("/api/footage/dropped", footageDroppedHandler)).Methods("GET")
	// read-only JSON stats for the console's stats page: lifetime totals over
	// the whole log, a recent playback window, and community numbers.
	r.Handle("/api/stats/lifetime", httpmw.Tagged("/api/stats/lifetime", lifetimeStatsHandler)).Methods("GET")
	r.Handle("/api/stats/playback", httpmw.Tagged("/api/stats/playback", playbackStatsHandler)).Methods("GET")
	r.Handle("/api/stats/playback/daily", httpmw.Tagged("/api/stats/playback/daily", playbackDailyHandler)).Methods("GET")
	r.Handle("/api/stats/community", httpmw.Tagged("/api/stats/community", s.communityStatsHandler)).Methods("GET")
	r.Handle("/api/stats/songs", httpmw.Tagged("/api/stats/songs", songStatsHandler)).Methods("GET")

	// The monthly miles and guess boards for one month, plus the months that
	// have data — the console's leaderboard pane builds its selector from the
	// same call that fills it.
	r.Handle("/api/leaderboards", httpmw.Tagged("/api/leaderboards", s.leaderboardsHandler)).Methods("GET")

	// Background audio: which bed this platform's OBS is playing, and the
	// switch between them.
	r.Handle("/api/audio", httpmw.Tagged("/api/audio", s.audioHandler)).Methods("GET")
	r.Handle("/api/audio", httpmw.Tagged("/api/audio", s.audioSetHandler)).Methods("POST")

	// Corner-rotator copy for the console's editor: read one platform's copy,
	// save it (which pushes it live over NATS), or reset it to the defaults
	// compiled into onscreens-server.
	rotatorRoute := "/api/rotators/{platform}"
	r.Handle(rotatorRoute, httpmw.Tagged(rotatorRoute, s.rotatorsGetHandler)).Methods("GET")
	r.Handle(rotatorRoute, httpmw.Tagged(rotatorRoute, s.rotatorsPutHandler)).Methods("PUT")
	r.Handle(rotatorRoute, httpmw.Tagged(rotatorRoute, s.rotatorsResetHandler)).Methods("DELETE")

	// catch everything else
	r.NotFoundHandler = httpmw.Tagged("/", catchAllHandler)

	app := httpmw.NewChain(s.cfg.ServerType, s.cfg.IsDevelopment(), func(any) { instrumentation.HTTPPanics.Inc(s.cfg.ServerType) })
	app.Use(httpmw.MaxBody(maxRequestBody))

	// attaching routes to handler happens last
	app.UseHandler(r)

	srv := &http.Server{
		Addr: fmt.Sprintf("0.0.0.0:%s", s.cfg.TripbotServerPort),
		// Every response here is short (small JSON for the console, the metrics
		// scrape), so a normal write deadline is safe. A long-lived stream on
		// this server — an SSE endpoint, say — would need WriteTimeout=0.
		ReadTimeout:       time.Second * 15,
		ReadHeaderTimeout: time.Second * 15,
		WriteTimeout:      time.Second * 15,
		IdleTimeout:       time.Second * 60,
		MaxHeaderBytes:    1 << 20, // 1 MB
		Handler:           otelhttp.NewHandler(app, s.cfg.ServerType),
	}

	// Run ListenAndServe in a goroutine so we can block on ctx.Done() and
	// trigger a graceful shutdown when a signal arrives. ErrServerClosed is
	// the expected return after Shutdown is called and is not an error.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down web server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(shutdownCtx, "error during web server shutdown", "err", err)
		}
		return nil
	}
}
