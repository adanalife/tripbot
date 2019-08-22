package main

import (
	// "fmt"
	// "log"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

const (
	leaderboardSize = 5
)

func main() {

	vidStr := "2018_0920_172817_007.MP4"
	vid, _ := video.LoadOrCreate(vidStr)
	dateObj := vid.Date

	spew.Dump(dateObj)
}
