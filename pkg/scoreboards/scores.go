package users

import (
	"log"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/davecgh/go-spew/spew"
)

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
func (s Score) save() {
	if c.Conf.Verbose {
		log.Println("saving score", u)
	}
	query := `UPDATE scores SET score=:score, WHERE id = :id`
	_, err := database.Connection().NamedExec(query, u)
	if err != nil {
		terrors.Log(err, "error saving score")
	}
}

// create() will actually create the DB record
func create(user_id, scoreboard_id uint16) (error, Score) {
	log.Printf("creating score user_id:%d, scoreboard_id:%d", user_id, scoreboard_id)
	tx := database.Connection().MustBegin()
	// create a new row, using default vals and creating a single visit
	tx.MustExec("INSERT INTO scores (user_id, scoreboard_id) VALUES ($1, $2)", user_id, scoreboard_id)
	err := tx.Commit()
	spew.Dump(err)
	//TODO: fix this
	return err, &Score{}
	// return err, Find(username)
}

//// User.String prints a colored version of the user
//func (u User) String() string {
//	if u.IsBot {
//		return aurora.Gray(15, u.Username).String()
//	}
//	if c.UserIsAdmin(u.Username) {
//		return aurora.Gray(11, u.Username).String()
//	}
//	return aurora.Magenta(u.Username).String()
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

//// Find will look up the username in the DB, and return a User if possible
//func Find(username string) User {
//	var user User
//	query := `SELECT * FROM users WHERE username=$1`
//	err := database.Connection().Get(&user, query, username)
//	// spew.Config.ContinueOnMethod = true
//	// spew.Config.MaxDepth = 2
//	// spew.Dump(user)
//	if err != nil {
//		//TODO: is there a better way to do this?
//		return User{ID: 0}
//	}
//	return user
//}
