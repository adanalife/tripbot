package scoreboards

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"gorm.io/gorm"
)

// insertGuess writes one guess_submitted row the way events.GuessSubmitted does.
func insertGuess(t *testing.T, db *gorm.DB, username, guessed, actual string, correct bool, at time.Time) {
	t.Helper()
	meta := fmt.Sprintf(`{"guessed":%q,"actual":%q,"correct":%t}`, guessed, actual, correct)
	err := db.Exec(`INSERT INTO events (platform, username, event, meta, date_created)
	                VALUES ('twitch', ?, 'guess_submitted', ?, ?)`, username, meta, at).Error
	if err != nil {
		t.Fatalf("insert guess for %s: %v", username, err)
	}
}

// insertVisit writes a login/logout pair sharing session n, minutes apart.
func insertVisit(t *testing.T, db *gorm.DB, username string, n int, at time.Time, minutes int) {
	t.Helper()
	session := fmt.Sprintf("00000000-0000-0000-0000-%012d", n)
	for _, ev := range []struct {
		event string
		at    time.Time
	}{{"login", at}, {"logout", at.Add(time.Duration(minutes) * time.Minute)}} {
		err := db.Exec(`INSERT INTO events (platform, username, event, session_id, date_created)
		                VALUES ('twitch', ?, ?, ?::uuid, ?)`, username, ev.event, session, ev.at).Error
		if err != nil {
			t.Fatalf("insert %s for %s: %v", ev.event, username, err)
		}
	}
}

// TestViewerWrapped covers one viewer's month end to end: the guess tally and
// distinct states, the most-missed state with the name it got instead, the
// shared-rank place on the monthly board, and the visit minutes — with last
// month's rows, another viewer's rows and an open visit all left out.
func TestViewerWrapped(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	month := time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC)
	day := func(d int, h int) time.Time { return month.AddDate(0, 0, d-1).Add(time.Duration(h) * time.Hour) }

	alice := createUser(t, db, "alice", "twitch", false)
	bob := createUser(t, db, "bob", "twitch", false)
	carol := createUser(t, db, "carol", "twitch", false)
	board := createScoreboardOn(t, db, "guess_state_2026_10", "twitch")
	insertScore(t, db, alice, board, 3)
	insertScore(t, db, bob, board, 5)
	insertScore(t, db, carol, board, 3)

	insertGuess(t, db, "alice", "Nevada", "Nevada", true, day(2, 10))
	insertGuess(t, db, "alice", "Utah", "Utah", true, day(3, 10))
	insertGuess(t, db, "alice", "Utah", "Utah", true, day(4, 10))
	insertGuess(t, db, "alice", "New Hampshire", "Vermont", false, day(5, 10))
	insertGuess(t, db, "alice", "New Hampshire", "Vermont", false, day(6, 10))
	insertGuess(t, db, "alice", "Maine", "Vermont", false, day(7, 10))
	insertGuess(t, db, "alice", "Vermont", "Maine", false, day(8, 10))
	// last month, and someone else's: neither counts
	insertGuess(t, db, "alice", "Ohio", "Ohio", true, month.AddDate(0, 0, -1))
	insertGuess(t, db, "bob", "Texas", "Texas", true, day(9, 10))

	insertVisit(t, db, "alice", 1, day(2, 9), 30)
	insertVisit(t, db, "alice", 2, day(3, 9), 60)
	insertVisit(t, db, "alice", 3, month.AddDate(0, 0, -2), 500)
	// an open visit: login with no logout
	if err := db.Exec(`INSERT INTO events (platform, username, event, session_id, date_created)
	                   VALUES ('twitch', 'alice', 'login', '00000000-0000-0000-0000-000000000042', ?)`, day(10, 9)).Error; err != nil {
		t.Fatal(err)
	}

	w, err := ViewerWrapped(ctx, testConf, "alice", day(15, 12))
	if err != nil {
		t.Fatalf("ViewerWrapped: %v", err)
	}
	want := Wrapped{
		Month: month, Guesses: 7, Correct: 3, States: 2,
		Rank: 2, Players: 3,
		MissedMost: "Vermont", CalledIt: "New Hampshire",
		Minutes: 90,
	}
	if w != want {
		t.Errorf("ViewerWrapped(alice) = %+v, want %+v", w, want)
	}

	// A viewer with nothing this month gets zeros and the month, not an error.
	w, err = ViewerWrapped(ctx, testConf, "stranger", day(15, 12))
	if err != nil {
		t.Fatalf("ViewerWrapped(stranger): %v", err)
	}
	if w != (Wrapped{Month: month, Players: 3}) {
		t.Errorf("ViewerWrapped(stranger) = %+v, want zeros with Players 3", w)
	}
}
