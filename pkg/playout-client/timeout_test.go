package playoutClient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A playout that accepts the connection and never answers must not hold the
// caller's goroutine: no caller on this path supplies a deadline, so the
// client's own Timeout is the only thing that ends the call.
func TestGetTimesOutOnHangingServer(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer srv.Close()
	// Declared after the Close defer so it runs first: Close waits for the
	// outstanding handler, which only returns once this channel is closed.
	defer close(block)

	c := New("unused.invalid:0", nil, "test", "twitch")
	if c.httpClient.Timeout != requestTimeout {
		t.Fatalf("New left the client unbounded: Timeout = %v, want %v", c.httpClient.Timeout, requestTimeout)
	}
	// Shortened so the test finishes in milliseconds; the wiring above is what
	// asserts the production bound.
	c.httpClient.Timeout = 100 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := c.get(context.Background(), srv.URL)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hanging server returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("get blocked past the client timeout")
	}
}
