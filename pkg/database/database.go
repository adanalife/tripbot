package database

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/XSAM/otelsql"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/uptrace/opentelemetry-go-extra/otelgorm"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// this is how we will share the DB connection
var dbConnection *sqlx.DB
var gormConn *gorm.DB

func init() {
	var err error

	err = godotenv.Load(".env." + c.Conf.Environment)
	// In cluster contexts (staging/production) the .env file is not shipped —
	// env values come from envconfig instead — so the missing-file error is
	// expected and noise. Only surface it for local-dev workflows.
	if err != nil && (c.Conf.Environment == "development" || c.Conf.Environment == "testing") {
		slog.Warn("error loading .env file, continuing anyway", "err", err)
	}

	// first we have to check we have all of the right ENV vars
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
}

func connectToDB() *sqlx.DB {
	// otelsql.Open instruments the postgres driver so every query becomes
	// a span; the returned *sql.DB is wrapped into sqlx as usual.
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
	return sqlx.NewDb(db, "postgres")
}

func Connection() *sqlx.DB {
	// if it does not exist, create it
	if dbConnection == nil {
		dbConnection = connectToDB()
	}
	connected := isAlive()
	for connected != true { // reconnect if we lost connection
		slog.Warn("connection to DB was lost, waiting to reconnect")
		time.Sleep(5 * time.Second)
		dbConnection = connectToDB()
		connected = isAlive()
		if connected {
			slog.Info("DB connection made")
		}
	}
	return dbConnection
}

func isAlive() bool {
	if dbConnection == nil {
		return false
	}
	err := dbConnection.Ping()
	if err != nil {
		slog.Error("error connecting to DB", "err", err)
		return false
	}
	return true
}

// GormDB returns a singleton *gorm.DB that wraps the same otelsql-instrumented
// *sql.DB used by Connection(), adding GORM-level span metadata via otelgorm.
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

func connectGorm() *gorm.DB {
	// Reuse the otelsql-instrumented *sql.DB so both layers share one connection pool.
	sqlDB := Connection().DB
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
