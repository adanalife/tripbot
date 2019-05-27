package main

import (
	"flag"

	"github.com/dmerrick/danalol-stream/pkg/miles"
)

var leaderboardSize int

func init() {
	flag.IntVar(&leaderboardSize, "n", 5, "The size of the leaderboard")
	flag.Parse()
}

func main() {
	// datastore := store.FindOrCreate("db/tripbot-copy.db")

	miles.TopUsers(leaderboardSize)

	// fmt.Println("Odometer Leaderboard")
	// for _, user := range userList {
	// 	fmt.Println(datastore.MilesForUser(user), "miles:", user)
	// }
}
