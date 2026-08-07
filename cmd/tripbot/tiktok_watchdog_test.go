package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adanalife/tripbot/pkg/gateway"
)

// A re-mint is only a recovery if the old room is torn down before the new one
// is minted — starting first would bind a second relay target against a room
// the stop is about to reap — and if the OBS push is re-opened after the bind,
// which is what actually points it at the new room.
func TestRemintTikTokEgress_BouncesOBSAfterStarting(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restartOBS := func(context.Context) error {
		calls = append(calls, "restart obs output")
		return nil
	}
	if err := remintTikTokEgress(context.Background(), gateway.New(srv.URL), 0, restartOBS); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"POST /v1/egress/stop", "POST /v1/egress/start", "restart obs output"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

// A failed stop must not be followed by a start: minting a fresh room while the
// old one may still be bound leaves two rooms and no way to tell which the push
// lands on. Nor is there anything to point the push at, so OBS stays untouched.
func TestRemintTikTokEgress_SkipsStartWhenStopFails(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusServiceUnavailable) // e.g. no Streamlabs token
	}))
	defer srv.Close()

	bounced := false
	restartOBS := func(context.Context) error { bounced = true; return nil }
	if err := remintTikTokEgress(context.Background(), gateway.New(srv.URL), 0, restartOBS); err == nil {
		t.Fatal("expected an error when the stop fails")
	}
	if calls != 1 {
		t.Errorf("gateway calls = %d, want 1 (start must not run)", calls)
	}
	if bounced {
		t.Error("OBS output bounced with no new target bound")
	}
}

// A bounce that fails leaves the room minted but unfed, so the re-mint has to
// report failure — the watchdog's retry is the only thing that reaches it.
func TestRemintTikTokEgress_ReportsBounceFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restartOBS := func(context.Context) error { return errors.New("obs unreachable") }
	if err := remintTikTokEgress(context.Background(), gateway.New(srv.URL), 0, restartOBS); err == nil {
		t.Fatal("expected an error when the OBS bounce fails")
	}
}
