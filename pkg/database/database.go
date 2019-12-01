//TODO: this would be better as just 'db'
package database

import (
	"errors"
	"fmt"
	"os"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/jmoiron/sqlx"
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

func init() {
	var err error
	// initialize the SQL database
	DBCon, err = initialize()
	if err != nil {
		terrors.Fatal(err, "error initializing the DB")
	}
}

func initialize() (*sqlx.DB, error) {
	// first we have to check we have all of the right ENV vars
	for _, env := range requiredENV {
		if os.Getenv(env) == "" {
			return DBCon, errors.New("missing required ENV var")
		}
	}

	DBCon, err := sqlx.Connect("postgres", connStr())
	if err != nil {
		return DBCon, err
	}
	// force a connection and test that it worked
	err = DBCon.Ping()
	return DBCon, err
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
