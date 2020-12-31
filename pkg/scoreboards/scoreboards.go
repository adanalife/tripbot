package scoreboards

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/jmoiron/sqlx"
)

// Scoreboard represents a bucket of scores, and has a name to identify it
type Scoreboard struct {
	ID          uint16    `db:"id"`
	Name        string    `db:"name"`
	DateCreated time.Time `db:"date_created"`
}

func TopUsers(scoreboardName string, size int) [][]string {
	var leaderboard [][]string

	ignoredUsers := append(c.IgnoredUsers, strings.ToLower(c.Conf.ChannelName))

	// we use MySQL-style ? bindvars instead of postgres ones here
	// because that's what sqlx wants for In()
	q := `SELECT users.username, scores.value FROM scoreboards, scores, users WHERE scoreboards.name = ? AND scores.user_id = users.id AND scores.scoreboard_id = scoreboards.id AND users.username NOT IN (?) ORDER BY scores.value DESC LIMIT ?;`
	query, args, err := sqlx.In(q, scoreboardName, ignoredUsers, size)
	if err != nil {
		terrors.Log(err, "error generating query")
	}

	// Rebind will convert the query to postgres syntax
	query = database.Connection().Rebind(query)
	rows, err := database.Connection().Query(query, args...)
	if err != nil {
		terrors.Log(err, "error running query")
	}

	for rows.Next() {
		var username string
		var value float32
		err = rows.Scan(&username, &value)
		if err != nil {
			terrors.Log(err, "error scanning row")
			continue
		}
		valueAsString := fmt.Sprintf("%.1f", value)
		pair := []string{username, valueAsString}
		leaderboard = append(leaderboard, pair)
	}

	return leaderboard
}

// findOrCreateScoreboard will find a Scoreboard in the DB or create one
func findOrCreateScoreboard(name string) (Scoreboard, error) {
	scoreboard, err := findScoreboard(name)
	if err != nil {
		if err == sql.ErrNoRows {
			// create a new scoreboard if none found
			scoreboard, err = createScoreboard(name)
		} else {
			// it was some other error
			terrors.Log(err, "error getting scoreboard from db")
		}
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
		terrors.Log(err, "error committing new scoreboard in DB")
		return scoreboard, err
	}
	return findScoreboard(name)
}
