package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// flushRecorder is an http.ResponseWriter that satisfies the bits the SSE
// handler needs through http.ResponseController: Flush() (http.Flusher) and
// SetWriteDeadline. It buffers writes under a mutex and signals each write on a
// channel so the test can wait without racing the handler goroutine.
type flushRecorder struct {
	mu     sync.Mutex
	header http.Header
	buf    bytes.Buffer
	status int
	writes chan struct{}
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{header: make(http.Header), writes: make(chan struct{}, 100)}
}

func (f *flushRecorder) Header() http.Header { return f.header }
func (f *flushRecorder) WriteHeader(s int)   { f.status = s }
func (f *flushRecorder) Write(p []byte) (int, error) {
	f.mu.Lock()
	n, err := f.buf.Write(p)
	f.mu.Unlock()
	select {
	case f.writes <- struct{}{}:
	default:
	}
	return n, err
}

// Flush satisfies http.Flusher so http.ResponseController(f).Flush() works.
func (f *flushRecorder) Flush() {}

// SetWriteDeadline satisfies the deadline interface ResponseController looks for
// — exercises the handler's happy path (deadline cleared for the SSE stream).
func (f *flushRecorder) SetWriteDeadline(time.Time) error { return nil }

func (f *flushRecorder) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.buf.String()
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestEventsHandler_streamsChatEvent(t *testing.T) {
	rec := newFlushRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/admin/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		eventsHandler(rec, req)
		close(done)
	}()

	// Wait until the handler has registered with the hub, so our broadcast
	// isn't dropped before the client channel exists.
	waitFor(t, func() bool { return eventHub.numSubscribers() >= 1 })

	eventHub.broadcast(sseEvent{Name: "chat", Data: `<div class="chat-line">hi</div>`})

	select {
	case <-rec.writes:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event write")
	}

	cancel()
	<-done // handler returns on ctx cancel and unregisters

	body := rec.String()
	if !strings.Contains(body, "event: chat\n") {
		t.Errorf("missing named event line in %q", body)
	}
	if !strings.Contains(body, "chat-line") {
		t.Errorf("missing data fragment in %q", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if eventHub.numSubscribers() != 0 {
		t.Errorf("subscriber not cleaned up after disconnect: %d", eventHub.numSubscribers())
	}
}

// TestEventsHandler_flattensNewlines guards SSE framing: a data fragment with a
// stray newline must not break the "event:/data:" framing.
func TestEventsHandler_flattensNewlines(t *testing.T) {
	rec := newFlushRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/admin/events", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		eventsHandler(rec, req)
		close(done)
	}()
	waitFor(t, func() bool { return eventHub.numSubscribers() >= 1 })

	eventHub.broadcast(sseEvent{Name: "chat", Data: "line1\nline2"})
	select {
	case <-rec.writes:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
	cancel()
	<-done

	body := rec.String()
	if strings.Contains(body, "line1\nline2") {
		t.Errorf("newline not flattened in data: %q", body)
	}
	if !strings.Contains(body, "data: line1 line2") {
		t.Errorf("expected flattened data line, got %q", body)
	}
}
