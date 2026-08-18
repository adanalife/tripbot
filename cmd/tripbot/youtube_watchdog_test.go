package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/adanalife/tripbot/pkg/gateway"
)

// The live-check has to answer false for a broadcast that exists but hasn't
// gone live, because that pending state is the whole failure the YouTube leg
// recovers: OBS pushes, Studio waits on an encoder it never recognised, and
// nothing else in the loop can tell the two apart. Reading "a broadcast is
// there, so we're up" would make the watchdog blind to the one outage it was
// added for (2026-08-05, dark 9h41m with zero recovery attempts).
func TestYouTubeChannelLive(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"active broadcast", `{"video_id":"vid123","live":true,"privacy":"unlisted"}`, true},
		{"pending broadcast", `{"video_id":"vid123","live":false,"privacy":"unlisted"}`, false},
		{"no broadcast at all", `{"live":false}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			live, err := youtubeChannelLive(gateway.New(srv.URL))(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if live != tt.want {
				t.Errorf("live = %v, want %v", live, tt.want)
			}
			if gotPath != "/v1/broadcast" {
				t.Errorf("request path = %q, want /v1/broadcast", gotPath)
			}
		})
	}
}

// A gateway that can't answer must report the error rather than a bare false.
// WatchSilentDisconnect holds its collected misses on an errored check and
// clears them on a definite "live", so collapsing an outage into false would
// count unreachability as evidence the channel is dark and eventually restart
// a stream that was fine.
func TestYouTubeChannelLiveReportsGatewayFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	live, err := youtubeChannelLive(gateway.New(srv.URL))(context.Background())
	if err == nil {
		t.Fatal("expected an error when the gateway is unreachable")
	}
	if live {
		t.Error("live = true alongside an error; the caller reads the error first")
	}
}
