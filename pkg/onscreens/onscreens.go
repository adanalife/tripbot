package onscreens

import (
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
)

var defaultDuration = time.Duration(30 * time.Second)

type UpdateFunc func(*Onscreen) error

type Onscreen struct {
	Content  string
	Expires  time.Time
	Interval time.Duration
	Update   UpdateFunc
	image    bool
}

func New() *Onscreen {
	newOnscreen := &Onscreen{}
	newOnscreen.Content = ""
	newOnscreen.Expires = time.Now().Add(time.Duration(defaultDuration))
	newOnscreen.Interval = time.Duration(10 * time.Second)
	return newOnscreen
}

// intended to be run in a goroutine
func (d Onscreen) Start() {
	fmt.Println("starting")
	spew.Dump(d)
	// loop until we're past expiry time
	for time.Now().Before(d.Expires) {
		fmt.Println("updating")
		d.Update(&d)
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
		showText(d.Content)
	}
}
func (d Onscreen) Hide() {
	if d.image {
		hideImage(d.Content)
	} else {
		hideText(d.Content)
	}
}

func showText(text string) {
	fmt.Println(text)
	spew.Dump(text)
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
