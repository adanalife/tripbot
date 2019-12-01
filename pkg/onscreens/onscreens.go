package onscreens

import (
	"fmt"
	"io/ioutil"
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

// intended to be run in a goroutine
func (d *Onscreen) Start() {
	fmt.Println("starting")
	spew.Dump(d)
	// loop until we're past expiry time
	for time.Now().Before(d.Expires) {
		fmt.Println("updating")
		err := d.Update(d)
		if err != nil {
			terrors.Log(err, "error during update")
		}
		fmt.Println("sleeping")
		time.Sleep(d.Interval)
	}
	fmt.Println("ending")
	spew.Dump(d)
}

func (d Onscreen) Show() {
	if d.image {
		showImage(d.Content)
	} else {
		d.showText()
	}
}
func (d Onscreen) Hide() {
	if d.image {
		hideImage(d.Content)
	} else {
		hideText(d.Content)
	}
}

func (d Onscreen) showText() {
	if d.OutputFile == "" {
		terrors.Log(nil, "no OutputFile set")
		return
	}
	fmt.Println("writing to file:", d.OutputFile)
	b := []byte(d.Content)
	err := ioutil.WriteFile(d.OutputFile, b, 0644)
	if err != nil {
		terrors.Log(err, "error writing to file")
	}
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
