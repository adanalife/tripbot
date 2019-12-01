package main

import (
	"fmt"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/database"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

// func (u User) SetLastSeen() {
// 	query := `UPDATE users SET last_seen=:last_seen WHERE id = :id`
// 	_, err := database.DBCon.NamedExec(query, u)
// 	if err != nil {
// 		terrors.Log(err, "error saving user")
// 	}
// }

// func (u User) SetFirstSeen() {
// 	query := `UPDATE users SET first_seen=:first_seen WHERE id = :id`
// 	_, err := database.DBCon.NamedExec(query, u)
// 	if err != nil {
// 		terrors.Log(err, "error saving user")
// 	}
// }

func main() {
	fmt.Println("fetching all users")
	evnts := []events.Event{}
	err := database.DBCon.Select(&evnts, "SELECT DISTINCT username from events where event='login'")
	if err != nil {
		terrors.Log(err, "problem with DB")
	}
	// spew.Dump(evnts)

	fmt.Println(len(evnts), "users found")

	var allUsernames []string
	for _, evnt := range evnts {
		allUsernames = append(allUsernames, evnt.Username)
	}
	// spew.Dump(allUsernames)

	// free up the RAM
	evnts = []events.Event{}

	for _, username := range allUsernames {
		fmt.Println("working on", username)
		user := users.FindOrCreate(username)

		if user.Miles == 0 {
			last := lastEvent(username)
			user.LastSeen = last
			user.SetLastSeen()
		}

		// first := firstEvent(username)

		// fmt.Println("first is older than first_seen")

		// if userIsNew(user) {
		// 	last := lastEvent(username)
		// 	user.LastSeen = last
		// }

		// user.FirstSeen = first

		// if last.Before(user.LastSeen) && userIsNew(user) {
		// 	fmt.Println("last is older than last_seen and user is new")
		// 	user.LastSeen = last
		// }

	}
}

func userIsNew(user users.User) bool {
	if user.LastSeen == user.DateCreated {
		fmt.Println(user, "is new")
		return true
	}
	return false
}

func firstEvent(username string) time.Time {
	first := events.Event{}
	query := fmt.Sprintf("SELECT * from events where username ='%s' ORDER BY date_created LIMIT 1", username)
	err := database.DBCon.Get(&first, query)
	if err != nil {
		terrors.Log(err, "no events found")
	}

	return first.DateCreated
}

func lastEvent(username string) time.Time {
	last := events.Event{}
	query := fmt.Sprintf("SELECT * from events where username ='%s' ORDER BY date_created DESC LIMIT 1", username)
	err := database.DBCon.Get(&last, query)
	if err != nil {
		terrors.Log(err, "no events found")
	}

	return last.DateCreated
}
