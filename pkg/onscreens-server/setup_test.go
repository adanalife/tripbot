package onscreensServer

import (
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

// newTestServer constructs a fresh *Server for the calling test, giving
// each test real state isolation rather than the cross-test sharing the
// old sync.Once helper provided.
//
// New() spawns the seven onscreens' background goroutines (rotator loops +
// expiry sweepers). They have no stop hook today, so each test leaks its
// goroutines for the duration of the test process. That's fine for state
// isolation — every *Server has its own onscreen singletons, and the
// leaked goroutines only touch the *Server that spawned them.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(Config{Version: "test", Conf: testConf})
}
