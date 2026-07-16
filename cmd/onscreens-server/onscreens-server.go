package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/adanalife/tripbot/pkg/bootstrap"
	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/natsclient"
	onscreensServer "github.com/adanalife/tripbot/pkg/onscreens-server"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

// httpShutdownTimeout is how long main waits for in-flight requests to
// finish before forcing connections closed. 5s matches the exporter flush
// deadlines in pkg/bootstrap — the whole shutdown path should complete in
// well under 15s so process supervisors don't SIGKILL us mid-drain.
const httpShutdownTimeout = 5 * time.Second

func main() {
	slog.Info("onscreens-server starting", "version", version)

	conf := c.Load()

	// ctx is canceled on SIGINT/SIGTERM; srv.Start returns when that
	// happens, the drain below runs, and the process exits 0. There is no
	// separate signal-handler goroutine — this is the only shutdown path.
	ctx, flush := bootstrap.Start("onscreens-server", version, conf)
	defer flush()

	// Connect to NATS so Server.Start can attach subscribers. Optional —
	// when NATS_URL is empty the conn is nil and the subscriber registration
	// is skipped; HTTP remains the sole transport.
	natsclient.Connect(conf.NatsURL, "onscreens-server")

	// construct the server — runs all per-onscreen init (singletons +
	// background loops) up front so the HTTP routes have everything to
	// read by the time the listener accepts.
	srv := onscreensServer.New(onscreensServer.Config{Version: version, Conf: conf})

	// start the webserver — blocks until ListenAndServe fails or the
	// signal context cancels
	if err := srv.Start(ctx); err != nil {
		terrors.Fatal(err, "couldn't start server")
	}

	// drain in-flight requests (so onscreens-render responses aren't cut)
	// before the deferred flush sends the exporter backlog.
	slog.Warn("shutting down")
	httpCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	if err := srv.Shutdown(httpCtx); err != nil {
		slog.Error("error during onscreens-server HTTP shutdown", "err", err)
	}
	cancel()
}
