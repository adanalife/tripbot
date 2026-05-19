package httpmw

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/urfave/negroni/v3"
)

func TestRecoveryCatchesPanic(t *testing.T) {
	var called bool
	var recovered any
	rec := NewRecovery(func(r any) {
		called = true
		recovered = r
	})

	n := negroni.New(rec)
	n.UseHandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})

	w := httptest.NewRecorder()
	n.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !called {
		t.Fatal("OnPanic was not invoked")
	}
	if got, ok := recovered.(string); !ok || got != "kaboom" {
		t.Fatalf("OnPanic got %v, want \"kaboom\"", recovered)
	}
}

func TestRecoveryPassThrough(t *testing.T) {
	rec := NewRecovery(func(any) { t.Fatal("OnPanic should not fire when there's no panic") })

	n := negroni.New(rec)
	n.UseHandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	n.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestRecoveryNilCallback(t *testing.T) {
	rec := NewRecovery(nil)

	n := negroni.New(rec)
	n.UseHandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})

	w := httptest.NewRecorder()
	n.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil OnPanic should still recover)", w.Code)
	}
}
