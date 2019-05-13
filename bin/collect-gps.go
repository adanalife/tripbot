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
	screencapDir = "/Volumes/usbshare1/first frame of every video"
	croppedPath  = "/Volumes/usbshare1/cropped-corners"
)

// readText uses OCR to read the text from an image file
func readText(imgFile string) string {
	client := gosseract.NewClient()
	defer client.Close()
	client.SetImage(imgFile)
	text, _ := client.Text()
	return text
}

// cropImage cuts a dashcam screencap down to just the bottom right corner
func cropImage(srcFilename string) string {
	// exit early if the cropped file already exists
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
	// save the resulting image to the disk
	err = imaging.Save(croppedImage, croppedFile)
	if err != nil {
		log.Fatalf("failed to save image: %v", err)
	}
	return croppedFile
}

// fileExists simply returns true if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}

func main() {

	// loop over every file in the screencapDir
	err := filepath.Walk(screencapDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			// skip the directory name itself
			if path == screencapDir {
				return nil
			}
			// crop the image
			croppedImage := cropImage(path)
			// read off the text
			textFromImage := readText(croppedImage)
			fmt.Println(textFromImage)
			return nil
		})
	if err != nil {
		log.Println(err)
	}

}
