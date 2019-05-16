package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/miles"
)

const (
	leaderboardSize = 5
)

func main() {
	db := context.Background().Value(helpers.StoreKey)

	userList, err := db.TopUsers(leaderboardSize)
	if err != nil {
		log.Fatalf("unable to calculate leaderboard: %s", err)
	}

	fmt.Println("Odometer Leaderboard")
	for _, user := range userList {
		fmt.Println(miles.ForUser(user), "miles:", user)
	}
}
