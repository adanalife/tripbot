package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract"
)

const (
	screencapFile = "2018_1008_005230_049.png"
	screencapPath = "/Volumes/usbshare1/first frame of every video"
	croppedPath   = "/Volumes/usbshare1/cropped-corners"
)

func readText(croppedFile string) string {
	client := gosseract.NewClient()
	defer client.Close()
	client.SetImage(croppedFile)
	text, _ := client.Text()
	return text
}

func cropImage(srcFilename string) string {

	// we want to exit early if the cropped file already exists
	croppedFile := filepath.Join(croppedPath, path.Base(srcFilename))
	if fileExists(croppedFile) {
		return croppedFile
	}

	// open the image
	src, err := imaging.Open(srcFilename)
	if err != nil {
		log.Fatalf("failed to open image: %v", err)
	}

	// crop the image to just the bottom left text
	croppedImage := imaging.CropAnchor(src, 600, 60, imaging.BottomLeft)
	// Save the resulting image
	err = imaging.Save(croppedImage, croppedFile)
	return croppedFile
}

func fileExists(f string) bool {
	_, err := os.Stat(f)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

func main() {

	err := filepath.Walk(screencapPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if path == screencapPath {
				return nil
			}
			// log.Println("cropping", path)
			// log.Println("cropping", path)
			// log.Println("cropped file:", croppedImage)
			croppedImage := cropImage(path)
			textFromImage := readText(croppedImage)
			fmt.Println(textFromImage)
			return nil
		})
	if err != nil {
		log.Println(err)
	}

}
