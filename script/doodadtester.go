package main

import (
	"fmt"

	"github.com/dmerrick/danalol-stream/pkg/doodads"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

func main() {
	LeaderboardDoodad := doodads.Doodad{}

	content := generateContent()
	LeaderboardDoodad.Content = content

	LeaderboardDoodad.Show()

}

func generateContent() string {
	var output string
	output = "Odometer Leaderboard\n"

	users.InitLeaderboard()
	users.UpdateLeaderboard()

	size := 5
	leaderboard := users.Leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}
	return output
}
