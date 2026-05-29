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
func withProfileSeams(t *testing.T, u users.User, sessions int64) {
	t.Helper()
	savedFind, savedCount := findUser, sessionCount
	t.Cleanup(func() { findUser, sessionCount = savedFind, savedCount })
	findUser = func(context.Context, string) users.User { return u }
	sessionCount = func(context.Context, string) int64 { return sessions }
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
	}, 87)

	body := renderProfile(t, "danalol")
	for _, want := range []string{
		"danalol",
		"123.0",      // miles
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
	withProfileSeams(t, users.User{ID: 0}, 0)

	body := renderProfile(t, "ghost")
	if !strings.Contains(body, "no record") {
		t.Errorf("expected empty state, got %q", body)
	}
	if !strings.Contains(body, "ghost") {
		t.Errorf("empty card should still name the user")
	}
}

func TestUserProfileHandler_BotBadge(t *testing.T) {
	withProfileSeams(t, users.User{ID: 7, Username: "tripbot4000", IsBot: true}, 3)
	if body := renderProfile(t, "tripbot4000"); !strings.Contains(body, "profile-bot") {
		t.Errorf("expected bot badge, got %q", body)
	}
}
