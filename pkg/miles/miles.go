package miles

import (
	"fmt"
	"log"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

func ForUser(user string) int {
	evnts := []events.Event{}
	query := fmt.Sprintf("SELECT username, event, date_created from events where username = '%s' AND event in ('login', 'logout')", user)
	err := database.DBCon.Select(&evnts, query)
	if err != nil {
		log.Println("error fetching events from db", err)
	}
	pairs := splitIntoPairs(evnts)
	dur := combinePairs(pairs)
	return helpers.DurationToMiles(dur)
}

func splitIntoPairs(evnts []events.Event) [][]events.Event {
	var pairs [][]events.Event
	// no events were found
	if len(evnts) == 0 {
		return pairs
	}

	// check if their most recent event is a login
	if evnts[len(evnts)-1].Event == "login" {
		log.Println("user is logged in, adding a logout event")
		// ... in which case add the current time to the list
		evnts = append(evnts, events.Event{DateCreated: time.Now(), Event: "logout"})
	}

	for i := 0; i < len(evnts)-1; i++ {
		// we're only looking for logins here
		if evnts[i].Event == "logout" {
			continue
		}

		// this will contain a login/logout pair
		pair := make([]events.Event, 2)

		// check if the _next_ event is a login
		if evnts[i+1].Event == "login" {
			// next event is login, so we'll use that instead
			continue
		}

		// okay so we know the next event isn't a login
		if evnts[i].Event == "login" {
			// set the pair
			pair[0] = evnts[i]
			pair[1] = evnts[i+1]
		}

		if len(pair) != 2 {
			log.Fatal("pair wasn't full for some reason")
		}

		pairs = append(pairs, pair)
	}
	return pairs
}

func combinePairs(pairs [][]events.Event) time.Duration {
	var durSum time.Duration
	for _, pair := range pairs {
		login, logout := pair[0].DateCreated, pair[1].DateCreated
		durSum = durSum + logout.Sub(login)
	}
	return durSum
}
