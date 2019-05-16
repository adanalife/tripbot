package main

import (
	"fmt"
	// "log"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	"github.com/dmerrick/danalol-stream/pkg/store"
)

const (
	leaderboardSize = 5
)

func main() {
	datastore := store.CreateOrFindInContext()
	spew.Dump(datastore)

	userList := datastore.TopUsers(leaderboardSize)
	spew.Dump(userList)
	// if err != nil {
	// 	log.Fatalf("unable to calculate leaderboard: %s", err)
	// }

	fmt.Println("Odometer Leaderboard")
	for _, user := range userList {
		// fmt.Println(miles.ForUser(user), "miles:", user)
		fmt.Println("miles:", user)
	}
}
