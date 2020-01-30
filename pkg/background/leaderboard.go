package background

import (
	"fmt"
	"path"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

var leaderboardDuration = time.Duration(20 * time.Second)

var Leaderboard *onscreens.Onscreen

func ShowOnscreenLeaderboard() {
	Leaderboard = onscreens.New()
	Leaderboard.OutputFile = path.Join(helpers.ProjectRoot(), "OBS/leaderboard.txt")
	spew.Dump(Leaderboard.OutputFile)
	spew.Dump(leaderboardContent())
	Leaderboard.Show(leaderboardContent(), leaderboardDuration)
}

// leaderboardContent creates the content for the leaderboard
func leaderboardContent() string {
	var output string
	output = "Odometer Leaderboard\n"

	size := 5
	leaderboard := users.Leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}

	return output
}
