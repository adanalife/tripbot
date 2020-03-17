//TODO: this would be better as just 'db'
package database

import (
	"fmt"
	"log"
	"os"

	"github.com/dmerrick/danalol-stream/pkg/config"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	//TODO: this would be better as just 'con'
	// this is how we will share the DB connection
	DBCon *sqlx.DB
)

//TODO: is it a bad idea to actually connect to the DB here
// AKA "automatically"?
func init() {
	var err error

	err = godotenv.Load(".env." + config.Environment)
	if err != nil {
		log.Println("Error loading .env file:", err)
		log.Println("Continuing anyway...")
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
