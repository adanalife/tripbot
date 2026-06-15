package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

// withChatters swaps the chatters seam to a fixed set for the duration of a test.
func withChatters(t *testing.T, logins ...string) {
	t.Helper()
	saved := chatters
	set := make(map[string]struct{}, len(logins))
	for _, l := range logins {
		set[l] = struct{}{}
	}
	chatters = func() map[string]struct{} { return set }
	t.Cleanup(func() { chatters = saved })
}

func TestChattersHandler_JSON(t *testing.T) {
	withChatters(t, "charlie", "alice", "bob")

	r := mux.NewRouter()
	r.Handle("/api/chatters", http.HandlerFunc(chattersHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chatters", nil))

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var got chattersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	// sorted ascending so the console renders a stable order
	want := []string{"alice", "bob", "charlie"}
	if !reflect.DeepEqual(got.Chatters, want) {
		t.Errorf("chatters = %v, want %v", got.Chatters, want)
	}
	if got.Count != 3 {
		t.Errorf("count = %d, want 3", got.Count)
	}
}

func TestChattersHandler_Empty(t *testing.T) {
	withChatters(t)

	r := mux.NewRouter()
	r.Handle("/api/chatters", http.HandlerFunc(chattersHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chatters", nil))

	var got chattersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Count != 0 || len(got.Chatters) != 0 {
		t.Errorf("expected empty chatters, got %+v", got)
	}
	// must serialize as [] not null so the console can iterate unconditionally
	if !strings.Contains(rec.Body.String(), `"chatters":[]`) {
		t.Errorf("expected empty array, got: %s", rec.Body.String())
	}
}
