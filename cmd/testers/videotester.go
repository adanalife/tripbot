package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

func main() {
	toTry := []string{
		"foobar",
		"2018_0514_184752_015.MP4",
		"2018_0514_184752_015_a.MP4",
		"/Volumes/usbshare1/Dashcam/_all/2018_0514_184752_015.MP4",
		"/Volumes/usbshare1/Dashcam/_all/2018_0514_184752_015_opt.MP4",
	}

	for _, str := range toTry {
		fmt.Println()
		fmt.Println()
		Display(str)
	}
}

func Display(str string) {
	vid, err := video.LoadOrCreate(str)

	spew.Dump(str)
	if err != nil {
		spew.Dump(err)
	} else {
		spew.Dump(vid)
		spew.Dump(vid.File())
		spew.Dump(vid.Path())
	}
}
