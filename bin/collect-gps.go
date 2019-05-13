package main

import (
	"io/ioutil"
	"log"
	_ "path/filepath"

	_ "github.com/disintegration/imaging"
	_ "github.com/otiai10/gosseract"
)

const (
	screencapFile = "2018_1008_005230_049.png"
	screencapPath = "/Volumes/usbshare1/first frame of every video"
	croppedPath   = "./cropped"
)

func main() {
	// imgFile := filepath.Join(framePath, screencapFile)
	// croppedFile := filepath.Join(croppedPath, filepath.Base(screencapFile), ".png")

	// _, dashcamTimestamp := filepath.Split(imgFile)

	files, err := ioutil.ReadDir(screencapPath)
	if err != nil {
		log.Fatal(err)
	}

	for i, f := range files {
		log.Println(f.Name())
		if i > 2 {
			break
		}
	}

	// 	// Open a test image.
	// 	src, err := imaging.Open(imgFile)
	// 	if err != nil {
	// 		log.Fatalf("failed to open image: %v", err)
	// 	}

	// 	// Crop the original image to 300x300px size using the center anchor.
	// 	src = imaging.CropAnchor(src, 600, 60, imaging.BottomLeft)
	// 	// Save the resulting image as JPEG.
	// 	err = imaging.Save(src, croppedFile)

	// 	client := gosseract.NewClient()
	// 	defer client.Close()
	// 	client.SetImage(croppedFile)
	// 	text, _ := client.Text()
	// 	log.Println(text)
}
