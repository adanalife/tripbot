package onscreensServer

import (
	"context"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// init runs before any test. terrors.Log dereferences a package-level
// config.Config interface that's nil until Initialize is called — without
// this, any handler test that walks an error path NPEs in the logger.
func init() {
	terrors.Initialize(*c.Conf, "test")
}

// newTestServer constructs a fresh *Server for the calling test, giving
// each test real state isolation rather than the cross-test sharing the
// old sync.Once helper provided. A t.Cleanup hook calls Shutdown so the
// per-onscreen background goroutines (expiry sweepers + rotator loops)
// don't accumulate across the test process.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv := New(Config{Version: "test"})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return srv
}
