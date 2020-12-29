package scoreboards

import (
	"log"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/davecgh/go-spew/spew"
)

// CREATE TABLE scoreboards (
//   id           SERIAL PRIMARY KEY,
//   name         VARCHAR(64) UNIQUE NOT NULL,
//   date_created TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
// );

type Scoreboard struct {
	ID          uint16    `db:"id"`
	Name        string    `db:"name"`
	DateCreated time.Time `db:"date_created"`
}

// Create() will actually create the DB record
func Create(name string) (Scoreboard, error) {
	log.Println("creating scoreboard", name)
	tx := database.Connection().MustBegin()
	// create a new row, using default vals and creating a single visit
	result, err := tx.Exec("INSERT INTO scoreboards (name) VALUES ($1)", name)
	if err != nil {
		return Scoreboard{ID: 0}, err
	}
	spew.Dump("res", result)
	err = tx.Commit()
	spew.Dump(err)
	return FindScoreboard(name), err
}

//// FindScoreboard will look up the username in the DB, and return a Scoreboard if possible
func FindScoreboard(name string) Scoreboard {
	var scoreboard Scoreboard
	query := `SELECT * FROM scoreboards WHERE name=$1`
	err := database.Connection().Get(&scoreboard, query, name)

	// spew.Config.ContinueOnMethod = true
	// spew.Config.MaxDepth = 2
	// spew.Dump(scoreboard)

	if err != nil {
		//TODO: is there a better way to do this?
		return Scoreboard{ID: 0}
	}
	return scoreboard
}

//// create() will actually create the DB record
//func create(username string) User {
//	log.Println("creating user", username)
//	tx := database.Connection().MustBegin()
//	// create a new row, using default vals and creating a single visit
//	tx.MustExec("INSERT INTO users (username, num_visits) VALUES ($1, $2)", username, 1)
//	tx.Commit()
//	return Find(username)
//}

//// FindOrCreate will try to find the user in the DB, otherwise it will create a new user
//func FindOrCreate(username string) User {
//	if c.Conf.Verbose {
//		log.Printf("FindOrCreate(%s)", username)
//	}
//	user := Find(username)
//	if user.ID != 0 {
//		return user
//	}
//	// create the user in the DB
//	return create(username)
//}

//// FindOrCreate will try to find the user in the DB, otherwise it will create a new user
//func FindOrCreate(username string) User {
//	if c.Conf.Verbose {
//		log.Printf("FindOrCreate(%s)", username)
//	}
//	user := Find(username)
//	if user.ID != 0 {
//		return user
//	}
//	// create the user in the DB
//	return create(username)
//}
