package onscreensServer

import (
	"sync"

	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// init runs before any test. terrors.Log dereferences a package-level
// config.Config interface that's nil until Initialize is called — without
// this, any handler test that walks an error path NPEs in the logger.
func init() {
	terrors.Initialize(*c.Conf, "test")
}

// testServer constructs a *Server once per test binary and reuses it
// across tests. Constructing fresh per test would be cleaner but each
// New() call spawns the seven background goroutines (rotator loops +
// expiry sweepers); the current tests are read-only against handler
// validation paths so the shared instance is fine and saves the
// goroutine churn.
var (
	testServerOnce sync.Once
	testServerInst *Server
)

func testServer() *Server {
	testServerOnce.Do(func() {
		testServerInst = New(Config{Version: "test"})
	})
	return testServerInst
}
