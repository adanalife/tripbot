package onscreensServer

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestServerShutdownStopsBackgroundGoroutines asserts that Shutdown
// actually drains the expiry sweepers + rotator loops that New spawns,
// rather than leaving them running until process exit.
//
// Goroutine counts have noise (GC, runtime scheduler), so the test
// doesn't pin an exact number — it just checks the count after Shutdown
// is meaningfully lower than the count while the server is up. With
// nine spawned goroutines (seven expiry sweepers + two rotators) the
// signal is well clear of normal background noise.
func TestServerShutdownStopsBackgroundGoroutines(t *testing.T) {
	// New() spawns the onscreen goroutines.
	srv := New(Config{Version: "test"})
	// Give the runtime a moment to actually schedule them.
	time.Sleep(50 * time.Millisecond)
	started := runtime.NumGoroutine()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned err: %v", err)
	}

	// Give the runtime a moment to reap the now-exited goroutines.
	time.Sleep(50 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Expect at least one fewer goroutine — really expect ~9 fewer, but
	// the loose bound avoids flake on a busy runtime.
	if after >= started {
		t.Errorf("expected goroutine count to drop after Shutdown; started=%d after=%d", started, after)
	}
}

// TestServerShutdownIdempotent confirms a second Shutdown call after
// the first doesn't panic (closing the same channel twice would).
func TestServerShutdownIdempotent(t *testing.T) {
	srv := New(Config{Version: "test"})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown returned err: %v", err)
	}
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown returned err: %v", err)
	}
}
