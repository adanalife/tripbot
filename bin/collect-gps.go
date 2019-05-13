package main

import (
	_ "io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/disintegration/imaging"
	_ "github.com/otiai10/gosseract"
)

const (
	screencapFile = "2018_1008_005230_049.png"
	screencapPath = "/Volumes/usbshare1/first frame of every video"
	croppedPath   = "./cropped"
)

func cropImage(srcFilename string) string {
	// Open a test image.
	src, err := imaging.Open(srcFilename)
	if err != nil {
		log.Fatalf("failed to open image: %v", err)
	}

	// crop the image to just the bottom left text
	croppedImage := imaging.CropAnchor(src, 600, 60, imaging.BottomLeft)
	// Save the resulting image
	croppedFile := filepath.Join(croppedPath, srcFilename)
	err = imaging.Save(croppedImage, croppedFile)
	return croppedFile
}

func main() {
	// imgFile := filepath.Join(framePath, screencapFile)

	// _, dashcamTimestamp := filepath.Split(imgFile)

	// files, err := ioutil.ReadDir(screencapPath)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// for i, f := range files {
	// 	log.Println(f.Name())
	// 	if i > 2 {
	// 		break
	// 	}
	// }

	err := filepath.Walk(screencapPath,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			log.Println("cropping", path)
			croppedImage := cropImage(path)
			log.Println(readText(croppedImage))
			break
			return nil
		})
	if err != nil {
		log.Println(err)
	}

	// 	client := gosseract.NewClient()
	// 	defer client.Close()
	// 	client.SetImage(croppedFile)
	// 	text, _ := client.Text()
	// 	log.Println(text)
}
