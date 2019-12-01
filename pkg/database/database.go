//TODO: this would be better as just 'db'
package database

import (
	"fmt"
	"os"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	//TODO: this would be better as just 'con'
	// this is how we will share the DB connection
	DBCon *sqlx.DB

	//TODO should this be a const?
	requiredENV = []string{
		"DATABASE_USER",
		"DATABASE_DB",
		"DATABASE_PASS",
		"DATABASE_HOST",
	}
)

//TODO: is it a bad idea to actually connect to the DB here
// AKA "automatically"?
func init() {
	var err error

	err = godotenv.Load()
	if err != nil {
		terrors.Fatal(err, "Error loading .env file")
	}

	// first we have to check we have all of the right ENV vars
	for _, env := range requiredENV {
		if os.Getenv(env) == "" {
			terrors.Fatal(nil, "missing required ENV var "+env)
		}
	}

	DBCon, err = sqlx.Connect("postgres", connStr())
	if err != nil {
		terrors.Fatal(err, "error initializing the DB")
	}
	// force a connection and test that it worked
	err = DBCon.Ping()
	if err != nil {
		terrors.Fatal(err, "error connecting to DB")
	}
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
