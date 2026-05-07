package onscreensServer

import (
	"log"
	"time"
)

var GPSImage *Onscreen

var gpsDuration = time.Duration(150 * time.Second)

func InitGPSImage() {
	log.Println("Creating GPS image onscreen")
	GPSImage = New()
}

//TODO: this should probably return an error
func ShowGPSImage() {
	GPSImage.Show("")
}

//TODO: this should probably return an error
func HideGPSImage() {
	GPSImage.Hide()
}
