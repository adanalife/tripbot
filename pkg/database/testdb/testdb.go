// Package testdb hands tests a real postgres connection wired into the
// database singleton, so repo-level tests exercise the actual schema and
// dialect instead of sqlmock string-matching. New gives per-test isolation
// via a transaction that rolls back in cleanup; Shared installs the
// pool-backed connection for tests that need session-scoped features
// (advisory locks) a transaction can't reach.
//
// The connection targets the compose stack's db service (see
// infra/docker/docker-compose.testing.yml) using the same DATABASE_* env
// vars as production code. When postgres is unreachable the calling test
// skips — unless TESTDB_REQUIRED is set (the compose test service sets it),
// where a skip would silently mask a wiring bug.
package testdb

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	once    sync.Once
	pool    *gorm.DB
	dialErr error
)

// New installs a transaction-scoped *gorm.DB as the database singleton and
// returns it. The transaction rolls back in test cleanup, so writes never
// leak between tests. Like SetGormDB itself, not safe for t.Parallel().
func New(t *testing.T) *gorm.DB {
	t.Helper()
	tx := connect(t).Begin()
	if tx.Error != nil {
		t.Fatalf("testdb: begin: %v", tx.Error)
	}
	database.SetGormDB(tx)
	t.Cleanup(func() {
		database.SetGormDB(nil)
		tx.Rollback()
	})
	return tx
}

// Shared installs the pool-backed *gorm.DB as the database singleton and
// returns it. No transaction, no rollback — for tests exercising
// session-scoped behavior like pg advisory locks, which need real pooled
// connections (gorm's DB() errors on a transaction). Callers must clean up
// any rows they write.
func Shared(t *testing.T) *gorm.DB {
	t.Helper()
	gdb := connect(t)
	database.SetGormDB(gdb)
	t.Cleanup(func() { database.SetGormDB(nil) })
	return gdb
}

func connect(t *testing.T) *gorm.DB {
	t.Helper()
	once.Do(func() {
		// connect_timeout keeps the no-docker case (task test:macos) at a
		// fast skip instead of a hanging dial.
		dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable&connect_timeout=3",
			os.Getenv("DATABASE_USER"), os.Getenv("DATABASE_PASS"),
			os.Getenv("DATABASE_HOST"), os.Getenv("DATABASE_DB"))
		// database/sql, not otelsql — tests don't need spans. The postgres
		// driver is registered by pkg/database's lib/pq import.
		sqlDB, err := sql.Open("postgres", dsn)
		if err == nil {
			err = sqlDB.Ping()
		}
		if err != nil {
			dialErr = err
			return
		}
		pool, dialErr = gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	})
	if dialErr != nil {
		if os.Getenv("TESTDB_REQUIRED") != "" {
			t.Fatalf("testdb: postgres unreachable with TESTDB_REQUIRED set: %v", dialErr)
		}
		t.Skipf("testdb: postgres unreachable, skipping: %v", dialErr)
	}
	return pool
}
