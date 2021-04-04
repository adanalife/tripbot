package users

import (
	"log"
	"time"

	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/twitch"
	"github.com/logrusorgru/aurora"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
)

type User struct {
	ID           uint16    `db:"id"`
	Username     string    `db:"username"`
	Miles        float32   `db:"miles"`
	NumVisits    uint16    `db:"num_visits"`
	HasDonated   bool      `db:"has_donated"`
	IsBot        bool      `db:"is_bot"`
	FirstSeen    time.Time `db:"first_seen"`
	LastSeen     time.Time `db:"last_seen"`
	DateCreated  time.Time `db:"date_created"`
	LoggedIn     time.Time
	lastCmd      time.Time
	lastLocation time.Time
}

// this is how long they have before they can guess again
var guessCooldown = 3 * time.Minute

func (u User) loggedInDur() time.Duration {
	// exit early if they're not logged in
	if !isLoggedIn(u.Username) {
		return 0 * time.Second
	}
	// lookup the user in the session so the LoggedIn value is current
	return time.Now().Sub(LoggedIn[u.Username].LoggedIn)
}

func (u User) sessionMiles() float32 {
	// exit early if they're not logged in
	if !isLoggedIn(u.Username) {
		return 0.0
	}
	loggedInDur := u.loggedInDur()
	sessionMiles := helpers.DurationToMiles(loggedInDur)
	// give subscribers a miles bonus
	if u.IsSubscriber() {
		bonusMiles := u.BonusMiles()
		if c.Conf.Verbose {
			log.Println(u.String(), "will get", aurora.Green(bonusMiles), "bonus miles")
		}
		sessionMiles += bonusMiles
	}
	return sessionMiles
}

func (u User) CurrentMiles() float32 {
	return u.Miles + u.sessionMiles()
}

func (u User) BonusMiles() float32 {
	if isLoggedIn(u.Username) {
		loggedInDur := u.loggedInDur()
		sessionMiles := helpers.DurationToMiles(loggedInDur)
		return sessionMiles * 0.05
	}
	return 0.0
}

func (u User) CurrentMonthlyMiles() float32 {
	return u.GetScore(scoreboards.CurrentMilesScoreboard()) + u.sessionMiles()
}

func (u User) CurrentScore() float32 {
	//TODO: update to Score (not miles)
	return u.Miles + u.sessionMiles()
}

func (u User) CurrentMonthlyScore() float32 {
	//TODO: update to Score (not miles)
	return u.GetScore(scoreboards.CurrentMilesScoreboard()) + u.sessionMiles()
}

// User.save() will take the given user and store it in the DB
func (u User) save() {
	if c.Conf.Verbose {
		log.Println("saving user", u)
	}
	query := `UPDATE users SET last_seen=:last_seen, num_visits=:num_visits, miles=:miles WHERE id = :id`
	_, err := database.Connection().NamedExec(query, u)
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
	if c.UserIsAdmin(u.Username) {
		return aurora.Gray(11, u.Username).String()
	}
	return aurora.Magenta(u.Username).String()
}

// FindOrCreate will try to find the user in the DB, otherwise it will create a new user
func FindOrCreate(username string) User {
	if c.Conf.Verbose {
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
	query := `SELECT * FROM users WHERE username=$1`
	err := database.Connection().Get(&user, query, username)
	if err != nil {
		//TODO: is there a better way to do this?
		return User{ID: 0}
	}
	return user
}

// HasCommandAvailable lets users run a command once a day,
// unless they are a follower in which case they can run
// as many as they like
func (u *User) HasCommandAvailable() bool {
	// followers get unlimited commands
	if u.IsFollower() {
		return true
	}
	// check if they ran a command in the last 24 hrs
	now := time.Now()
	if now.Sub(u.lastCmd) > 24*time.Hour {
		log.Println("letting", u, "run a command")
		// update their lastCmd time
		u.lastCmd = now
		return true
	}
	return false
}

// GuessCooldownRemaining returns the amount of time a user needs to
// wait before they can guess again
func (u User) GuessCooldownRemaining() time.Duration {
	now := time.Now()
	cooldownExpiry := u.lastLocation.Add(guessCooldown)

	if u.lastLocation.Add(guessCooldown).After(now) {
		return cooldownExpiry.Sub(now)
	}
	return 0 * time.Minute
}

// HasGuessCommandAvailable returns true if the user is allowed to use the guess command
func (u *User) HasGuessCommandAvailable(lastTimewarpTime time.Time) bool {
	// let the user run if there has been a timewarp recently
	if u.lastLocation.Before(lastTimewarpTime) {
		return true
	}

	// check if they ran a location command recently
	if u.GuessCooldownRemaining() <= 0 {
		log.Println("letting", u, "run guess command")
		return true
	}
	return false
}

func (u *User) SetLastLocationTime() {
	u.lastLocation = time.Now()
}

//TODO: maybe return an err here?
// create() will actually create the DB record
func create(username string) User {
	log.Println("creating user", username)
	tx := database.Connection().MustBegin()
	// create a new row, using default vals and creating a single visit
	//TODO: do something with results, error returned here
	tx.Exec("INSERT INTO users (username, num_visits) VALUES ($1, $2)", username, 1)
	tx.Commit()
	return Find(username)
}
