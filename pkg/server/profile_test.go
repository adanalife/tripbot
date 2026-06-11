package server

import (
	"context"
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
		"123.00",     // lifetime miles
		"42.00",      // this month
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

// TestUserProfileHandler_FloorsZeroMiles covers a brand-new viewer with no
// accrued miles: the displayed values floor at 0.01 instead of "0.00", which
// reads as broken. Display-only — the stored value is unchanged.
func TestUserProfileHandler_FloorsZeroMiles(t *testing.T) {
	withProfileSeams(t, users.User{ID: 11, Username: "newbie", Miles: 0}, 1, 0)
	body := renderProfile(t, "newbie")
	if strings.Contains(body, ">0.00<") {
		t.Errorf("brand-new viewer should not render 0.00 miles: %q", body)
	}
	if !strings.Contains(body, ">0.01<") {
		t.Errorf("expected floored 0.01 miles, got %q", body)
	}
}

func TestFloorDisplayMiles(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{0, 0.01},
		{0.004, 0.01},
		{0.01, 0.01},
		{5.5, 5.5},
	}
	for _, tc := range cases {
		if got := floorDisplayMiles(tc.in); got != tc.want {
			t.Errorf("floorDisplayMiles(%v) = %v, want %v", tc.in, got, tc.want)
		}
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
