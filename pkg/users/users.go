package users

import (
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
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

func FindOrCreate(username string) User {
	//TODO: remove this
	log.Printf("FindOrCreate(%s)", username)
	var emptyUser User
	user := Find(username)
	if user != emptyUser {
		return user
	}
	// create the user in the DB
	return create(username)
}

func Find(username string) User {
	var emptyUser User
	//TODO: does this have to be a slice?
	users := []User{}
	database.DBCon.Select(&users, "SELECT * from users where username='$1'", username)
	spew.Dump(users)
	if len(users) == 0 {
		log.Println("could not find user", username)
		return emptyUser
	}
	return users[0]
}

//TODO: maybe return an err here?
func create(username string) User {
	log.Println("creating user", username)
	tx := database.DBCon.MustBegin()
	// create a new row, using default vals and creating a single visit
	tx.MustExec("INSERT INTO users (username, num_visits) VALUES ($1, 1)", username)
	tx.Commit()
	return Find(username)
}
