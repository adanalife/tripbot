package scoreboards

import (
	"database/sql"
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/davecgh/go-spew/spew"
)

//TODO: change underscores to camelCase
//TODO: change Score.score to value

type Score struct {
	ID           uint16    `db:"id"`
	UserID       uint16    `db:"user_id"`
	ScoreboardID uint16    `db:"scoreboard_id"`
	Value        float32   `db:"value"`
	DateCreated  time.Time `db:"date_created"`
}

func GetScoreByName(username, scoreboardName string) (float32, error) {
	var score Score
	userID, err := getUserIDByName(username)
	if err != nil {
		terrors.Log(err, "error getting user ID")
		return -1.0, err
	}
	scoreboard, err := findOrCreateScoreboard(scoreboardName)
	if err != nil {
		//TODO
	}
	score, err = findOrCreateScore(userID, scoreboard.ID)

	if err != nil {
		spew.Dump(err)
		terrors.Log(err, "error finding score")
		return -1.0, err
	}
	return score.Value, err
}

func AddToScoreByName(username, scoreboardName string, scoreToAdd float32) error {
	var score Score
	userID, err := getUserIDByName(username)
	if err != nil {
		//TODO
		return err
	}
	scoreboard, err := findOrCreateScoreboard(scoreboardName)
	if err != nil {
		//TODO
	}
	score, err = findOrCreateScore(userID, scoreboard.ID)
	spew.Dump(score)
	if err != nil {
		terrors.Log(err, "error finding score")
		return err
	}
	score.Value += scoreToAdd
	return score.save()
}

// findOrCreateScore will look up the username in the DB, and return a Score if possible
func findOrCreateScore(user_id, scoreboard_id uint16) (Score, error) {
	score, err := findScore(user_id, scoreboard_id)
	if err != nil {
		if err == sql.ErrNoRows {
			score, err = createScore(user_id, scoreboard_id)
		} else {
			// it was some other error
			terrors.Log(err, "error getting score from db")
		}
	}
	return score, err
}

// findScore will look up the username in the DB, and return a Score if possible
func findScore(user_id, scoreboard_id uint16) (Score, error) {
	var score Score
	query := `SELECT * FROM scores WHERE user_id=$1 AND scoreboard_id=$2`
	err := database.Connection().Get(&score, query, user_id, scoreboard_id)
	return score, err
}

// createScore() will actually create the DB record
func createScore(user_id, scoreboard_id uint16) (Score, error) {
	var score Score
	log.Printf("creating score user_id:%d, scoreboard_id:%d", user_id, scoreboard_id)
	tx := database.Connection().MustBegin()
	// create a new row using default value
	_, err := tx.Exec("INSERT INTO scores (user_id, scoreboard_id) VALUES ($1, $2)", user_id, scoreboard_id)
	if err != nil {
		terrors.Log(err, "error inserting score in DB")
		return score, err
	}
	err = tx.Commit()
	if err != nil {
		terrors.Log(err, "error commiting score change in DB")
		return score, err
	}
	return findScore(user_id, scoreboard_id)
}

// User.save() will take the given score and store it in the DB
func (s Score) save() error {
	if c.Conf.Verbose {
		log.Println("saving score", s)
	}
	query := `UPDATE scores SET value=:value WHERE id = :id`
	_, err := database.Connection().NamedExec(query, s)
	if err != nil {
		terrors.Log(err, "error saving score")
	}
	return err
}

func getUserIDByName(username string) (uint16, error) {
	var userID uint16
	query := `SELECT id FROM users WHERE username=$1`
	row := database.Connection().QueryRow(query, username)
	// spew.Dump(row)
	err := row.Scan(&userID)
	if err != nil {
		terrors.Log(err, "error scanning row")
	}
	return userID, err
}
