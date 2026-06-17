package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/users"
	"github.com/gorilla/mux"
)

// withProfileSeams stubs the DB-backed data reads so the handler renders without
// a real database.
func withProfileSeams(t *testing.T, u users.User, sessions int64, monthly float32) {
	t.Helper()
	savedFind, savedCount, savedMonthly := findUser, sessionCount, monthlyMiles
	t.Cleanup(func() { findUser, sessionCount, monthlyMiles = savedFind, savedCount, savedMonthly })
	findUser = func(context.Context, string) users.User { return u }
	sessionCount = func(context.Context, string) int64 { return sessions }
	monthlyMiles = func(context.Context, users.User) float32 { return monthly }
}

func TestUserProfileAPIHandler_JSON(t *testing.T) {
	withProfileSeams(t, users.User{
		ID:          42,
		Username:    "danalol",
		Miles:       123.0,
		DateCreated: time.Date(2019, 5, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:    time.Date(2026, 5, 29, 13, 5, 0, 0, time.UTC),
	}, 87, 42.0)

	r := mux.NewRouter()
	r.Handle("/api/user/{username}", http.HandlerFunc(userProfileAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/danalol", nil))

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var got userProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if !got.Found || got.Username != "danalol" || got.Miles != 123.0 ||
		got.MonthlyMiles != 42.0 || got.Sessions != 87 {
		t.Errorf("unexpected profile: %+v", got)
	}
	// snake_case wire format the console reads.
	if !strings.Contains(rec.Body.String(), `"monthly_miles"`) {
		t.Errorf("expected snake_case keys: %s", rec.Body.String())
	}
}

func TestUserProfileAPIHandler_NotFound(t *testing.T) {
	withProfileSeams(t, users.User{ID: 0}, 0, 0)
	r := mux.NewRouter()
	r.Handle("/api/user/{username}", http.HandlerFunc(userProfileAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/ghost", nil))

	var got userProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Found || got.Username != "ghost" {
		t.Errorf("expected not-found ghost, got %+v", got)
	}
}

func TestUserProfileAPIHandler_BotFlag(t *testing.T) {
	withProfileSeams(t, users.User{ID: 7, Username: "tripbot4000", IsBot: true}, 3, 0)
	r := mux.NewRouter()
	r.Handle("/api/user/{username}", http.HandlerFunc(userProfileAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/tripbot4000", nil))

	var got userProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Found || !got.IsBot {
		t.Errorf("expected found bot, got %+v", got)
	}
}
