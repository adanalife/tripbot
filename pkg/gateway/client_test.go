package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_TrimsTrailingSlash(t *testing.T) {
	if got := New("http://gateway-twitch:8080/").BaseURL(); got != "http://gateway-twitch:8080" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", got)
	}
}

func TestFollowedAt_Following(t *testing.T) {
	followedAt := time.Now().Add(-72 * time.Hour).UTC().Truncate(time.Second)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"followed_at":"` + followedAt.Format(time.RFC3339) + `"}`))
	}))
	defer srv.Close()

	when, ok, err := New(srv.URL).FollowedAt(context.Background(), "Viewer1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
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

func TestFollowedAt_NotAFollower(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not a follower"}`))
	}))
	defer srv.Close()

	when, ok, err := New(srv.URL).FollowedAt(context.Background(), "viewer1")
	if err != nil {
		t.Fatalf("404 is not a follower, not an error; got %v", err)
	}
	if ok {
		t.Error("expected ok=false on 404")
	}
	if !when.IsZero() {
		t.Errorf("expected zero time on 404, got %v", when)
	}
}

func TestFollowedAt_ErrorsOnBadResponse(t *testing.T) {
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
			if _, _, err := New(srv.URL).FollowedAt(context.Background(), "viewer1"); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestFollowedAt_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // closed server → connection refused

	if _, _, err := New(srv.URL).FollowedAt(context.Background(), "viewer1"); err == nil {
		t.Error("expected a transport error")
	}
}

func TestIsLive(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"live":true}`))
	}))
	defer srv.Close()

	live, err := New(srv.URL).IsLive(context.Background(), "adanalife_")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !live {
		t.Error("expected live=true")
	}
	if gotPath != "/v1/live/adanalife_" {
		t.Errorf("request path = %q, want /v1/live/adanalife_", gotPath)
	}
}

func TestIsLive_ErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := New(srv.URL).IsLive(context.Background(), "adanalife_"); err == nil {
		t.Error("expected an error on non-200")
	}
}

func TestSendChat(t *testing.T) {
	var gotBody struct{ Identity, Text string }
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"sent"}`))
	}))
	defer srv.Close()

	if err := New(srv.URL).SendChat(context.Background(), "broadcaster", "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/chat" {
		t.Errorf("request = %s %s, want POST /v1/chat", gotMethod, gotPath)
	}
	if gotBody.Identity != "broadcaster" || gotBody.Text != "hello" {
		t.Errorf("body = %+v, want {broadcaster hello}", gotBody)
	}
}

func TestSendChat_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented) // e.g. a platform with no chat send
	}))
	defer srv.Close()

	if err := New(srv.URL).SendChat(context.Background(), "broadcaster", "hi"); err == nil {
		t.Error("expected an error on non-2xx")
	}
}
