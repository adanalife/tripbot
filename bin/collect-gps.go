package main

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract"
)

const (
	tesseractCfg = "/Users/dmerrick/other_projects/danalol-stream/tesseract.cfg"
	screencapDir = "/Volumes/usbshare1/first frame of every video"
	croppedPath  = "/Volumes/usbshare1/cropped-corners"
)

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
			coordStr := extractCoords(textFromImage)
			// don't print anything if we didn't get good coords
			if coordStr == "" {
				return nil
			}
			fmt.Println(coordStr)

			fmt.Println(googleMapsURL(coordStr))
			return nil
		})
	if err != nil {
		log.Println(err)
	}

}

func extractCoords(text string) string {
	// strip all whitespace
	tidy := strings.Replace(text, " ", "", -1)
	split := splitOnRegex(tidy, "MPH")
	// exit here if we didn't read the coords correctly
	if len(split) < 2 {
		return ""
	}
	coords := split[1]
	return coords
}

// readText uses OCR to read the text from an image file
func readText(imgFile string) string {
	client := gosseract.NewClient()
	client.SetConfigFile(tesseractCfg)
	client.SetWhitelist("NSEW.1234567890MPH")
	client.SetBlacklist("abcdefghijklmnopqrstuvwxyz")
	// https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#page-segmentation-method
	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)
	defer client.Close()
	client.SetImage(imgFile)
	text, err := client.Text()
	if err != nil {
		log.Fatalf("failed to read text: %v", err)
	}
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

func splitOnRegex(text string, delimeter string) []string {
	reg := regexp.MustCompile(delimeter)
	indexes := reg.FindAllStringIndex(text, -1)
	laststart := 0
	result := make([]string, len(indexes)+1)
	for i, element := range indexes {
		result[i] = text[laststart:element[0]]
		laststart = element[1]
	}
	result[len(indexes)] = text[laststart:len(text)]
	return result
}

func googleMapsURL(coordsStr string) string {
	return fmt.Sprintf("https://www.google.com/maps?q=%s", coordsStr)
}
