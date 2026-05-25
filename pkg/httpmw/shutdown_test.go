package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownHandler_Responds202AndFiresSignal(t *testing.T) {
	savedSignal, savedDelay := ShutdownSignal, ShutdownDelay
	t.Cleanup(func() { ShutdownSignal, ShutdownDelay = savedSignal, savedDelay })

	fired := make(chan struct{}, 1)
	ShutdownSignal = func() error {
		fired <- struct{}{}
		return nil
	}
	ShutdownDelay = 5 * time.Millisecond

	rec := httptest.NewRecorder()
	ShutdownHandler()(rec, httptest.NewRequest(http.MethodPost, "/admin/shutdown", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusAccepted)
	}

	select {
	case <-fired:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("shutdown signal was not fired within timeout")
	}
}
