package events

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

// GuessRecord counts only the caller's own guess_submitted rows, and reads the
// correct flag whether the writer stored a JSON boolean or the string "true".
func TestGuessRecord(t *testing.T) {
	db := testdb.New(t)

	seed := func(username, meta string) {
		t.Helper()
		err := db.Exec(`INSERT INTO events (platform, username, event, meta, date_created)
		                VALUES ('twitch', ?, 'guess_submitted', ?, now())`, username, meta).Error
		if err != nil {
			t.Fatalf("seed guess for %s: %v", username, err)
		}
	}

	seed("gr_alice", `{"guessed":"utah","actual":"utah","correct":true}`)
	seed("gr_alice", `{"guessed":"utah","actual":"utah","correct":"true"}`)
	seed("gr_alice", `{"guessed":"ohio","actual":"utah","correct":false}`)
	seed("gr_bob", `{"guessed":"utah","actual":"utah","correct":true}`)
	// A non-guess row of alice's must not reach the denominator.
	if err := db.Exec(`INSERT INTO events (platform, username, event, date_created)
	                   VALUES ('twitch', 'gr_alice', 'login', now())`).Error; err != nil {
		t.Fatalf("seed login: %v", err)
	}

	total, correct := GuessRecord(context.Background(), "twitch", "gr_alice")
	if total != 3 || correct != 2 {
		t.Errorf("GuessRecord = (%d, %d), want (3, 2)", total, correct)
	}

	// A viewer with no guesses reads as an empty record, not an error.
	if total, correct := GuessRecord(context.Background(), "twitch", "gr_nobody"); total != 0 || correct != 0 {
		t.Errorf("GuessRecord for a stranger = (%d, %d), want (0, 0)", total, correct)
	}

	// Platform scopes the read: alice's twitch rows are not youtube's.
	if total, _ := GuessRecord(context.Background(), "youtube", "gr_alice"); total != 0 {
		t.Errorf("GuessRecord on the wrong platform = %d, want 0", total)
	}
}
