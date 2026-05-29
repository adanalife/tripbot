package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

	emitted := time.Date(2026, 5, 29, 13, 5, 0, 0, time.UTC)
	payload, _ := json.Marshal(eventbus.ChatMessage{
		Username: "DanaLol", Text: "hi there", EmittedAt: emitted.Format(time.RFC3339Nano),
	})
	h.handleChat(context.Background(), payload)

	// ring updated, timestamp parsed
	snap := h.snapshotChat()
	if len(snap) != 1 || snap[0].Username != "DanaLol" || snap[0].Text != "hi there" {
		t.Fatalf("ring = %+v, want one DanaLol/hi there line", snap)
	}
	if !snap[0].At.Equal(emitted) {
		t.Errorf("ring line At = %v, want %v", snap[0].At, emitted)
	}
	// client got a rendered fragment with username, text, and a time element
	ev := <-client
	if ev.Name != "chat" {
		t.Errorf("event name = %q, want chat", ev.Name)
	}
	if !strings.Contains(ev.Data, "DanaLol") || !strings.Contains(ev.Data, "hi there") {
		t.Errorf("fragment %q missing username/text", ev.Data)
	}
	if !strings.Contains(ev.Data, `<time class="ct-ts"`) || !strings.Contains(ev.Data, "13:05") {
		t.Errorf("fragment %q missing timestamp", ev.Data)
	}
}

func TestHub_handleChat_badPayloadNoCrash(t *testing.T) {
	h := NewHub()
	h.handleChat(context.Background(), []byte("not json"))
	if len(h.snapshotChat()) != 0 {
		t.Errorf("bad payload should not append to ring")
	}
}

// TestHub_updateViewers_flashDirection walks the count through first-value,
// rise, fall, and no-change to assert the flash direction the panel renders.
func TestHub_updateViewers_flashDirection(t *testing.T) {
	h := NewHub()
	// First value sets the baseline — no flash (nothing to compare against).
	if dir := h.updateViewers(10); dir != "" {
		t.Errorf("first value: dir = %q, want \"\" (baseline, no flash)", dir)
	}
	if dir := h.updateViewers(12); dir != "up" {
		t.Errorf("rise: dir = %q, want up", dir)
	}
	if dir := h.updateViewers(11); dir != "down" {
		t.Errorf("fall: dir = %q, want down", dir)
	}
	if dir := h.updateViewers(11); dir != "" {
		t.Errorf("unchanged: dir = %q, want \"\" (no flash)", dir)
	}
}

func TestHub_handleViewerCount_broadcastsRenderedCount(t *testing.T) {
	h := NewHub()
	client := h.register()

	payload, _ := json.Marshal(eventbus.ViewerCount{Count: 7, EmittedAt: "2026-05-29T13:05:00Z"})
	h.handleViewerCount(context.Background(), payload)

	ev := <-client
	if ev.Name != "viewers" {
		t.Errorf("event name = %q, want viewers", ev.Name)
	}
	// First value: no flash class, count present.
	if !strings.Contains(ev.Data, `class="chatters-count"`) || !strings.Contains(ev.Data, ">7<") {
		t.Errorf("fragment %q missing unflashed count 7", ev.Data)
	}

	// A rise should carry the flash-up class.
	payload, _ = json.Marshal(eventbus.ViewerCount{Count: 9, EmittedAt: "2026-05-29T13:06:00Z"})
	h.handleViewerCount(context.Background(), payload)
	ev = <-client
	if !strings.Contains(ev.Data, "flash-up") || !strings.Contains(ev.Data, ">9<") {
		t.Errorf("fragment %q missing flash-up / count 9", ev.Data)
	}
}

func TestHub_handleViewerCount_badPayloadNoCrash(t *testing.T) {
	h := NewHub()
	h.handleViewerCount(context.Background(), []byte("not json"))
	// A bad payload must not flip viewersKnown or set a count.
	if h.viewersKnown {
		t.Errorf("bad payload should not establish a baseline")
	}
}

func TestRenderViewerCount(t *testing.T) {
	if got := renderViewerCount(5, ""); got != `<span class="chatters-count">5</span>` {
		t.Errorf("unflashed = %q", got)
	}
	if got := renderViewerCount(5, "up"); !strings.Contains(got, "chatters-count flash-up") {
		t.Errorf("up = %q, want flash-up class", got)
	}
	if got := renderViewerCount(5, "down"); !strings.Contains(got, "chatters-count flash-down") {
		t.Errorf("down = %q, want flash-down class", got)
	}
}

func TestHub_handleVideoChanged_broadcastsNowLine(t *testing.T) {
	h := NewHub()
	client := h.register()

	since := time.Date(2026, 5, 29, 13, 5, 0, 0, time.UTC)
	payload, _ := json.Marshal(eventbus.VideoChanged{
		File: "wy_0042.MP4", State: "Wyoming", Flagged: false,
		EmittedAt: since.Format(time.RFC3339Nano),
	})
	h.handleVideoChanged(context.Background(), payload)

	ev := <-client
	if ev.Name != "video" {
		t.Errorf("event name = %q, want video", ev.Name)
	}
	if !strings.Contains(ev.Data, "wy_0042.MP4") || !strings.Contains(ev.Data, "Wyoming") {
		t.Errorf("fragment %q missing file/state", ev.Data)
	}
	// elapsed span carries the clip start (emitted_at) as data-since so the JS
	// ticker counts up from the right moment.
	if !strings.Contains(ev.Data, `class="now-elapsed"`) ||
		!strings.Contains(ev.Data, `data-since="`+itoa(int(since.Unix()))+`"`) {
		t.Errorf("fragment %q missing now-elapsed with data-since=%d", ev.Data, since.Unix())
	}
}

func TestHub_handleVideoChanged_badPayloadNoCrash(t *testing.T) {
	h := NewHub()
	h.handleVideoChanged(context.Background(), []byte("not json"))
}

func TestRenderVideoLine_escapesHTML(t *testing.T) {
	got := renderVideoLine(eventbus.VideoChanged{File: "<script>x</script>", State: "Wyoming"})
	if strings.Contains(got, "<script>") {
		t.Errorf("file not escaped: %q", got)
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
