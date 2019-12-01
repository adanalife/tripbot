package main

import (
	"flag"
	"fmt"

	"github.com/dmerrick/danalol-stream/pkg/users"
)

var leaderboardSize int

func init() {
	flag.IntVar(&leaderboardSize, "n", 5, "The size of the leaderboard")
	flag.Parse()
}

func main() {
	users.InitLeaderboard()
	leaderboard := users.Leaderboard[:leaderboardSize]

	fmt.Println("Odometer Leaderboard")
	for _, score := range leaderboard {
		fmt.Printf("%s miles: %s\n", score[1], score[0])
	}
}
