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
	db := "tripbot-copy.db"
	datastore := store.FindOrCreate(db)

	userList := datastore.TopUsers(leaderboardSize)
	// if err != nil {
	// 	log.Fatalf("unable to calculate leaderboard: %s", err)
	// }

	fmt.Println("Odometer Leaderboard")
	for _, user := range userList {
		fmt.Println(datastore.MilesForUser(user), "miles:", user)
	}
}
