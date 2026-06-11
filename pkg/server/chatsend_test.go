package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// recordingPublisher captures the last publish for assertions.
type recordingPublisher struct {
	calls int
	subj  string
	data  []byte
}

func (r *recordingPublisher) Publish(_ context.Context, subject string, payload []byte) {
	r.calls++
	r.subj = subject
	r.data = payload
}

func postForm(s *Server, form string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/admin/chat/send", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.chatSendHandler(rec, req)
	return rec
}

func TestChatSendHandler_PublishesValidSend(t *testing.T) {
	rec := &recordingPublisher{}
	s := New()
	s.publisher = rec

	resp := postForm(s, "identity=broadcaster&text=hello+world")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%q", resp.Code, resp.Body.String())
	}
	if rec.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", rec.calls)
	}
	if !strings.HasSuffix(rec.subj, ".chat.send.twitch") {
		t.Errorf("subject = %q, want suffix .chat.send.twitch", rec.subj)
	}
	var ev chatEvents.Send
	if err := json.Unmarshal(rec.data, &ev); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if ev.Identity != chatEvents.IdentityBroadcaster || ev.Text != "hello world" {
		t.Errorf("payload = %+v, want broadcaster/'hello world'", ev)
	}
}

func TestChatSendHandler_Rejects(t *testing.T) {
	cases := []struct {
		name string
		form string
		want int
	}{
		{"bad identity", "identity=stranger&text=hi", http.StatusBadRequest},
		{"missing identity", "text=hi", http.StatusBadRequest},
		{"empty text", "identity=bot&text=", http.StatusBadRequest},
		{"whitespace text", "identity=bot&text=+++", http.StatusBadRequest},
		{"too long", "identity=bot&text=" + strings.Repeat("a", maxChatMessageLen+1), http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingPublisher{}
			s := New()
			s.publisher = rec
			resp := postForm(s, tc.form)
			if resp.Code != tc.want {
				t.Errorf("status = %d, want %d", resp.Code, tc.want)
			}
			if rec.calls != 0 {
				t.Errorf("publish calls = %d, want 0 (nothing should publish on a rejected send)", rec.calls)
			}
		})
	}
}

func TestRenderSendForm_GatesOnLogin(t *testing.T) {
	healthyBot := mytwitch.AccountTokenStatus{Account: "bot", LoginAs: "tripbot4000"}
	healthyBcast := mytwitch.AccountTokenStatus{Account: "broadcaster", LoginAs: "adanalife_"}
	loggedOutBcast := mytwitch.AccountTokenStatus{Account: "broadcaster", LoginAs: "adanalife_", Reason: "missing"}

	t.Run("both logged in shows toggle", func(t *testing.T) {
		html := renderSendForm([]mytwitch.AccountTokenStatus{healthyBot, healthyBcast})
		if !strings.Contains(html, `value="bot"`) || !strings.Contains(html, `value="broadcaster"`) {
			t.Errorf("expected both radio options, got: %s", html)
		}
		if !strings.Contains(html, `data-broadcaster-user="adanalife_"`) {
			t.Errorf("expected broadcaster data attr, got: %s", html)
		}
		// broadcaster is the default (talking as the channel owner is the
		// common case)
		if !strings.Contains(html, `value="broadcaster" checked`) {
			t.Errorf("expected broadcaster radio pre-checked, got: %s", html)
		}
		if strings.Contains(html, `value="bot" checked`) {
			t.Errorf("bot radio should not be pre-checked when broadcaster is available, got: %s", html)
		}
	})

	t.Run("bot-only falls back to bot pre-checked", func(t *testing.T) {
		// only the bot logged in → single hidden input, but the default
		// computation should still resolve to bot rather than nothing
		html := renderSendForm([]mytwitch.AccountTokenStatus{healthyBot})
		if !strings.Contains(html, `value="bot"`) {
			t.Errorf("expected bot identity, got: %s", html)
		}
	})

	t.Run("logged-out identity is omitted", func(t *testing.T) {
		html := renderSendForm([]mytwitch.AccountTokenStatus{healthyBot, loggedOutBcast})
		if strings.Contains(html, `value="broadcaster"`) {
			t.Errorf("logged-out broadcaster should not be offered, got: %s", html)
		}
		// single available identity → hidden input, no toggle
		if !strings.Contains(html, `type="hidden"`) || !strings.Contains(html, `value="bot"`) {
			t.Errorf("expected single hidden bot identity, got: %s", html)
		}
	})

	t.Run("none logged in shows hint", func(t *testing.T) {
		html := renderSendForm([]mytwitch.AccountTokenStatus{
			{Account: "bot", LoginAs: "tripbot4000", Reason: "expired"},
		})
		if !strings.Contains(html, "chat-send-empty") {
			t.Errorf("expected empty hint, got: %s", html)
		}
		if strings.Contains(html, "<form") {
			t.Errorf("expected no form when nothing is logged in, got: %s", html)
		}
	})
}
