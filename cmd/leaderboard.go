package main

import (
	"flag"
	"fmt"

	"github.com/dmerrick/danalol-stream/pkg/store"
)

var leaderboardSize int

func init() {
	flag.IntVar(&leaderboardSize, "num", 5, "The size of the leaderboard")
	flag.Parse()
}

func main() {
	datastore := store.FindOrCreate("tripbot-copy.db")

	userList := datastore.TopUsers(leaderboardSize)

	fmt.Println("Odometer Leaderboard")
	for _, user := range userList {
		fmt.Println(datastore.MilesForUser(user), "miles:", user)
	}
}
