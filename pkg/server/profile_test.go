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
	savedFind, savedCount, savedMonthly, savedEarliest := findUser, sessionCount, monthlyMiles, earliestEvent
	t.Cleanup(func() {
		findUser, sessionCount, monthlyMiles, earliestEvent = savedFind, savedCount, savedMonthly, savedEarliest
	})
	findUser = func(context.Context, string) users.User { return u }
	sessionCount = func(context.Context, string) int64 { return sessions }
	monthlyMiles = func(context.Context, users.User) float32 { return monthly }
	// default: no surviving event history. Tests exercising the first-seen
	// fallback override earliestEvent after calling withProfileSeams.
	earliestEvent = func(context.Context, string, string) time.Time { return time.Time{} }
}

func renderProfile(t *testing.T, username string) string {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/admin/user/{username}", http.HandlerFunc(userProfileHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/user/"+username, nil))
	return rec.Body.String()
}

func TestUserProfileHandler_Found(t *testing.T) {
	withProfileSeams(t, users.User{
		ID:          42,
		Username:    "danalol",
		Miles:       123.0,
		IsBot:       false,
		DateCreated: time.Date(2019, 5, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:    time.Date(2026, 5, 29, 13, 5, 0, 0, time.UTC),
	}, 87, 42.0)

	body := renderProfile(t, "danalol")
	for _, want := range []string{
		"danalol",
		"123.0",      // lifetime miles
		"42.0",       // this month
		">87<",       // sessions
		"2019-05-01", // first seen
		`href="https://twitch.tv/danalol"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
	if strings.Contains(body, "no record") {
		t.Errorf("found user should not render the empty state")
	}
}

func TestUserProfileHandler_NotFound(t *testing.T) {
	withProfileSeams(t, users.User{ID: 0}, 0, 0)

	body := renderProfile(t, "ghost")
	if !strings.Contains(body, "no record") {
		t.Errorf("expected empty state, got %q", body)
	}
	if !strings.Contains(body, "ghost") {
		t.Errorf("empty card should still name the user")
	}
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

func TestUserProfileHandler_BotBadge(t *testing.T) {
	withProfileSeams(t, users.User{ID: 7, Username: "tripbot4000", IsBot: true}, 3, 0)
	if body := renderProfile(t, "tripbot4000"); !strings.Contains(body, "profile-bot") {
		t.Errorf("expected bot badge, got %q", body)
	}
}

// TestUserProfileHandler_UnknownDates covers a found user with no event history
// (zero-value timestamps — e.g. a freshly-seeded account): show "unknown", not
// Go's 0001-01-01 zero time.
func TestUserProfileHandler_UnknownDates(t *testing.T) {
	withProfileSeams(t, users.User{ID: 9, Username: "freshacct"}, 0, 0)
	body := renderProfile(t, "freshacct")
	if strings.Contains(body, "0001") {
		t.Errorf("zero time should not render as 0001-...: %q", body)
	}
	if !strings.Contains(body, "unknown") {
		t.Errorf("expected 'unknown' for zero-value seen dates: %q", body)
	}
}

// TestUserProfileHandler_FirstSeenFallback covers an account created during the
// date_created bug window: its User row dates are zero, but a surviving real
// event reconstructs first-seen. The earliest such event should win.
func TestUserProfileHandler_FirstSeenFallback(t *testing.T) {
	withProfileSeams(t, users.User{ID: 11, Username: "olduser"}, 5, 0)
	earliestEvent = func(context.Context, string, string) time.Time {
		return time.Date(2021, 3, 14, 0, 0, 0, 0, time.UTC)
	}
	body := renderProfile(t, "olduser")
	if !strings.Contains(body, "first seen</dt><dd>2021-03-14") {
		t.Errorf("expected event-derived first seen 2021-03-14, got %q", body)
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
	body := renderProfile(t, "veteran")
	if !strings.Contains(body, "2019-06-02") {
		t.Errorf("expected earliest (event) first seen 2019-06-02, got %q", body)
	}
}
