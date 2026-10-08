package onscreensServer

import (
	"context"
	"testing"
	"testing/synctest"
)

// Shutdown ends every goroutine New starts. synctest.Test waits for all of the
// bubble's goroutines to exit and fails the test if any is still blocked, so a
// sweeper or rotator loop that ignores stop() turns this red with no timing
// heuristics. Calling Shutdown twice also covers stop()'s idempotence.
func TestServerShutdownStopsBackgroundGoroutines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := New(Config{Version: "test", Conf: testConf})
		for range 2 {
			if err := srv.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
		}
	})
}
