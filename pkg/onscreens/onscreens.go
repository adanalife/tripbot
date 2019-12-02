package onscreens

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
)

var defaultDuration = time.Duration(30 * time.Second)

type UpdateFunc func(*Onscreen) error

type Onscreen struct {
	Content    string
	Expires    time.Time
	Interval   time.Duration
	Update     UpdateFunc
	image      bool
	OutputFile string
}

func New() *Onscreen {
	newOnscreen := &Onscreen{}
	newOnscreen.Content = ""
	newOnscreen.Expires = time.Now().Add(time.Duration(defaultDuration))
	newOnscreen.Interval = time.Duration(10 * time.Second)
	return newOnscreen
}

func (osc *Onscreen) expired() bool {
	return time.Now().After(osc.Expires)
}

func (osc *Onscreen) Extend(dur time.Duration) {
	// if it's expired, expire dur from now
	if osc.expired() {
		osc.Expires = time.Now().Add(dur)
		return
	}
	// otherwise, add dur to the current expiry date
	osc.Expires = osc.Expires.Add(dur)
}

// intended to be run in a goroutine
func (osc *Onscreen) Start() {
	fmt.Println("starting")
	spew.Dump(osc)

	for true {
		fmt.Println("updating")
		err := osc.Update(osc)
		if err != nil {
			terrors.Log(err, "error during update")
		}

		if osc.expired() {
			osc.Hide()
		} else {
			osc.Show()
		}

		// fmt.Println("sleeping")
		time.Sleep(osc.Interval)
	}
	fmt.Println("ending")
}

func (osc *Onscreen) Show() {
	if osc.image {
		showImage(osc.Content)
	} else {
		osc.showText()
	}
}
func (osc *Onscreen) Hide() {
	if osc.image {
		hideImage(osc.Content)
	} else {
		osc.hideText()
	}
}

func (osc Onscreen) showText() {
	if osc.OutputFile == "" {
		terrors.Log(nil, "no OutputFile set")
		return
	}
	fmt.Println("writing to file:", osc.OutputFile)
	b := []byte(osc.Content)
	err := ioutil.WriteFile(osc.OutputFile, b, 0644)
	if err != nil {
		terrors.Log(err, "error writing to file")
	}
}

func (osc Onscreen) hideText() {
	fmt.Println("removing file:", osc.OutputFile)
	os.Remove(osc.OutputFile)
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
