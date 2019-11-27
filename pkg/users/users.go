package users

import (
	"fmt"
	"log"
	"strings"
	"time"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/twitch"
	"github.com/logrusorgru/aurora"

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
	LoggedIn    time.Time
}

func (u User) CurrentMiles() float32 {
	if isLoggedIn(u.Username) {
		loggedInDur := time.Now().Sub(u.LoggedIn)
		return u.Miles + miles.DurationToMiles(loggedInDur)
	}
	return u.Miles
}

// User.save() will take the given user and store it in the DB
func (u User) save() {
	if config.Verbose {
		log.Println("saving user", u)
	}
	query := `UPDATE users SET last_seen=:last_seen, num_visits=:num_visits, miles=:miles WHERE id = :id`
	_, err := database.DBCon.NamedExec(query, u)
	if err != nil {
		terrors.Log(err, "error saving user")
	}
}

// IsFollower returns true if the user is a follower
func (u User) IsFollower() bool {
	return twitch.UserIsFollower(u.Username)
}

// IsSubscriber returns true if the user is a subscriber
func (u User) IsSubscriber() bool {
	return twitch.UserIsSubscriber(u.Username)
}

// User.String prints a colored version of the user
func (u User) String() string {
	if u.IsBot {
		return aurora.Gray(15, u.Username).String()
	}
	if u.Username == strings.ToLower(config.ChannelName) {
		return aurora.Gray(11, u.Username).String()
	}
	return aurora.Magenta(u.Username).String()
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

// Find will look up the username in the DB, and return a User if possible
func Find(username string) User {
	var user User
	query := fmt.Sprintf("SELECT * FROM users WHERE username='%s'", username)
	err := database.DBCon.Get(&user, query)
	// spew.Config.ContinueOnMethod = true
	// spew.Config.MaxDepth = 2
	// spew.Dump(user)
	if err != nil {
		//TODO: is there a better way to do this?
		return User{ID: 0}
	}
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
