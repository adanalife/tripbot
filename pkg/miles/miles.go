package miles

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/events"
)

// func TopUsers(size int) map[string]float32 {
func TopUsers(size int) [][]string {
	//TODO maybe don't use an Events map?
	evnts := []events.Event{}
	oneMonthAgo := time.Now().Add(time.Duration(-30*24) * time.Hour)
	err := database.DBCon.Select(&evnts, "SELECT DISTINCT username from events where event='login' and date_created >= $1", oneMonthAgo)
	if err != nil {
		log.Println("problem with db:", err)
	}
	leaderboard := make(map[string]float32)
	for _, event := range evnts {
		user := event.Username
		leaderboard[user] = ForUser(user)
	}
	allScoresSorted := sortByValue(leaderboard)
	return allScoresSorted[:size]
}

// DurationToMiles converts Durations to miles
func DurationToMiles(dur time.Duration) float32 {
	// 0.1mi every 3 minutes
	return float32(0.1 * dur.Minutes() / 3.0)
}

// ForUser returns the miles for a given user
func ForUser(user string) float32 {
	evnts := []events.Event{}
	query := fmt.Sprintf("SELECT username, event, date_created from events where username = '%s' AND event in ('login', 'logout')", user)
	err := database.DBCon.Select(&evnts, query)
	if err != nil {
		log.Println("error fetching events from db", err)
	}
	pairs := splitIntoPairs(evnts)
	dur := combinePairs(pairs)
	return DurationToMiles(dur)
}

// splitIntoPairs takes a list of events and smartly pairs together matching login/logout events
func splitIntoPairs(evnts []events.Event) [][]events.Event {
	var pairs [][]events.Event
	// no events were found
	if len(evnts) == 0 {
		return pairs
	}

	// check if their most recent event is a login
	if evnts[len(evnts)-1].Event == "login" {
		// log.Println("user is logged in, adding a logout event")
		// ... in which case add the current time to the list
		evnts = append(evnts, events.Event{DateCreated: time.Now(), Event: "logout"})
	}

	// now we're going to loop over all of the events and split them into pairs
	for i := 0; i < len(evnts)-1; i++ {
		// we're only looking for logins here
		if evnts[i].Event == "logout" {
			continue
		}

		// this will be our login/logout pair
		pair := make([]events.Event, 2)

		// check if the _next_ event is a login
		if evnts[i+1].Event == "login" {
			// next event is login, so we'll use that instead
			continue
		}

		// okay so we know the next event isn't a login, we know the next is a logout
		if evnts[i].Event == "login" {
			// set the pair
			pair[0] = evnts[i]
			pair[1] = evnts[i+1]
		}

		// add this login/logout pair to the list
		pairs = append(pairs, pair)
	}
	return pairs
}

// combinePairs takes pairs of login/logout events and sums up the time between them
func combinePairs(pairs [][]events.Event) time.Duration {
	var durSum time.Duration
	for _, pair := range pairs {
		login, logout := pair[0].DateCreated, pair[1].DateCreated
		durSum = durSum + logout.Sub(login)
	}
	return durSum
}

// this is ugly but I took it right from StackOverflow
// https://stackoverflow.com/a/18695428
func sortByValue(kv map[string]float32) [][]string {
	sorted := [][]string{}
	n := map[float64][]string{}
	var a []float64
	var shouldBeIgnored bool
	for k, v := range kv {
		n[float64(v)] = append(n[float64(v)], k)
	}
	for k := range n {
		a = append(a, k)
	}
	sort.Float64s(a)
	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}

	for _, k := range a {
		for _, user := range n[k] {
			shouldBeIgnored = false
			for _, ignored := range config.IgnoredUsers {
				if user == ignored {
					shouldBeIgnored = true
				}
			}
			if shouldBeIgnored {
				continue
			}

			sorted = append(sorted, []string{user, fmt.Sprintf("%.1f", k)})
		}
	}
	return sorted
}
