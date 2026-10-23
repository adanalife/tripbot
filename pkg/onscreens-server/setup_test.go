package onscreensServer

import (
	"context"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// testConf is the config test Servers and rotators carry — the same values
// .env.testing supplies, as a literal so tests don't reach through a loaded
// global.
var testConf = &c.OnscreensServerConfig{
	Environment: "testing",
	Platform:    "twitch",
}

// init runs before any test. terrors.Log dereferences a package-level
// config.Config interface that's nil until Initialize is called — without
// this, any handler test that walks an error path NPEs in the logger.
func init() {
	terrors.Initialize(*testConf, "test")
}

// newTestServer constructs a fresh *Server for the calling test, so no state
// is shared across tests. Cleanup shuts it down, so the overlays' background
// goroutines (expiry sweepers + rotator loops) don't outlive the test.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	srv := New(Config{Version: "test", Conf: testConf})
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv
}
