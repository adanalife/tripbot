package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adanalife/tripbot/pkg/gateway"
)

// The mint has to precede the bounce: a broadcast that isn't bound yet has no
// ingest for the fresh RTMP session to auto-start from, so a push re-opened
// first lands on nothing listening.
func TestRecoverYouTubeEgress_MintsBeforeBouncingOBS(t *testing.T) {
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
	if err := recoverYouTubeEgress(context.Background(), gateway.New(srv.URL), restartOBS); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"POST /v1/egress/start", "restart obs output"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, calls[i], want[i])
		}
	}
}

// A failed mint must still bounce the output. The fault that hit prod twice —
// a broadcast waiting on an encoder it hasn't recognised — is fixed by the
// bounce alone, so skipping it on a gateway error would give up the recovery
// that actually works. The error is still reported, because a permanently
// broken egress path is worth surfacing.
func TestRecoverYouTubeEgress_BouncesEvenWhenMintFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable) // e.g. no reusable liveStream to bind
	}))
	defer srv.Close()

	bounced := false
	restartOBS := func(context.Context) error { bounced = true; return nil }
	if err := recoverYouTubeEgress(context.Background(), gateway.New(srv.URL), restartOBS); err == nil {
		t.Error("expected the mint failure to be reported")
	}
	if !bounced {
		t.Error("output not bounced: the pending-broadcast recovery was skipped")
	}
}

// A bounce that fails leaves the broadcast minted but unfed, so the recovery has
// to report failure — the watchdog's retry is the only thing that reaches it.
func TestRecoverYouTubeEgress_ReportsBounceFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	restartOBS := func(context.Context) error { return errors.New("obs unreachable") }
	if err := recoverYouTubeEgress(context.Background(), gateway.New(srv.URL), restartOBS); err == nil {
		t.Fatal("expected an error when the OBS bounce fails")
	}
}
