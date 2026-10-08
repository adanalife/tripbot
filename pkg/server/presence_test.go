package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

func TestPresenceInsightsHandler(t *testing.T) {
	db := testdb.New(t)
	seedUser(t, db, "pr_alice", false)
	seedUser(t, db, "pr_farm", false)
	seedUser(t, db, "pr_bot", true)
	if err := db.Exec(`UPDATE users SET exclude_from_leaderboard = true WHERE username = 'pr_alice'`).Error; err != nil {
		t.Fatalf("flag pr_alice: %v", err)
	}
	now := time.Now()

	seedSession(t, db, "twitch", "pr_alice", now.Add(-48*time.Hour), 120)
	seedSession(t, db, "twitch", "pr_alice", now.Add(-24*time.Hour), 180)
	// Began a day before the window and ended 12h ago: 156h inside it, 180h long.
	seedSession(t, db, "twitch", "pr_farm", now.Add(-8*24*time.Hour), 180*60)
	seedSession(t, db, "twitch", "pr_bot", now.Add(-30*time.Hour), 20*60)
	// Left out: an open login and a session wholly before the window.
	seedSession(t, db, "twitch", "pr_alice", now.Add(-time.Hour), -1)
	seedSession(t, db, "twitch", "pr_alice", now.Add(-10*24*time.Hour), 60)

	rec := insightsGET(t, "/api/insights/presence", presenceInsightsHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got presenceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []presenceRow{
		{Platform: "twitch", Username: "pr_farm", PresenceHours: 156, Sessions: 1, LongestSessionHours: 180},
		{Platform: "twitch", Username: "pr_bot", PresenceHours: 20, Sessions: 1, LongestSessionHours: 20, IsBot: true},
		{Platform: "twitch", Username: "pr_alice", PresenceHours: 5, Sessions: 2, LongestSessionHours: 3, ExcludeFromLeaderboard: true},
	}
	if got.Days != 7 || len(got.Accounts) != len(want) {
		t.Fatalf("got %+v, want days=7 and %d accounts", got, len(want))
	}
	for i := range want {
		if got.Accounts[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got.Accounts[i], want[i])
		}
	}

	rec = insightsGET(t, "/api/insights/presence?limit=1", presenceInsightsHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Accounts) != 1 || got.Accounts[0].Username != "pr_farm" {
		t.Errorf("limit=1 returned %+v, want only pr_farm", got.Accounts)
	}
}

// stubFlagger records the flag writes it receives; a username of "ghost" has
// no row.
type stubFlagger struct{ got []string }

func (f *stubFlagger) write(username, flag string, v bool) error {
	if username == "ghost" {
		return gorm.ErrRecordNotFound
	}
	f.got = append(f.got, username+" "+flag+"="+map[bool]string{true: "true", false: "false"}[v])
	return nil
}

func (f *stubFlagger) SetBot(_ context.Context, u string, v bool) error {
	return f.write(u, "is_bot", v)
}

func (f *stubFlagger) SetExcludeFromLeaderboard(_ context.Context, u string, v bool) error {
	return f.write(u, "exclude_from_leaderboard", v)
}

func TestUserFlagsHandler(t *testing.T) {
	post := func(s *Server, username, body string) *httptest.ResponseRecorder {
		r := mux.NewRouter()
		r.Handle("/api/user/{username}/flags", http.HandlerFunc(s.userFlagsHandler)).Methods("POST")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/user/"+username+"/flags", strings.NewReader(body)))
		return rec
	}

	if rec := post(New(testConf), "farm", `{"is_bot":true}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("unwired: status = %d, want 503", rec.Code)
	}

	f := &stubFlagger{}
	s := New(testConf)
	s.SetUserFlags(f)
	for _, body := range []string{``, `{}`, `{"is_bot":"yes"}`} {
		if rec := post(s, "farm", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
	if rec := post(s, "ghost", `{"is_bot":true}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown user: status = %d, want 404", rec.Code)
	}

	rec := post(s, "Farm", `{"is_bot":true,"exclude_from_leaderboard":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	want := []string{"farm is_bot=true", "farm exclude_from_leaderboard=false"}
	if strings.Join(f.got, ",") != strings.Join(want, ",") {
		t.Errorf("writes = %v, want %v", f.got, want)
	}
	for _, field := range []string{`"is_bot":true`, `"exclude_from_leaderboard":false`, `"username":"farm"`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Errorf("body missing %s: %s", field, rec.Body.String())
		}
	}
}
