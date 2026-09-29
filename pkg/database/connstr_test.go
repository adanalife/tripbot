package database

import (
	"database/sql"
	"os"
	"testing"
)

// The timeouts ride the DSN as query parameters, so a typo or a driver that
// drops unknown keys would leave them silently unset. Asking the server what
// the session got is the only check that proves lib/pq passed them through.
func TestConnStrSetsStatementTimeout(t *testing.T) {
	if os.Getenv("DATABASE_HOST") == "" {
		if os.Getenv("TESTDB_REQUIRED") != "" {
			t.Fatal("DATABASE_HOST unset with TESTDB_REQUIRED set")
		}
		t.Skip("DATABASE_HOST unset; run under `task test:pkg -- ./pkg/database/`")
	}
	db, err := sql.Open("postgres", connStr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var got string
	if err := db.QueryRow("SHOW statement_timeout").Scan(&got); err != nil {
		t.Fatalf("SHOW statement_timeout: %v", err)
	}
	if got != "1min" {
		t.Errorf("statement_timeout = %q, want 1min", got)
	}
}
