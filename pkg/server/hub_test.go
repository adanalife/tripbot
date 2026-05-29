package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/eventbus"
)

func TestHub_appendChat_ringCapAndOrder(t *testing.T) {
	h := NewHub()
	total := chatRingSize + 50
	for i := 0; i < total; i++ {
		h.appendChat(ChatLine{Username: "u", Text: string(rune('a')) + itoa(i)})
	}
	snap := h.snapshotChat()
	if len(snap) != chatRingSize {
		t.Fatalf("ring size = %d, want %d", len(snap), chatRingSize)
	}
	// Oldest 50 should have been evicted; first retained is index 50, last is total-1.
	if want := "a" + itoa(50); snap[0].Text != want {
		t.Errorf("oldest retained = %q, want %q", snap[0].Text, want)
	}
	if want := "a" + itoa(total-1); snap[len(snap)-1].Text != want {
		t.Errorf("newest = %q, want %q", snap[len(snap)-1].Text, want)
	}
}

func TestHub_registerBroadcastUnregister(t *testing.T) {
	h := NewHub()
	a := h.register()
	b := h.register()

	h.broadcast(sseEvent{Name: "chat", Data: "<x>"})
	if got := (<-a).Data; got != "<x>" {
		t.Errorf("client a got %q", got)
	}
	if got := (<-b).Data; got != "<x>" {
		t.Errorf("client b got %q", got)
	}

	h.unregister(a)
	// a is closed now; receiving yields the zero value with ok=false.
	if _, ok := <-a; ok {
		t.Errorf("client a channel should be closed after unregister")
	}
	// unregister is idempotent — second call must not panic.
	h.unregister(a)

	h.broadcast(sseEvent{Name: "chat", Data: "<y>"})
	if got := (<-b).Data; got != "<y>" {
		t.Errorf("client b got %q after a unregistered", got)
	}
}

// TestHub_broadcast_dropsSlowClient asserts a client whose buffer is full
// doesn't block broadcast (the NATS callback path must never stall on a slow
// browser). We fill the buffer, then broadcast more; broadcast returns and the
// channel holds at most its capacity.
func TestHub_broadcast_dropsSlowClient(t *testing.T) {
	h := NewHub()
	slow := h.register()
	for i := 0; i < sseClientBuffer+10; i++ {
		h.broadcast(sseEvent{Name: "chat", Data: "x"}) // must not block
	}
	if len(slow) != sseClientBuffer {
		t.Errorf("slow client buffer len = %d, want %d (extras dropped)", len(slow), sseClientBuffer)
	}
}

func TestHub_handleChat_updatesRingAndBroadcasts(t *testing.T) {
	h := NewHub()
	client := h.register()

	payload, _ := json.Marshal(eventbus.ChatMessage{Username: "DanaLol", Text: "hi there"})
	h.handleChat(context.Background(), payload)

	// ring updated
	snap := h.snapshotChat()
	if len(snap) != 1 || snap[0].Username != "DanaLol" || snap[0].Text != "hi there" {
		t.Fatalf("ring = %+v, want one DanaLol/hi there line", snap)
	}
	// client got a rendered fragment
	ev := <-client
	if ev.Name != "chat" {
		t.Errorf("event name = %q, want chat", ev.Name)
	}
	if !strings.Contains(ev.Data, "DanaLol") || !strings.Contains(ev.Data, "hi there") {
		t.Errorf("fragment %q missing username/text", ev.Data)
	}
}

func TestHub_handleChat_badPayloadNoCrash(t *testing.T) {
	h := NewHub()
	h.handleChat(context.Background(), []byte("not json"))
	if len(h.snapshotChat()) != 0 {
		t.Errorf("bad payload should not append to ring")
	}
}

func TestRenderChatLine_escapesHTML(t *testing.T) {
	got := renderChatLine(ChatLine{Username: "evil", Text: "<script>alert(1)</script>"})
	if strings.Contains(got, "<script>") {
		t.Errorf("chat text not escaped: %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("expected escaped script tag, got %q", got)
	}
}

// itoa is a tiny int->string helper to avoid pulling strconv into table loops.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
