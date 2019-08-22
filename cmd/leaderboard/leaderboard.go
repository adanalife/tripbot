package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/dmerrick/danalol-stream/pkg/database"
	"github.com/dmerrick/danalol-stream/pkg/miles"
	"github.com/joho/godotenv"
)

var leaderboardSize int

func init() {
	flag.IntVar(&leaderboardSize, "n", 5, "The size of the leaderboard")
	flag.Parse()
}

func main() {
	var err error

	godotenv.Load()
	database.DBCon, err = database.Initialize()
	if err != nil {
		log.Fatal("error initializing db:", err)
	}

	leaderboard := miles.TopUsers(leaderboardSize)

	fmt.Println("Odometer Leaderboard")
	for _, score := range leaderboard {
		fmt.Printf("%s miles: %s\n", score[1], score[0])
	}
}
