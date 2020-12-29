package scoreboards

import (
	"database/sql"
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
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

// create() will actually create the DB record
func create(name string) (Scoreboard, error) {
	var scoreboard Scoreboard
	if c.Conf.Verbose {
		log.Println("creating scoreboard", name)
	}
	tx := database.Connection().MustBegin()
	// create a new scoreboard
	_, err := tx.Exec("INSERT INTO scoreboards (name) VALUES ($1)", name)
	if err != nil {
		//TODO
		return scoreboard, err
	}
	err = tx.Commit()
	if err != nil {
		//TODO
		return scoreboard, err
	}
	return FindScoreboard(name)
}

//// findScoreboard will look up the username in the DB, and return a Scoreboard if possible
func findScoreboard(name string) (Scoreboard, error) {
	var scoreboard Scoreboard
	query := `SELECT * FROM scoreboards WHERE name=$1`
	err := database.Connection().Get(&scoreboard, query, name)

	return scoreboard, err
}

func FindOrCreateScoreboard(name string) (Scoreboard, error) {
	scoreboard, err := findScoreboard(name)
	if err == sql.ErrNoRows {
		scoreboard, err = create(name)
	} else {
		// it was some other error
		terrors.Log(err, "error getting scoreboard from db")
	}
	return scoreboard, err
}
