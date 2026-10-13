package events

import (
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

// The events table rejects every mutation but INSERT, at the database, so no
// code path (or hand-run query) can rewrite the history users.miles and the
// rollups derive from. Each attempt runs under a savepoint: a raised error
// aborts the enclosing test transaction otherwise.
func TestEventsTableIsAppendOnly(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO events (username, event) VALUES ('appendonly', 'login')`).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	for _, stmt := range []string{
		`UPDATE events SET event = 'logout' WHERE username = 'appendonly'`,
		`DELETE FROM events WHERE username = 'appendonly'`,
		`TRUNCATE events`,
	} {
		db.SavePoint("mutation")
		err := db.Exec(stmt).Error
		db.RollbackTo("mutation")
		if err == nil || !strings.Contains(err.Error(), "events is append-only") {
			t.Errorf("%s: err = %v, want the append-only rejection", stmt, err)
		}
	}
}
