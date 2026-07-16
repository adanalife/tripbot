package chatbot

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGatewayTwitch_FollowedAt_Following(t *testing.T) {
	followedAt := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"followed_at":"` + followedAt.Format(time.RFC3339) + `"}`))
	}))
	defer srv.Close()

	when, ok := newGatewayTwitch(srv.URL).FollowedAt("Viewer1")
	if !ok {
		t.Fatal("expected ok=true for a follower")
	}
	if !when.Equal(followedAt) {
		t.Errorf("followed_at = %v, want %v", when, followedAt)
	}
	if gotPath != "/v1/followed-at/Viewer1" {
		t.Errorf("request path = %q, want /v1/followed-at/Viewer1", gotPath)
	}
}

func TestGatewayTwitch_FollowedAt_NotAFollower(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not a follower"}`))
	}))
	defer srv.Close()

	if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
		t.Error("expected ok=false on 404")
	}
}

func TestGatewayTwitch_FollowedAt_FailsClosedOnError(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"upstream 502", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}},
		{"malformed body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
				t.Error("expected ok=false (fail closed)")
			}
		})
	}
}

func TestGatewayTwitch_FollowedAt_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed server → connection refused

	if _, ok := newGatewayTwitch(srv.URL).FollowedAt("viewer1"); ok {
		t.Error("expected ok=false on transport error")
	}
}

func TestNewTwitch_NoURLIsNoop(t *testing.T) {
	// A non-Twitch instance has no TWITCH_API_URL, so there's no gateway to
	// reach — newTwitch returns the fail-closed no-op adapter.
	if _, ok := newTwitch(&App{Cfg: testConf}).(noTwitch); !ok {
		t.Error("expected noTwitch when TWITCH_API_URL is empty")
	}
}
