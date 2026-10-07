package eventsub

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// subRequest is one POST the fake Helix endpoint received.
type subRequest struct {
	method, auth, clientID, contentType string
	Type                                string            `json:"type"`
	Version                             string            `json:"version"`
	Condition                           map[string]string `json:"condition"`
	Transport                           struct {
		Method    string `json:"method"`
		SessionID string `json:"session_id"`
	} `json:"transport"`
}

// fakeTwitch stands in for both Twitch endpoints: a Helix subscription API
// answering with respond, and an EventSub WebSocket that sends a welcome, waits
// for Run's subscribe round to finish, then sends frames and closes with
// closeCode. The library runs every callback, OnWelcome included, on its own
// goroutine, so without the wait the close could overtake the round.
type fakeTwitch struct {
	respond   func(eventType string) (int, string)
	frames    []string
	closeCode websocket.StatusCode
	// holdOpen keeps the socket open after frames instead of closing it.
	holdOpen bool
	// onSubscribe runs after each subscribe request is recorded.
	onSubscribe func()

	mu    sync.Mutex
	reqs  []subRequest
	round chan struct{}
}

// roundLogger signals round on each "subscribe round complete" log line, the one
// observable end of a subscribe round.
type roundLogger struct{ round chan struct{} }

func (l roundLogger) Enabled(context.Context, slog.Level) bool { return true }
func (l roundLogger) WithAttrs([]slog.Attr) slog.Handler       { return l }
func (l roundLogger) WithGroup(string) slog.Handler            { return l }
func (l roundLogger) Handle(_ context.Context, r slog.Record) error {
	if r.Message == "eventsub subscribe round complete" {
		l.round <- struct{}{}
	}
	return nil
}

// waitRound blocks until Run finishes a subscribe round.
func (f *fakeTwitch) waitRound(t *testing.T) {
	t.Helper()
	select {
	case <-f.round:
	case <-time.After(10 * time.Second):
		t.Error("subscribe round never completed")
	}
}

func (f *fakeTwitch) start(t *testing.T) Config {
	t.Helper()
	f.round = make(chan struct{}, 1)
	prev := slog.Default()
	slog.SetDefault(slog.New(roundLogger{f.round}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	helix := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := subRequest{
			method:      r.Method,
			auth:        r.Header.Get("Authorization"),
			clientID:    r.Header.Get("Client-Id"),
			contentType: r.Header.Get("Content-Type"),
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("subscribe body is not JSON: %v", err)
		}
		f.mu.Lock()
		f.reqs = append(f.reqs, req)
		f.mu.Unlock()
		status, body := f.respond(req.Type)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		if f.onSubscribe != nil {
			f.onSubscribe()
		}
	}))
	t.Cleanup(helix.Close)

	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		ctx := r.Context()
		if err := conn.Write(ctx, websocket.MessageText, []byte(welcomeFrame("sess-1"))); err != nil {
			t.Errorf("write welcome: %v", err)
			return
		}
		if f.holdOpen {
			<-ctx.Done()
			return
		}
		f.waitRound(t)
		for _, frame := range f.frames {
			if err := conn.Write(ctx, websocket.MessageText, []byte(frame)); err != nil {
				t.Errorf("write frame: %v", err)
				return
			}
		}
		_ = conn.Close(f.closeCode, "")
	}))
	t.Cleanup(ws.Close)

	return Config{
		ClientID:          "cid",
		BroadcasterToken:  "tok",
		BroadcasterUserID: "42",
		wsURL:             "ws" + strings.TrimPrefix(ws.URL, "http"),
		subscribeURL:      helix.URL,
	}
}

func (f *fakeTwitch) requests() []subRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]subRequest(nil), f.reqs...)
}

func accepted(string) (int, string) { return http.StatusAccepted, `{"data":[],"total":1}` }

func welcomeFrame(sessionID string) string {
	return `{"metadata":{"message_id":"w","message_type":"session_welcome"},` +
		`"payload":{"session":{"id":"` + sessionID + `","status":"connected"}}}`
}

func notificationFrame(subType, event string) string {
	return `{"metadata":{"message_id":"n-` + subType + `","message_type":"notification"},` +
		`"payload":{"subscription":{"type":"` + subType + `","version":"1","status":"enabled"},"event":` + event + `}}`
}

func run(t *testing.T, cfg Config, h Handlers) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := Run(ctx, cfg, h)
	if ctx.Err() != nil {
		t.Fatal("Run did not return before the test deadline")
	}
	return err
}

// Every registered handler must produce exactly one correctly shaped Twitch
// subscription, and each notification must reach its handler with the right
// fields — a mismatch here is the bot silently missing follows or subs.
func TestRun_SubscribesAndDispatches(t *testing.T) {
	f := &fakeTwitch{
		respond:   accepted,
		closeCode: websocket.StatusNormalClosure,
		frames: []string{
			notificationFrame("channel.follow", `{"user_name":"follower"}`),
			// A revocation is logged, not fatal: later notifications still arrive.
			`{"metadata":{"message_id":"r","message_type":"revocation"},` +
				`"payload":{"subscription":{"type":"channel.follow","status":"authorization_revoked"}}}`,
			notificationFrame("channel.subscribe", `{"user_name":"subber","is_gift":true,"tier":"2000"}`),
			notificationFrame("channel.subscription.end", `{"user_name":"leaver","is_gift":false,"tier":"1000"}`),
			notificationFrame("channel.subscription.gift", `{"user_name":"gifter","total":5,"tier":"1000","is_anonymous":true}`),
			notificationFrame("channel.subscription.message",
				`{"user_name":"resubber","tier":"3000","cumulative_months":12,"streak_months":3,"message":{"text":"hi dana"}}`),
			notificationFrame("channel.raid", `{"from_broadcaster_user_name":"raider","viewers":77}`),
		},
	}
	cfg := f.start(t)

	// Handlers run on their own goroutines, in no guaranteed order.
	var mu sync.Mutex
	var got []string
	calls := make(chan struct{}, 16)
	record := func(s string) {
		mu.Lock()
		got = append(got, s)
		mu.Unlock()
		calls <- struct{}{}
	}
	h := Handlers{
		OnFollow: func(u string) { record("follow " + u) },
		OnSubscribe: func(u string, gift bool, tier string) {
			record("sub " + u + " " + tier + " " + strconv.FormatBool(gift))
		},
		OnUnsubscribe: func(u string, gift bool, tier string) {
			record("unsub " + u + " " + tier + " " + strconv.FormatBool(gift))
		},
		OnGift: func(g string, n int, tier string, anon bool) {
			record("gift " + g + " " + strconv.Itoa(n) + " " + tier + " " + strconv.FormatBool(anon))
		},
		OnResub: func(u string, cum, streak int, tier, msg string) {
			record("resub " + u + " " + strconv.Itoa(cum) + "/" + strconv.Itoa(streak) + " " + tier + " " + msg)
		},
		OnRaid: func(from string, viewers int) { record("raid " + from + " " + strconv.Itoa(viewers)) },
	}

	if err := run(t, cfg, h); err != nil {
		t.Fatalf("Run after a normal close = %v, want nil", err)
	}

	want := []string{
		"follow follower",
		"gift gifter 5 1000 true",
		"raid raider 77",
		"resub resubber 12/3 3000 hi dana",
		"sub subber 2000 true",
		"unsub leaver 1000 false",
	}
	for range want {
		select {
		case <-calls:
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for handler calls")
		}
	}
	mu.Lock()
	slices.Sort(got)
	mu.Unlock()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("handler calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	bcast := map[string]string{"broadcaster_user_id": "42"}
	wantSubs := map[string]struct {
		version string
		cond    map[string]string
	}{
		"channel.follow":               {"2", map[string]string{"broadcaster_user_id": "42", "moderator_user_id": "42"}},
		"channel.subscribe":            {"1", bcast},
		"channel.subscription.end":     {"1", bcast},
		"channel.subscription.gift":    {"1", bcast},
		"channel.subscription.message": {"1", bcast},
		"channel.raid":                 {"1", map[string]string{"to_broadcaster_user_id": "42"}},
	}
	reqs := f.requests()
	if len(reqs) != len(wantSubs) {
		t.Fatalf("got %d subscribe requests, want %d", len(reqs), len(wantSubs))
	}
	for _, r := range reqs {
		w, ok := wantSubs[r.Type]
		if !ok {
			t.Errorf("unexpected subscription %q", r.Type)
			continue
		}
		delete(wantSubs, r.Type)
		if r.method != http.MethodPost || r.auth != "Bearer tok" || r.clientID != "cid" || r.contentType != "application/json" {
			t.Errorf("%s: method=%q auth=%q client-id=%q content-type=%q", r.Type, r.method, r.auth, r.clientID, r.contentType)
		}
		if r.Transport.Method != "websocket" || r.Transport.SessionID != "sess-1" {
			t.Errorf("%s: transport = %+v, want websocket on sess-1", r.Type, r.Transport)
		}
		if r.Version != w.version {
			t.Errorf("%s: version = %q, want %q", r.Type, r.Version, w.version)
		}
		if !maps.Equal(r.Condition, w.cond) {
			t.Errorf("%s: condition = %v, want %v", r.Type, r.Condition, w.cond)
		}
	}
}

// A nil handler means no Twitch-side subscription at all, not a subscription
// whose events get dropped.
func TestRun_SubscribesOnlyRegisteredHandlers(t *testing.T) {
	f := &fakeTwitch{respond: accepted, closeCode: websocket.StatusNormalClosure}
	cfg := f.start(t)

	if err := run(t, cfg, Handlers{OnRaid: func(string, int) {}}); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	reqs := f.requests()
	if len(reqs) != 1 || reqs[0].Type != "channel.raid" {
		t.Fatalf("subscribe requests = %+v, want exactly one channel.raid", reqs)
	}
}

// Only a token that buys nothing is ErrUnauthorized; everything else is an
// ordinary connection error the caller redials on.
func TestRun_TokenRejection(t *testing.T) {
	const unauthorized = `{"error":"Unauthorized","status":401,"message":"Invalid OAuth token"}`
	cases := []struct {
		name     string
		respond  func(string) (int, string)
		wantAuth bool
	}{
		{
			name:     "every subscription 401",
			respond:  func(string) (int, string) { return http.StatusUnauthorized, unauthorized },
			wantAuth: true,
		},
		{
			name: "one 401, the rest accepted",
			respond: func(ev string) (int, string) {
				if ev == "channel.follow" {
					return http.StatusUnauthorized, unauthorized
				}
				return accepted(ev)
			},
		},
		{
			name:    "every subscription 403",
			respond: func(string) (int, string) { return http.StatusForbidden, `{"status":403}` },
		},
		{
			name:    "accepted with a malformed body",
			respond: func(string) (int, string) { return http.StatusAccepted, `{not json` },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTwitch{respond: tc.respond, closeCode: 4003}
			cfg := f.start(t)
			h := Handlers{OnFollow: func(string) {}, OnRaid: func(string, int) {}}

			err := run(t, cfg, h)
			if err == nil {
				t.Fatal("Run after a 4003 close = nil, want an error")
			}
			if got := errors.Is(err, ErrUnauthorized); got != tc.wantAuth {
				t.Errorf("errors.Is(%v, ErrUnauthorized) = %v, want %v", err, got, tc.wantAuth)
			}
			if n := len(f.requests()); n != 2 {
				t.Errorf("got %d subscribe requests, want 2", n)
			}
		})
	}
}

// Cancelling the context is the shutdown path: Run must return promptly, and
// with nil, rather than hold the socket open. The library itself returns nil or
// "use of closed network connection" depending on which of its goroutines
// notices the cancellation first. The cancel lands mid-round, so this also
// covers a subscribe round that outlives the socket.
func TestRun_ReturnsNilOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTwitch{respond: accepted, holdOpen: true, onSubscribe: cancel}
	cfg := f.start(t)

	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, Handlers{OnFollow: func(string) {}}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after cancel returned %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
	f.waitRound(t)
}

// A 403 names the scope the grant lacks: each refused event type's scope is
// reported once, and a round with no refusal reports none, so a re-consent
// clears the report on the next dial.
func TestRun_ReportsMissingScopes(t *testing.T) {
	cases := []struct {
		name    string
		respond func(string) (int, string)
		want    []string
	}{
		{
			name: "follow and the subscription family refused",
			respond: func(ev string) (int, string) {
				if ev == "channel.raid" {
					return accepted(ev)
				}
				return http.StatusForbidden, `{"error":"Forbidden","status":403}`
			},
			want: []string{"channel:read:subscriptions", "moderator:read:followers"},
		},
		{
			name: "a 401 is a refused token, not a missing scope",
			respond: func(ev string) (int, string) {
				if ev == "channel.follow" {
					return http.StatusUnauthorized, `{"status":401}`
				}
				return accepted(ev)
			},
			want: []string{},
		},
		{name: "everything accepted", respond: accepted, want: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeTwitch{respond: tc.respond, closeCode: websocket.StatusNormalClosure}
			cfg := f.start(t)
			// The library runs the callback on its own goroutine.
			reports := make(chan []string, 1)
			h := Handlers{
				OnFollow:        func(string) {},
				OnSubscribe:     func(string, bool, string) {},
				OnUnsubscribe:   func(string, bool, string) {},
				OnGift:          func(string, int, string, bool) {},
				OnResub:         func(string, int, int, string, string) {},
				OnRaid:          func(string, int) {},
				OnMissingScopes: func(s []string) { reports <- slices.Clone(s) },
			}
			if err := run(t, cfg, h); err != nil {
				t.Fatalf("Run = %v, want nil", err)
			}
			var got []string
			select {
			case got = <-reports:
			case <-time.After(10 * time.Second):
				t.Fatal("OnMissingScopes never called")
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("missing scopes = %v, want %v", got, tc.want)
			}
		})
	}
}
