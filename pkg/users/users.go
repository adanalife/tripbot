package users

import (
	"fmt"
	"log"
	"time"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/miles"
)

type User struct {
	ID          uint16    `db:"id"`
	Username    string    `db:"username"`
	Miles       float32   `db:"miles"`
	NumVisits   uint16    `db:"num_visits"`
	HasDonated  bool      `db:"has_donated"`
	IsBot       bool      `db:"is_bot"`
	FirstSeen   time.Time `db:"first_seen"`
	LastSeen    time.Time `db:"last_seen"`
	DateCreated time.Time `db:"date_created"`
}

func (u User) CurrentMiles() float32 {
	loggedInDur := time.Now().Sub(LoggedIn[u.Username])
	return u.Miles + miles.DurationToMiles(loggedInDur)
}

// User.save() will take the given user and store it in the DB
func (u User) save() {
	if config.Verbose {
		log.Println("saving user", u.Username)
	}
	query := `UPDATE users SET last_seen=:last_seen, num_visits=:num_visits, miles=:miles WHERE id = :id`
	_, err := database.DBCon.NamedExec(query, u)
	if err != nil {
		terrors.Log(err, "error saving user")
	}
}

// FindOrCreate will try to find the user in the DB, otherwise it will create a new user
func FindOrCreate(username string) User {
	if config.Verbose {
		log.Printf("FindOrCreate(%s)", username)
	}
	user := Find(username)
	if user.ID != 0 {
		return user
	}
	// create the user in the DB
	return create(username)
}

//TODO: does this need to be public?
// Find will look up the username in the DB, and return a User if possible
func Find(username string) User {
	user := User{}
	database.DBCon.Get(&user, "SELECT * FROM users WHERE username=$1", username)
	return user
}

//TODO: maybe return an err here?
// create() will actually create the DB record
func create(username string) User {
	log.Println("creating user", username)
	tx := database.DBCon.MustBegin()
	// create a new row, using default vals and creating a single visit
	tx.MustExec("INSERT INTO users (username, num_visits) VALUES ($1, $2)", username, 1)
	tx.Commit()
	return Find(username)
}

// Leaderboard returns the users with the most miles
// note that we return an array because maps are unordered
func Leaderboard(size int) [][]string {
	var leaderboard [][]string
	users := []User{}
	query := fmt.Sprintf("SELECT * FROM users WHERE miles != 0 ORDER BY miles DESC LIMIT %d", size)
	database.DBCon.Select(&users, query)
	for _, user := range users {
		miles := fmt.Sprintf("%.1f", user.CurrentMiles())
		pair := []string{user.Username, miles}
		leaderboard = append(leaderboard, pair)
	}
	return leaderboard
}
