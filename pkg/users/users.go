package users

import (
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
)

type User struct {
	ID          uint16    `db:"id"`
	Username    string    `db:"username"`
	Miles       float32   `db:"miles"`
	NumVisits   uint16    `db:"num_visits"`
	HasDonated  bool      `db:"has_donated"`
	FirstSeen   time.Time `db:"first_seen"`
	LastSeen    time.Time `db:"last_seen"`
	DateCreated time.Time `db:"date_created"`
}

// User.save() will take the given user and store it in the DB
func (u User) save() {
	if config.Verbose {
		log.Println("saving user", u.Username)
		spew.Dump(u)
	}
	query := `UPDATE users SET last_seen=:last_seen, num_visits=:num_visits WHERE id = :id`
	_, err := database.DBCon.NamedExec(query, u)
	if err != nil {
		log.Println("error saving user:", err)
	}
}

//TODO: consider rolling this into logout()
func (u User) LogoutIfNecessary() {
	// check if the user is currently logged in
	if u.isLoggedIn() {
		u.logout()
		return
	}
	log.Println("hmm, LogoutIfNecessary() called and user not logged in:", u.Username)
}

// logout() removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func (u User) logout() {
	// remove the user from the logged in list
	// https://stackoverflow.com/a/34070691
	for i, v := range LoggedIn {
		if v == u {
			LoggedIn = append(LoggedIn[:i], LoggedIn[i+1:]...)
			break
		}
	}
	// update the last seen date
	u.LastSeen = time.Now()
	// store the user in the db
	u.save()
}

// isLoggedIn checks if the user is currently logged in
func (u User) isLoggedIn() bool {
	for _, loggedInUser := range LoggedIn {
		if u == loggedInUser {
			return true
		}
	}
	return false
}

// LoginIfNecessary checks the list of currently-logged in users and will
// run login() if this user isn't currently logged in
func LoginIfNecessary(username string) {
	// check if the user is currently logged in
	if isLoggedIn(username) {
		return
	}
	// they weren't logged in, so note in the DB
	login(username)
}

// login will record the users presence in the DB
func login(username string) {
	user := FindOrCreate(username)

	LoggedIn = append(LoggedIn, user)
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// update the last seen date
	user.LastSeen = time.Now()
	user.save()
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
	err := database.DBCon.Get(&user, "SELECT * FROM users WHERE username=$1", username)
	// fmt.Printf("%#v\n", user)
	if err != nil {
		log.Println("error finding user", username, err)
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
