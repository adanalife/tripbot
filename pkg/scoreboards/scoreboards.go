package scoreboards

import (
	"database/sql"
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

type Scoreboard struct {
	ID          uint16    `db:"id"`
	Name        string    `db:"name"`
	DateCreated time.Time `db:"date_created"`
}

func findOrCreateScoreboard(name string) (Scoreboard, error) {
	scoreboard, err := findScoreboard(name)
	if err == sql.ErrNoRows {
		// create a new scoreboard if none found
		scoreboard, err = createScoreboard(name)
	} else {
		// it was some other error
		terrors.Log(err, "error getting scoreboard from db")
	}
	return scoreboard, err
}

// findScoreboard will look up the scoreboard in the DB, and return a Scoreboard if possible
func findScoreboard(name string) (Scoreboard, error) {
	var scoreboard Scoreboard
	query := `SELECT * FROM scoreboards WHERE name=$1`
	err := database.Connection().Get(&scoreboard, query, name)
	return scoreboard, err
}

// createScoreboard() will actually create the DB record
func createScoreboard(name string) (Scoreboard, error) {
	var scoreboard Scoreboard
	if c.Conf.Verbose {
		log.Println("creating scoreboard", name)
	}
	tx := database.Connection().MustBegin()
	// create a new scoreboard
	_, err := tx.Exec("INSERT INTO scoreboards (name) VALUES ($1)", name)
	if err != nil {
		terrors.Log(err, "error inserting scoreboard into db")
		return scoreboard, err
	}
	err = tx.Commit()
	if err != nil {
		terrors.Log(err, "error commiting scoreboard change in DB")
		return scoreboard, err
	}
	return findScoreboard(name)
}
