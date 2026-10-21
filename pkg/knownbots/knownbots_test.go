package knownbots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// registry serves one canned answer per request, in order, repeating the last.
func registry(t *testing.T, answers ...func(http.ResponseWriter)) string {
	t.Helper()
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answers[min(n, len(answers)-1)](w)
		n++
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func body(s string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { _, _ = w.Write([]byte(s)) }
}

func status(code int) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

// The registry's real shape: [name, channel count, last-seen unix time].
const listed = `{"bots":[["commanderroot",152,1715883379],["StreamElements",9001,1715883000],[],[7,1,1],[""]],"_total":5}`

func TestRefreshNamesTheListedBots(t *testing.T) {
	l := New(registry(t, body(listed)))
	if l.IsKnownBot("commanderroot") {
		t.Fatal("an empty list named a bot before any refresh")
	}
	if err := l.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	for _, name := range []string{"commanderroot", "CommanderRoot", "streamelements"} {
		if !l.IsKnownBot(name) {
			t.Errorf("%q is listed but not known", name)
		}
	}
	if l.IsKnownBot("adanalife_") {
		t.Error("an unlisted viewer was called a bot")
	}
	// The malformed entries are skipped, not fatal.
	if l.Len() != 2 {
		t.Errorf("Len = %d, want 2", l.Len())
	}
}

// A failed or empty refresh keeps what the list already held: an outage must
// not turn every bot back into a person.
func TestRefreshKeepsThePreviousListOnFailure(t *testing.T) {
	l := New(registry(t,
		body(listed),
		status(http.StatusBadGateway),
		body(`{"bots":[],"_total":0}`),
		body(`<html>`),
	))
	if err := l.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	for i := range 3 {
		if err := l.Refresh(context.Background()); err == nil {
			t.Errorf("refresh %d reported success", i+2)
		}
		if !l.IsKnownBot("commanderroot") {
			t.Fatalf("refresh %d dropped the previous list", i+2)
		}
	}
}

func TestRefreshReplacesTheList(t *testing.T) {
	l := New(registry(t, body(listed), body(`{"bots":[["newbot",3,1]]}`)))
	for range 2 {
		if err := l.Refresh(context.Background()); err != nil {
			t.Fatalf("Refresh: %v", err)
		}
	}
	if l.IsKnownBot("commanderroot") || !l.IsKnownBot("newbot") {
		t.Error("a refresh merged into the old list instead of replacing it")
	}
}
