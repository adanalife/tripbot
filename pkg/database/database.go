//TODO: this would be better as just 'db'
package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dmerrick/tripbot/pkg/config"
	terrors "github.com/dmerrick/tripbot/pkg/errors"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

var (
	//TODO: this would be better as just 'con'
	// this is how we will share the DB connection
	dbConnection *sqlx.DB
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

	// force a connection and test that it worked
	// if !isAlive() {
	// 	terrors.Fatal(fmt.Errorf("database error"), "error creating first connection to the DB")
	// }
}

func connectToDB() *sqlx.DB {
	dbConnection, err := sqlx.Connect("postgres", connStr())
	if err != nil {
		terrors.Log(err, "error initializing the DB")
		// return &sqlx.DB{}
		return nil
	}
	return dbConnection
}

func Connection() *sqlx.DB {
	// during startup - if it does not exist, create it
	if dbConnection == nil {
		dbConnection = connectToDB()
	}
	connected := isAlive()
	for connected != true { // reconnect if we lost connection
		log.Print("Connection to DB was lost. Waiting...")
		time.Sleep(5 * time.Second)
		log.Print("Reconnecting...")
		dbConnection = connectToDB()
		connected = isAlive()
	}
	return dbConnection
}

func isAlive() bool {
	if dbConnection == nil {
		return false
	}
	err := dbConnection.Ping()
	if err != nil {
		terrors.Log(err, "error connecting to DB")
		return false
	}
	return true
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
