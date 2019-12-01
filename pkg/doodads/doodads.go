package doodads

import (
	"fmt"
	"time"
)

type Doodad struct {
	Content         string
	expires         time.Time
	image           bool
	defaultDuration time.Duration
}

// var LeaderboardDoodad Doodad

func (d Doodad) Show() {
	if d.image {
		showImage(d.Content)
	} else {
		showText(d.Content)
	}
}
func (d Doodad) Hide() {
	if d.image {
		hideImage(d.Content)
	} else {
		hideText(d.Content)
	}
}

func init() {
	// gpsImage := Doodad{
	// 	Content: path.Join(helpers.ProjectRoot(), "OBS/GPS.png")
	// 	image:   true,
	// }

	// LeaderboardDoodad := Doodad{
	// 	Content: path.Join(helpers.ProjectRoot(), "OBS/GPS.png")
	// 	image:   true,
	// }
}

func showText(text string) {
	fmt.Println(text)
}
func hideText(text string) {
	fmt.Println("hiding", text)
}

func showImage(imgPath string) {
	// src := path.Join(helpers.ProjectRoot(), "OBS/GPS.png")
	// dest := path.Join(helpers.ProjectRoot(), "OBS/GPS-live.png")
	// os.Link(src, dest)
}

func hideImage(imgPath string) {
	// noGPSDest := path.Join(helpers.ProjectRoot(), "OBS/GPS-live.png")
	// os.Remove(noGPSDest)
}
