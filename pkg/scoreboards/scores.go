package scoreboards

import (
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/davecgh/go-spew/spew"
)

//TODO: change underscores to camelCase
//TODO: change Score.score to value

// CREATE TABLE scores (
//   id            SERIAL PRIMARY KEY,
//   user_id       INTEGER NOT NULL,
//   scoreboard_id INTEGER NOT NULL,
//   score         REAL DEFAULT 0.0, /* float32: https://github.com/go-pg/pg/wiki/Model-Definition */
//   date_created  TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
// );

type Score struct {
	ID           uint16    `db:"id"`
	UserID       uint16    `db:"user_id"`
	ScoreboardID uint16    `db:"scoreboard_id"`
	Score        float32   `db:"score"`
	DateCreated  time.Time `db:"date_created"`
}

// User.save() will take the given score and store it in the DB
func (s Score) save() error {
	if c.Conf.Verbose {
		log.Println("saving score", s)
	}
	query := `UPDATE scores SET score=:score, WHERE id = :id`
	_, err := database.Connection().NamedExec(query, s)
	if err != nil {
		terrors.Log(err, "error saving score")
	}
	return err
}

// create() will actually create the DB record
func create(user_id, scoreboard_id uint16) (Score, error) {
	log.Printf("creating score user_id:%d, scoreboard_id:%d", user_id, scoreboard_id)
	tx := database.Connection().MustBegin()
	// create a new row, using default vals and creating a single visit
	tx.MustExec("INSERT INTO scores (user_id, scoreboard_id) VALUES ($1, $2)", user_id, scoreboard_id)
	err := tx.Commit()
	spew.Dump(err)
	return FindScore(user_id, scoreboard_id), err
}

func FindScoreByName(username, scoreboardName string) float32 {
	query := `SELECT id FROM users WHERE username=$1`
	result, err := database.Connection().Exec(query, username)
	spew.Dump(result)
	if err != nil {
		//TODO
	}
	userID := result
	scoreboard := FindScoreboard(scoreboardName)
	score := findScore(userID, scoreboard.ID)
	return score.score
}

func AddToScoreByName(username, scoreboardName string, scoreToAdd uint16) error {
	scoreboard := FindScoreboard(scoreboardName)
	score := findScore(user.ID, scoreboard.ID)
	score.score += scoreToAdd
	return score.save()
}

//TODO: return an error
// findScore will look up the username in the DB, and return a Score if possible
func findScore(user_id, scoreboard_id uint16) Score {
	var score Score
	query := `SELECT * FROM scores WHERE user_id=$1 AND scoreboard_id=$2`
	err := database.Connection().Get(&score, query, user_id, scoreboard_id)
	// spew.Config.ContinueOnMethod = true
	// spew.Config.MaxDepth = 2
	// spew.Dump(score)
	if err != nil {
		return Score{ID: 0}
	}
	return score
}

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
