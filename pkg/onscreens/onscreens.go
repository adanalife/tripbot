package onscreens

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	terrors "github.com/dmerrick/tripbot/pkg/errors"
	"github.com/dmerrick/tripbot/pkg/helpers"
)

//TODO: these live in the background package and could/should
// be moved into this package
//TODO: we don't always need SleepInterval/Expires... some
// of these run forever (maybe refactor into ShowFor()?)

// imageSuffix is added to the end of image files to make the "live"
const imageSuffix = "-live"

// defaultSleepInterval is how often onscreens refresh themselves
const defaultSleepInterval = time.Duration(5 * time.Second)

type Onscreen struct {
	Content       string
	Expires       time.Time
	DontExpire    bool
	SleepInterval time.Duration
	IsShowing     bool
	isImage       bool
	outputFile    string
}

func New(outputFile string) *Onscreen {
	newOnscreen := &Onscreen{}
	newOnscreen.Content = ""
	newOnscreen.Expires = time.Now()
	newOnscreen.DontExpire = false
	newOnscreen.SleepInterval = time.Duration(defaultSleepInterval)
	newOnscreen.outputFile = outputFile
	// start the background loop
	go newOnscreen.backgroundLoop()
	return newOnscreen
}

func NewImage(imageFile string) *Onscreen {
	osc := New(imageFile)
	osc.isImage = true
	return osc
}

// backgroundLoop will loop forever, hiding the Onscreen if needed
//TODO: do we need a way to close out this loop?
func (osc *Onscreen) backgroundLoop() {
	for { // forever
		if osc.IsShowing && osc.isExpired() {
			osc.Hide()
		}
		time.Sleep(osc.SleepInterval)
	}
}

func (osc *Onscreen) isExpired() bool {
	// return false if set to not expire
	if osc.DontExpire {
		return false
	}
	// return true if current date is after exp date
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

// Show makes an onscreen visible until hidden
func (osc *Onscreen) Show(content string) {
	// set it to never expire
	osc.DontExpire = true
	// make visible with the content
	osc.show(content)
}

// ShowFor makes an Onscreen visible for a duration of time
func (osc *Onscreen) ShowFor(content string, dur time.Duration) {
	// if it was set to not expire, running this
	// means we changed our mind
	osc.DontExpire = false
	// add the duration to the expiry time
	osc.Extend(dur)
	// make visible with the content
	osc.show(content)
}

// Hide will remove an onscreen from the screen
func (osc *Onscreen) Hide() {
	osc.IsShowing = false
	if osc.isImage {
		osc.hideImage()
	} else {
		osc.hideText()
	}
}

// show is what makes an Onscreen visible
func (osc *Onscreen) show(content string) {
	// mark it as visible
	osc.IsShowing = true
	// set the content
	osc.Content = content
	if osc.isImage {
		osc.showImage()
	} else {
		osc.showText()
	}
}

// showText will write the Content to the outputFile
func (osc Onscreen) showText() {
	b := []byte(osc.Content)
	err := ioutil.WriteFile(osc.outputFile, b, 0644)
	if err != nil {
		terrors.Log(err, "error writing to file")
	}
}

// showImage will create a new "live" image file
func (osc Onscreen) showImage() {
	// copy the image to the live location
	err := os.Link(osc.outputFile, osc.liveImage())
	if err != nil {
		terrors.Log(err, "error creating image")
	}
}

// hideText will truncate the outputFile (hiding the text)
func (osc Onscreen) hideText() {
	b := []byte("") // empty file
	err := ioutil.WriteFile(osc.outputFile, b, 0644)
	if err != nil {
		terrors.Log(err, "error emptying to file")
	}
}

// hideImage will remove the "live" version of an image file
func (osc Onscreen) hideImage() {
	if helpers.FileExists(osc.liveImage()) {
		err := os.Remove(osc.liveImage())
		if err != nil {
			terrors.Log(err, "error removing image")
		}
	}
}

// liveImage adds a suffix to the end of the file
// which is the file that OBS will be configured to look at
func (osc Onscreen) liveImage() string {
	return fmt.Sprintf("%s%s", osc.outputFile, imageSuffix)
}
