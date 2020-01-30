package onscreens

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

var defaultSleepInterval = time.Duration(5 * time.Second)

type Onscreen struct {
	Content       string
	Expires       time.Time
	SleepInterval time.Duration
	isImage       bool
	outputFile    string
}

func New(outputFile string) *Onscreen {
	newOnscreen := &Onscreen{}
	newOnscreen.Content = ""
	newOnscreen.Expires = time.Now()
	newOnscreen.SleepInterval = time.Duration(defaultSleepInterval)
	newOnscreen.outputFile = outputFile
	// start the background loop
	go newOnscreen.backgroundLoop()
	return newOnscreen
}

// backgroundLoop will loop forever, hiding the Onscren if needed
//TODO: do we need a way to close out this loop?
func (osc *Onscreen) backgroundLoop() {
	for { // forever
		if osc.isExpired() {
			fmt.Println("onscreen", osc.outputFile, "is expired")
			osc.Hide()
		} else {
			fmt.Println("not expired yet")
		}
		time.Sleep(osc.SleepInterval)
	}
}

func (osc *Onscreen) isExpired() bool {
	return time.Now().After(osc.Expires)
}

func (osc *Onscreen) Extend(dur time.Duration) {
	// if it's expired, expire dur from now
	if osc.isExpired() {
		osc.Expires = time.Now().Add(dur)
		return
	}
	// otherwise, add dur to the current expiry date
	osc.Expires = osc.Expires.Add(dur)
}

func (osc *Onscreen) Show(content string, dur time.Duration) {
	// set the content
	osc.Content = content
	// add the duration to the expiry time
	osc.Extend(dur)
	if osc.isImage {
		showImage(osc.Content)
	} else {
		osc.showText()
	}
}
func (osc *Onscreen) Hide() {
	if osc.isImage {
		hideImage(osc.Content)
	} else {
		osc.hideText()
	}
}

// showText will write the Content to the outputFile
func (osc Onscreen) showText() {
	fmt.Println("writing to file:", osc.outputFile)
	b := []byte(osc.Content)
	err := ioutil.WriteFile(osc.outputFile, b, 0644)
	if err != nil {
		terrors.Log(err, "error writing to file")
	}
}

// hideText will delete the outputFile (hiding the text)
func (osc Onscreen) hideText() {
	if helpers.FileExists(osc.outputFile) {
		err := os.Remove(osc.outputFile)
		if err != nil {
			terrors.Log(err, "error removing file")
		}
	}
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
