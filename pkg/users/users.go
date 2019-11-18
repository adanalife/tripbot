package users

import (
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
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

func (u User) save() {
	log.Println("this is where we'll save the user")
	spew.Dump(u)
	query := `UPDATE users SET last_seen=:last_seen, num_visits=:num_visits WHERE id = :id`
	_, err := database.DBCon.NamedExec(query, u)
	if err != nil {
		spew.Dump(err)
	}
}

func Login(username string) {
	user := FindOrCreate(username)
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// update the last seen date
	user.LastSeen = time.Now()
	user.save()
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
	user := User{}
	err := database.DBCon.Get(&user, "SELECT * FROM users WHERE username=$1", username)
	// fmt.Printf("%#v\n", user)
	if err != nil {
		log.Println("error finding user", username, err)
	}
	return user
}

//TODO: maybe return an err here?
func create(username string) User {
	log.Println("creating user", username)
	tx := database.DBCon.MustBegin()
	// create a new row, using default vals and creating a single visit
	tx.MustExec("INSERT INTO users (username, num_visits) VALUES ($1, $2)", username, 1)
	tx.Commit()
	return Find(username)
}
