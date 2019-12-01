package background

import (
	"fmt"
	"path"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

var defaultDuration = time.Duration(20 * time.Second)

var Leaderboard onscreens.Onscreen

func ShowOnscreenLeaderboard() {
	Leaderboard = onscreens.New()
	Leaderboard.Update = updateLeaderboard
	Leaderboard.Interval = defaultDuration // don't update it
	Leaderboard.Expires = time.Now().Add(defaultDuration)
	Leaderboard.OutputFile = path.Join(helpers.ProjectRoot(), "OBS/leaderboard.txt")
	go Leaderboard.Start()
}

// updateLeaderboard fills in the content for the leaderboard
func updateLeaderboard(osc *onscreens.Onscreen) error {
	var output string
	output = "Odometer Leaderboard\n"

	size := 5
	leaderboard := users.Leaderboard[:size]

	for _, score := range leaderboard {
		output = output + fmt.Sprintf("%s miles: %s\n", score[1], score[0])
	}

	osc.Content = output

	osc.Show()
	return nil
}
