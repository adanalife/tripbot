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
// a real database. findUserByID defaults to a miss — the id path is opt-in per
// test, matching the deployed state where most rows aren't stamped yet.
func withProfileSeams(t *testing.T, u users.User, sessions int64, monthly float32) {
	t.Helper()
	savedFind, savedCount, savedMonthly, savedEarliest := findUser, sessionCount, monthlyMiles, earliestEvent
	savedFindByID := findUserByID
	t.Cleanup(func() {
		findUser, sessionCount, monthlyMiles, earliestEvent = savedFind, savedCount, savedMonthly, savedEarliest
		findUserByID = savedFindByID
	})
	findUser = func(context.Context, string, string) (users.User, error) {
		// mirror pkg/users.Find's contract: a staged zero-ID user means "no row"
		if u.ID == 0 {
			return users.User{}, gorm.ErrRecordNotFound
		}
		return u, nil
	}
	findUserByID = func(context.Context, string, string) (users.User, error) {
		return users.User{}, gorm.ErrRecordNotFound
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

// profileJSONByID runs the handler with both identifiers, the way the console
// sends them once it has an id to send.
func profileJSONByID(t *testing.T, username, userID string) userProfile {
	t.Helper()
	r := mux.NewRouter()
	r.Handle("/api/user/{username}", http.HandlerFunc(New(testConf).userProfileAPIHandler)).Methods("GET")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/user/"+username+"?user_id="+userID, nil))
	var got userProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	return got
}

// The rename case this whole change exists for: the viewer now calls themselves
// something else, so the name they're requested under finds nothing, but their
// id still points at the row holding their miles and history.
func TestUserProfileAPIHandler_IDResolvesAcrossARename(t *testing.T) {
	// No row under the new name...
	withProfileSeams(t, users.User{ID: 0}, 0, 0)
	// ...but the id still finds the row, which stores the old name.
	findUserByID = func(context.Context, string, string) (users.User, error) {
		return users.User{ID: 42, Username: "oldname", Miles: 500.0}, nil
	}

	got := profileJSONByID(t, "newname", "12345")
	if !got.Found {
		t.Fatalf("expected the id to find the renamed user, got %+v", got)
	}
	if got.Miles != 500.0 {
		t.Errorf("miles = %v, want the row's 500", got.Miles)
	}
	// The stored name is reported, not the one asked for — it's the name the
	// events were written under, and what the row actually holds.
	if got.Username != "oldname" {
		t.Errorf("username = %q, want the stored %q", got.Username, "oldname")
	}
}

// An id nobody is stamped with yet must not shadow a perfectly good username —
// that's the state of every row until its owner next speaks.
func TestUserProfileAPIHandler_UnstampedIDFallsBackToUsername(t *testing.T) {
	withProfileSeams(t, users.User{ID: 42, Username: "danalol", Miles: 123.0}, 5, 0)
	got := profileJSONByID(t, "danalol", "12345")
	if !got.Found || got.Username != "danalol" || got.Miles != 123.0 {
		t.Errorf("expected the username fallback to answer, got %+v", got)
	}
}

// The downstream event reads are keyed by name, so they must use the row's name
// rather than the requested one — otherwise resolving by id after a rename
// reports a real user with zero sessions.
func TestUserProfileAPIHandler_EventReadsUseTheStoredUsername(t *testing.T) {
	withProfileSeams(t, users.User{ID: 0}, 0, 0)
	findUserByID = func(context.Context, string, string) (users.User, error) {
		return users.User{ID: 42, Username: "oldname"}, nil
	}
	var sessionArg, earliestArg string
	sessionCount = func(_ context.Context, _ string, username string) int64 {
		sessionArg = username
		return 87
	}
	earliestEvent = func(_ context.Context, _ string, username string) time.Time {
		earliestArg = username
		return time.Time{}
	}

	got := profileJSONByID(t, "newname", "12345")
	if sessionArg != "oldname" || earliestArg != "oldname" {
		t.Errorf("event reads keyed by (sessions=%q, earliest=%q), want the stored %q",
			sessionArg, earliestArg, "oldname")
	}
	if got.Sessions != 87 {
		t.Errorf("sessions = %d, want 87", got.Sessions)
	}
}

// Neither identifier is not a lookup for "some user" — it's a miss.
func TestUserProfileAPIHandler_NoIdentifiersIsNotFound(t *testing.T) {
	withProfileSeams(t, users.User{ID: 42, Username: "danalol"}, 5, 0)
	called := false
	findUser = func(context.Context, string, string) (users.User, error) {
		called = true
		return users.User{ID: 42, Username: "danalol"}, nil
	}
	// An empty {username} can't route, so exercise the gather directly.
	got := gatherUserProfile(context.Background(), testConf.Platform, "", "")
	if got.Found {
		t.Errorf("expected not-found with neither identifier, got %+v", got)
	}
	if called {
		t.Error("looked the user up with no identifier to look up by")
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
