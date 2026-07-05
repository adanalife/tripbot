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

func TestActiveBroadcast(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"video_id":"vid123","live":true,"privacy":"unlisted"}`))
	}))
	defer srv.Close()

	b, err := New(srv.URL).ActiveBroadcast(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.VideoID != "vid123" || !b.Live || b.Privacy != "unlisted" {
		t.Errorf("ActiveBroadcast = %+v, want {VideoID:vid123 Live:true Privacy:unlisted}", b)
	}
	if gotPath != "/v1/broadcast" {
		t.Errorf("request path = %q, want /v1/broadcast", gotPath)
	}
}

func TestActiveBroadcast_ErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer srv.Close()

	if _, err := New(srv.URL).ActiveBroadcast(context.Background()); err == nil {
		t.Error("expected an error on non-200 (e.g. a platform with no broadcast object)")
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

func TestUserID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"12345","login":"adanalife_","display_name":"ADanaLife_"}`))
	}))
	defer srv.Close()

	id, err := New(srv.URL).UserID(context.Background(), "adanalife_")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "12345" {
		t.Errorf("id = %q, want 12345", id)
	}
	if gotPath != "/v1/users/adanalife_" {
		t.Errorf("request path = %q, want /v1/users/adanalife_", gotPath)
	}
}

func TestUserID_ErrorsOnEmptyID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"","login":"ghost"}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).UserID(context.Background(), "ghost"); err == nil {
		t.Error("expected an error on empty id")
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

func TestChatters(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"count":3,"chatters":["alice","bob"]}`))
	}))
	defer srv.Close()

	count, logins, err := New(srv.URL).Chatters(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/chatters" {
		t.Errorf("path = %q, want /v1/chatters", gotPath)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (total can exceed len(logins))", count)
	}
	if len(logins) != 2 || logins[0] != "alice" || logins[1] != "bob" {
		t.Errorf("logins = %v, want [alice bob]", logins)
	}
}

func TestSubscribers(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"subscribers":["sub1","sub2"]}`))
	}))
	defer srv.Close()

	subs, err := New(srv.URL).Subscribers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/subscribers" {
		t.Errorf("path = %q, want /v1/subscribers", gotPath)
	}
	if len(subs) != 2 || subs[0] != "sub1" {
		t.Errorf("subscribers = %v, want [sub1 sub2]", subs)
	}
}

func TestFollowers(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"total":42}`))
	}))
	defer srv.Close()

	total, err := New(srv.URL).Followers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/followers" {
		t.Errorf("path = %q, want /v1/followers", gotPath)
	}
	if total != 42 {
		t.Errorf("total = %d, want 42", total)
	}
}

func TestCachedReads_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	c := New(srv.URL)
	if _, _, err := c.Chatters(context.Background()); err == nil {
		t.Error("Chatters: expected error on non-200")
	}
	if _, err := c.Subscribers(context.Background()); err == nil {
		t.Error("Subscribers: expected error on non-200")
	}
	if _, err := c.Followers(context.Background()); err == nil {
		t.Error("Followers: expected error on non-200")
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

func TestInboundChat_DecodesPageAndSendsCursor(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[{"author":"Viewer","text":"hi"}],"cursor":"c2","live":true,"poll_after_ms":3000}`))
	}))
	defer srv.Close()

	page, err := New(srv.URL).InboundChat(context.Background(), "c1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/v1/chat/inbound" {
		t.Errorf("path = %q, want /v1/chat/inbound", gotPath)
	}
	if gotQuery != "c1" {
		t.Errorf("cursor query = %q, want c1", gotQuery)
	}
	if !page.Live || page.Cursor != "c2" || page.PollAfterMS != 3000 {
		t.Errorf("page = %+v", page)
	}
	if len(page.Messages) != 1 || page.Messages[0].Author != "Viewer" || page.Messages[0].Text != "hi" {
		t.Errorf("messages = %+v", page.Messages)
	}
}

func TestInboundChat_OmitsCursorParamWhenEmpty(t *testing.T) {
	var hadCursorParam bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadCursorParam = r.URL.Query()["cursor"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messages":[],"cursor":"","live":false,"poll_after_ms":60000}`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL).InboundChat(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hadCursorParam {
		t.Error("empty cursor should omit the ?cursor param entirely")
	}
}
