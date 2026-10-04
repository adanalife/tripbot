package database

import (
	"database/sql"
	"net/url"
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

// A password is free text, and every URL delimiter in it would otherwise
// split the DSN in the wrong place. Runs without a database.
func TestConnStrEscapesCredentials(t *testing.T) {
	t.Setenv("DATABASE_USER", "trip bot")
	t.Setenv("DATABASE_PASS", "p@ss/w:rd#?%")
	t.Setenv("DATABASE_HOST", "pg.example.svc.cluster.local.")
	t.Setenv("DATABASE_DB", "tripbot")

	u, err := url.Parse(connStr())
	if err != nil {
		t.Fatalf("parse %q: %v", connStr(), err)
	}
	if got := u.User.Username(); got != "trip bot" {
		t.Errorf("user = %q", got)
	}
	if got, _ := u.User.Password(); got != "p@ss/w:rd#?%" {
		t.Errorf("password = %q", got)
	}
	if u.Host != "pg.example.svc.cluster.local." || u.Path != "/tripbot" {
		t.Errorf("host/path = %q %q", u.Host, u.Path)
	}
	q := u.Query()
	if q.Get("connect_timeout") != "5" || q.Get("statement_timeout") != "60000" || q.Get("sslmode") != "disable" {
		t.Errorf("query = %v", q)
	}
}
