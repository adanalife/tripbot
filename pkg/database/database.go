package database

import (
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// this is how we will share the DB connection
var gormConn *gorm.DB

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
// *sql.DB, with GORM-level span metadata added via otelgorm.
func GormDB() *gorm.DB {
	if gormConn == nil {
		gormConn = connectGorm()
	}
	return gormConn
}

// SetGormDB swaps the singleton *gorm.DB for tests. Pair it with a sqlmock-
// backed gorm.DB to assert on the SQL emitted by package-level callers (e.g.
// users.Find, scoreboards.TopUsers) without needing a live postgres. Restore
// to nil in test teardown so other tests don't inherit the mock.
//
// Not safe for parallel tests in the same package — run with t.Setenv-style
// per-test setup and avoid t.Parallel() when using this.
func SetGormDB(db *gorm.DB) {
	gormConn = db
}

// Close shuts down the shared DB connection pool.
func Close() error {
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

// returns a valid postgres:// url
func connStr() string {
	pgUser := os.Getenv("DATABASE_USER")
	pgPassword := os.Getenv("DATABASE_PASS")
	pgDatabase := os.Getenv("DATABASE_DB")
	pgHost := os.Getenv("DATABASE_HOST")

	connStr := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable", pgUser, pgPassword, pgHost, pgDatabase)
	return connStr
}
