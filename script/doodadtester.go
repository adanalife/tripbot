package main

import (
	"fmt"
	"path"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/onscreens"
	"github.com/dmerrick/danalol-stream/pkg/users"
)

func main() {
	users.InitLeaderboard()

	Leaderboard := onscreens.New()
	Leaderboard.Update = update
	Leaderboard.Expires = time.Now().Add(time.Duration(20 * time.Second))
	Leaderboard.OutputFile = path.Join(helpers.ProjectRoot(), "OBS/leaderboard.txt")

	spew.Dump(Leaderboard)

	go Leaderboard.Start()

	// just loop for for a while so the program doesnt terminate
	for Leaderboard != nil {
		time.Sleep(10 * time.Second)
	}
}

func update(osc *onscreens.Onscreen) error {
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
