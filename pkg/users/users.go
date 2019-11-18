package users

import (
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/database"
)

type User struct {
	Username string `db:"username"`
	// Event       string    `db:"event"`
	Miles       float32   `db:"miles"`
	NumVisits   uint16    `db:"num_visits"`
	HasDonated  bool      `db:"has_donated"`
	FirstSeen   time.Time `db:"first_seen"`
	LastSeen    time.Time `db:"last_seen"`
	DateCreated time.Time `db:"date_created"`
}

//TODO: maybe return an err here?
func create(username string) {
	log.Println("creating user", username)
	tx := database.DBCon.MustBegin()
	tx.MustExec("INSERT INTO users (username) VALUES ($1)", username)
	tx.Commit()
}

func FindOrCreate(username string) User {
	var emptyUser User
	user := Find(username)
	if user != emptyUser {
		return user
	}
	// create the user in the DB
	create(username)
	// we call Find() a second time here, which is a lil inefficient
	return Find(username)
}

func Find(username string) User {
	var emptyUser User
	//TODO: does this have to be a slice?
	users := []User{}
	database.DBCon.Select(&users, "SELECT username from users where username='$1'", username)
	if len(users) == 0 {
		log.Println("could not find user", username)
		return emptyUser
	}
	return users[0]
}
