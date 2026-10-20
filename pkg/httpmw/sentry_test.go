package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/urfave/negroni/v3"
)

// The servers put Sentry inside Recovery; a handler panic must still reach
// Recovery so it answers 500 and fires OnPanic.
func TestSentryInsideRecoveryStillAnswers500(t *testing.T) {
	var panics int
	n := negroni.New(NewRecovery(func(any) { panics++ }), NewSentry())
	n.UseHandlerFunc(func(http.ResponseWriter, *http.Request) { panic("kaboom") })

	w := httptest.NewRecorder()
	n.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if panics != 1 {
		t.Fatalf("OnPanic fired %d times, want 1", panics)
	}
}
