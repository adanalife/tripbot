package chatbot

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
)

// --- lifetimeMilesLeaderboardCmd ---
//
// Reads users.LifetimeMilesLeaderboard, which is a package-level [][]string
// hydrated at startup by users.InitLeaderboard. No DB on the read path.

func TestLifetimeMilesLeaderboardCmd_Empty(t *testing.T) {
	app := newTestApp(video.Video{})
	prev := users.LifetimeMilesLeaderboard
	users.LifetimeMilesLeaderboard = nil
	defer func() { users.LifetimeMilesLeaderboard = prev }()

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(newTestUser("viewer1"), nil)

	msg := out()
	if !strings.Contains(msg, "Top 0 lifetime miles") {
		t.Errorf("expected zero-size leaderboard header, got %q", msg)
	}
}

func TestLifetimeMilesLeaderboardCmd_WithUsers(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	prev := users.LifetimeMilesLeaderboard
	users.LifetimeMilesLeaderboard = [][]string{
		{"viewer1", "200.0"}, {"viewer2", "150.5"}, {"viewer3", "10.0"},
	}
	defer func() { users.LifetimeMilesLeaderboard = prev }()

	out, restore := captureSay(t)
	defer restore()

	app.lifetimeMilesLeaderboardCmd(newTestUser("caller"), nil)

	msg := out()
	if !strings.Contains(msg, "viewer1") || !strings.Contains(msg, "200.0mi") {
		t.Errorf("expected viewer1 with 200.0mi in output, got %q", msg)
	}
	if !strings.Contains(msg, "Top 3 lifetime miles") {
		t.Errorf("expected 'Top 3 lifetime miles' header, got %q", msg)
	}

	// confirm the overlay surface was driven with the same title + row count
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Total Miles", 3 rows)`) {
		t.Errorf("expected single ShowLeaderboard overlay call, got %v", rec.Calls)
	}
}

// --- monthlyMilesLeaderboardCmd ---
//
// scoreboards.TopUsers emits a JOIN across scores, scoreboards, and users.
// With sqlmock we just need to honor the one query the command makes.

func TestMonthlyMilesLeaderboardCmd_RendersTopUsers(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 42.5).
		AddRow("viewer2", 12.0)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyMilesLeaderboardCmd(newTestUser("caller"), nil)

	msg := out()
	if !strings.Contains(msg, "viewer1") || !strings.Contains(msg, "42.5") {
		t.Errorf("expected leaderboard row in output, got %q", msg)
	}
	if !strings.HasPrefix(msg, "Top 2 miles this month:") {
		t.Errorf("expected 'Top 2 miles this month:' prefix, got %q", msg)
	}
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Monthly Miles", 2 rows)`) {
		t.Errorf("expected one ShowLeaderboard overlay call with 2 rows, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- monthlyGuessLeaderboardCmd ---

func TestMonthlyGuessLeaderboardCmd_Empty_SaysNoneYet(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	// scoreboards.TopUsers returns an empty result set
	rows := sqlmock.NewRows([]string{"username", "value"})
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyGuessLeaderboardCmd(newTestUser("caller"), nil)

	if !strings.Contains(out(), "No one is on that leaderboard yet") {
		t.Errorf("expected empty-leaderboard message, got %q", out())
	}
	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call when leaderboard empty, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMonthlyGuessLeaderboardCmd_WithGuesses_StripsDecimals(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 7.0).
		AddRow("viewer2", 3.0)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	out, restore := captureSay(t)
	defer restore()

	app.monthlyGuessLeaderboardCmd(newTestUser("caller"), nil)

	msg := out()
	// guess scores are formatted as integers in the chat message
	if !strings.Contains(msg, "1. viewer1 (7)") {
		t.Errorf("expected integer-formatted guess count, got %q", msg)
	}
	if strings.Contains(msg, "7.0") {
		t.Errorf("decimals should be stripped, but found '7.0' in %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// --- milesCmd ---
//
// The interesting test paths require either zero queries (Find returns no
// rows → early-return) or the GetScore chain (3 queries for monthly miles).
// We cover the zero-query path here; the GetScore-chain variants are a
// follow-up once the test patterns mature.

func TestMilesCmd_OtherUser_NotInDB(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// users.Find runs a SELECT; returning no rows triggers gorm.ErrRecordNotFound
	// which Find translates into User{ID: 0}.
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("ghost", 1). // GORM appends the LIMIT 1 arg
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(newTestUser("caller"), []string{"ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_Self_WithMiles(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// Self-lookup skips users.Find but still runs the 3-query GetScore chain
	// for CurrentMonthlyMiles.
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("viewer1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 8.0))

	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "viewer1", Miles: 50.0}
	app.milesCmd(user, nil)

	msg := out()
	if !strings.Contains(msg, "@viewer1 has 8.00mi this month") {
		t.Errorf("expected monthly miles in self-lookup, got %q", msg)
	}
	if !strings.Contains(msg, "(50mi total)") {
		t.Errorf("expected lifetime total in self-lookup, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_Self_NewcomerHint(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// Brand-new user: monthly = 0, lifetime = 0 → triggers both newcomer hints.
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("newbie").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99))
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(100, 99, 7, 0.0))

	out, restore := captureSay(t)
	defer restore()

	user := &users.User{Username: "newbie", Miles: 0.0}
	app.milesCmd(user, nil)

	msg := out()
	if !strings.Contains(msg, "You'll earn more miles") {
		t.Errorf("expected newcomer hint, got %q", msg)
	}
	if !strings.Contains(msg, "takes a bit for me to notice you") {
		t.Errorf("expected zero-miles-specific hint, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// TestMilesCmd_OtherUser_Found walks the longer path: Find returns a row,
// then CurrentMonthlyMiles runs the GetScore chain (3 queries). Documents
// the GORM SQL shape these helpers emit so future tests can copy the pattern.
func TestMilesCmd_OtherUser_Found(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	// 1. users.Find — returns one row with miles=120
	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("viewer1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username", "miles"}).
			AddRow(42, "viewer1", 120.0))

	// 2. scoreboards.getUserIDByName — raw SELECT id by username
	mock.ExpectQuery(`SELECT id FROM users WHERE username = `).
		WithArgs("viewer1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	// 3. scoreboards.findOrCreateScoreboard — FirstOrCreate SELECT
	mock.ExpectQuery(`SELECT \* FROM "scoreboards" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(7, "miles_2026_05"))

	// 4. scoreboards.findOrCreateScore — FirstOrCreate SELECT for the score row
	mock.ExpectQuery(`SELECT \* FROM "scores" WHERE`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "scoreboard_id", "value"}).
			AddRow(99, 42, 7, 15.5))

	out, restore := captureSay(t)
	defer restore()

	app.milesCmd(newTestUser("caller"), []string{"viewer1"})

	msg := out()
	// monthly is 15.5, lifetime 120 → both should appear (rounded to int for total)
	if !strings.Contains(msg, "@viewer1 has 15.50mi this month") {
		t.Errorf("expected monthly miles line, got %q", msg)
	}
	if !strings.Contains(msg, "(120mi total)") {
		t.Errorf("expected lifetime miles in parens, got %q", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestMilesCmd_OtherUser_StripsAtSign(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})

	mock.ExpectQuery(`SELECT \* FROM "users" WHERE username = `).
		WithArgs("ghost", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "username"}))

	out, restore := captureSay(t)
	defer restore()

	// "@ghost" should be normalized to "ghost" before the lookup
	app.milesCmd(newTestUser("caller"), []string{"@ghost"})

	if !strings.Contains(out(), "I don't know them") {
		t.Errorf("expected unknown-user message, got %q", out())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
