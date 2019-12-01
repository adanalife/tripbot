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

func (osc *Onscreen) AddDur(dur time.Duration) {
	osc.Expires = osc.Expires.Add(dur)
}

// intended to be run in a goroutine
func (osc *Onscreen) Start() {
	fmt.Println("starting")
	spew.Dump(osc)
	// loop until we're past expiry time
	for time.Now().Before(osc.Expires) {
		fmt.Println("updating")
		err := osc.Update(osc)
		if err != nil {
			terrors.Log(err, "error during update")
		}
		// fmt.Println("sleeping")
		time.Sleep(osc.Interval)
	}
	fmt.Println("ending")
	spew.Dump(osc)

	// the loop is over, so hide the onscreen
	osc.Hide()
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
