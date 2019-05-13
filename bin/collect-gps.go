package main

import (
	"log"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract"
)

const (
	dashcamTimestamp = "2018_1008_005230_049"
	framePath        = "/Volumes/usbshare1/first frame of every video"
	croppedPath      = "./cropped"
)

func main() {
	// imgFile := fmt.Sprintf("%s/%s.png", framePath, dashcamTimestamp)
	// croppedFile := fmt.Sprintf("cropped-%s.png", dashcamTimestamp)
	imgFile := filepath.Join(framePath, dashcamTimestamp, ".png")
	croppedFile := filepath.Join(croppedPath, dashcamTimestamp, ".png")

	// Open a test image.
	src, err := imaging.Open(imgFile)
	if err != nil {
		log.Fatalf("failed to open image: %v", err)
	}

	// Crop the original image to 300x300px size using the center anchor.
	src = imaging.CropAnchor(src, 600, 60, imaging.BottomLeft)
	// Save the resulting image as JPEG.
	err = imaging.Save(src, croppedFile)

	client := gosseract.NewClient()
	defer client.Close()
	client.SetImage(croppedFile)
	text, _ := client.Text()
	log.Println(text)
}
