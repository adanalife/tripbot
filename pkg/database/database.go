package database

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// gormConn is the process-wide DB handle, guarded by gormMu. A mutex rather
// than a sync.Once because SetGormDB resets the handle to nil in test teardown,
// which a fired Once can't express — the next GormDB call has to be able to
// build a fresh handle.
var (
	gormMu   sync.Mutex
	gormConn *gorm.DB
)

func connectToDB() *sql.DB {
	// config.SetEnvironment has already loaded the env-specific dotenv file
	// (repo-root-resolved), so the DATABASE_* values are either in the process
	// env or genuinely missing — fail loudly rather than dialing garbage.
	requiredVars := []string{
		"DATABASE_USER",
		"DATABASE_DB",
		"DATABASE_HOST",
	}
	for _, v := range requiredVars {
		_, ok := os.LookupEnv(v)
		if !ok {
			log.Fatalf("You must set %s", v)
		}
	}
	// otelsql.Open instruments the postgres driver so every query becomes a span.
	db, err := otelsql.Open("postgres", connStr(),
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		slog.Error("DB connection failed", "err", err)
		return nil
	}
	if err := db.Ping(); err != nil {
		slog.Error("DB connection failed", "err", err)
		return nil
	}
	// Keep enough idle connections to serve a console poll's parallel
	// insights queries warm. Go's default of 2 makes each poll open fresh
	// connections, and a slow connect then surfaces as a statement-timeout
	// cancel. The idle timeout hands them back once polling stops.
	db.SetMaxIdleConns(10)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if _, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	); err != nil {
		slog.Warn("could not register DB stats metrics", "err", err)
	}
	return db
}

// connection blocks until a live *sql.DB is available, retrying every 5s —
// this is what lets pods boot before postgres is ready.
func connection() *sql.DB {
	db := connectToDB()
	for db == nil {
		slog.Warn("no DB connection, waiting to reconnect")
		time.Sleep(5 * time.Second)
		db = connectToDB()
		if db != nil {
			slog.Info("DB connection made")
		}
	}
	return db
}

// GormDB returns a singleton *gorm.DB wrapping an otelsql-instrumented
// *sql.DB, with GORM-level span metadata added via otelgorm. Safe to call from
// any goroutine: concurrent first callers queue on gormMu rather than each
// building their own handle, and the lock is held across the connect so the
// boot-time retry happens once.
//
// ponytail: still returns a bare *gorm.DB, so a caller can't handle a connect
// failure — connectGorm and the required-env check below Fatal instead. Giving
// this an error return means touching every database.GormDB() call site; do
// that when a caller actually needs to recover rather than exit.
func GormDB() *gorm.DB {
	gormMu.Lock()
	defer gormMu.Unlock()
	if gormConn == nil {
		gormConn = connectGormFn()
	}
	return gormConn
}

// connectGormFn is the connect path, indirected so tests can exercise the
// lazy-init locking without a live postgres.
var connectGormFn = connectGorm

// SetGormDB swaps the singleton *gorm.DB for tests. Pair it with a sqlmock-
// backed gorm.DB to assert on the SQL emitted by package-level callers (e.g.
// users.Find, scoreboards.TopUsers) without needing a live postgres. Restore
// to nil in test teardown so other tests don't inherit the mock.
//
// Not safe for parallel tests in the same package — run with t.Setenv-style
// per-test setup and avoid t.Parallel() when using this.
func SetGormDB(db *gorm.DB) {
	gormMu.Lock()
	defer gormMu.Unlock()
	gormConn = db
}

// Ping reports whether the shared DB handle is usable, for health reporting.
// It deliberately never builds a connection: an unconnected handle is a
// "not ready" answer, and GormDB's connect path Fatals on failure, so dialing
// from a health check would end the process it was asked to describe.
func Ping(ctx context.Context) error {
	gormMu.Lock()
	db := gormConn
	gormMu.Unlock()
	if db == nil {
		return errors.New("no database connection")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close shuts down the shared DB connection pool.
func Close() error {
	gormMu.Lock()
	defer gormMu.Unlock()
	if gormConn == nil {
		return nil
	}
	sqlDB, err := gormConn.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func connectGorm() *gorm.DB {
	sqlDB := connection()
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		log.Fatal("GORM init failed:", err)
	}
	if err := gdb.Use(otelgorm.NewPlugin()); err != nil {
		slog.Warn("otelgorm plugin install failed", "err", err)
	}
	return gdb
}

// connStr returns the postgres:// url the pool dials. The credentials are
// percent-encoded, so a password holding `@`, `/`, `#` or `%` parses intact.
//
// connect_timeout (seconds) bounds a dial to a host that never answers, which
// otherwise waits out the OS's SYN retries. statement_timeout (milliseconds) is
// a runtime parameter lib/pq passes through to the server, which cancels any
// query running past it; the slowest query this pool runs takes a few seconds,
// so a minute only ever stops a runaway. Migrations run in the migrate
// initContainer on a DSN of their own and are unaffected.
func connStr() string {
	pgUser := os.Getenv("DATABASE_USER")
	pgPassword := os.Getenv("DATABASE_PASS")
	pgDatabase := os.Getenv("DATABASE_DB")
	pgHost := os.Getenv("DATABASE_HOST")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(pgUser, pgPassword),
		Host:     pgHost,
		Path:     pgDatabase,
		RawQuery: "sslmode=disable&connect_timeout=5&statement_timeout=60000",
	}
	return u.String()
}
