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
	"gorm.io/gorm"
)

// withProfileSeams stubs the DB-backed data reads so the handler renders without
// a real database.
func withProfileSeams(t *testing.T, u users.User, sessions int64, monthly float32) {
	t.Helper()
	savedFind, savedCount, savedMonthly, savedEarliest := findUser, sessionCount, monthlyMiles, earliestEvent
	t.Cleanup(func() {
		findUser, sessionCount, monthlyMiles, earliestEvent = savedFind, savedCount, savedMonthly, savedEarliest
	})
	findUser = func(context.Context, string, string) (users.User, error) {
		// mirror pkg/users.Find's contract: a staged zero-ID user means "no row"
		if u.ID == 0 {
			return users.User{}, gorm.ErrRecordNotFound
		}
		return u, nil
	}
	sessionCount = func(context.Context, string, string) int64 { return sessions }
	monthlyMiles = func(context.Context, users.User) float32 { return monthly }
	// default: no surviving event history. Tests exercising the first-seen
	// fallback override earliestEvent after calling withProfileSeams.
	earliestEvent = func(context.Context, string, string) time.Time { return time.Time{} }
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
	r.Handle("/api/user/{username}", http.HandlerFunc(New(testConf).userProfileAPIHandler)).Methods("GET")
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
	r.Handle("/api/user/{username}", http.HandlerFunc(New(testConf).userProfileAPIHandler)).Methods("GET")
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
	r.Handle("/api/user/{username}", http.HandlerFunc(New(testConf).userProfileAPIHandler)).Methods("GET")
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

// profileJSON runs the JSON profile handler for a username and returns the
// decoded payload. (Replaces the removed HTML renderProfile helper — the admin
// panel's HTML popover is gone; the surviving surface is the JSON endpoint.)
func profileJSON(t *testing.T, username string) userProfile {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/user/{username}", http.HandlerFunc(New(testConf).userProfileAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/user/"+username, nil))
	var got userProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return got
}

// TestUserProfileHandler_FirstSeenFallback covers an account created during the
// date_created bug window: its User row dates are zero, but a surviving real
// event reconstructs first-seen. The earliest such event should win.
func TestUserProfileHandler_FirstSeenFallback(t *testing.T) {
	withProfileSeams(t, users.User{ID: 11, Username: "olduser"}, 5, 0)
	earliestEvent = func(context.Context, string, string) time.Time {
		return time.Date(2021, 3, 14, 0, 0, 0, 0, time.UTC)
	}
	want := time.Date(2021, 3, 14, 0, 0, 0, 0, time.UTC)
	if got := profileJSON(t, "olduser"); !got.FirstSeen.Equal(want) {
		t.Errorf("first seen = %v, want event-derived %v", got.FirstSeen, want)
	}
}

// TestUserProfileHandler_FirstSeenPrefersEarliest covers a user whose row has a
// real DateCreated but whose earliest event predates it — first-seen takes the
// earlier of the two.
func TestUserProfileHandler_FirstSeenPrefersEarliest(t *testing.T) {
	withProfileSeams(t, users.User{
		ID:          12,
		Username:    "veteran",
		DateCreated: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	}, 9, 0)
	earliestEvent = func(context.Context, string, string) time.Time {
		return time.Date(2019, 6, 2, 0, 0, 0, 0, time.UTC)
	}
	want := time.Date(2019, 6, 2, 0, 0, 0, 0, time.UTC)
	if got := profileJSON(t, "veteran"); !got.FirstSeen.Equal(want) {
		t.Errorf("first seen = %v, want earliest %v", got.FirstSeen, want)
	}
}
