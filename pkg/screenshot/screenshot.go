package screenshot

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/otiai10/gosseract"
)

const (
	tesseractCfg        = "~/other_projects/danalol-stream/tesseract.cfg"
	screencapDir        = "/Volumes/usbshare1/first frame of every video"
	getCurrentVidScript = "/Users/dmerrick/other_projects/danalol-stream/bin/current-file.sh"
	croppedPath         = "/Volumes/usbshare1/cropped-corners"
)

func GetCurrentVideo() string {
	// run the shell script to get currently-playing video
	out, err := exec.Command(getCurrentVidScript).Output()
	if err != nil {
		log.Fatalf("failed to run script: %v", err)
	}
	return string(out)
}

func ScreenshotPath(videoFile string) string {
	split := splitOnRegex(videoFile, "\\.")
	if len(split) < 2 {
		log.Fatalf("you must provide a valid file name")
	}
	screencapFile := fmt.Sprintf("%s.png", split[0])
	return path.Join(screencapDir, screencapFile)
}

func ProcessImage(path string) (string, error) {
	// crop the image
	croppedImage := cropImage(path)
	// read off the text
	textFromImage := readText(croppedImage)
	// pull out the coords
	coordStr := extractCoords(textFromImage)

	// don't do anything if we didn't get good coords
	if coordStr == "" {
		return coordStr, errors.New("error reading coords from file")
	}

	// fmt.Println(coordStr)
	// fmt.Println(googleMapsURL(coordStr))
	url := googleMapsURL(coordStr)
	return url, nil
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
	croppedImage = imaging.Grayscale(croppedImage)
	croppedImage = imaging.AdjustContrast(croppedImage, 20)
	croppedImage = imaging.Sharpen(croppedImage, 2)
	croppedImage = imaging.Invert(croppedImage)
	// save the resulting image to the disk
	err = imaging.Save(croppedImage, croppedFile)
	if err != nil {
		log.Fatalf("failed to save image: %v", err)
	}
	return croppedFile
}

// readText uses OCR to read the text from an image file
func readText(imgFile string) string {
	client := gosseract.NewClient()
	defer client.Close()

	// set up tesseract to improve OCR accuracy
	client.SetConfigFile(tesseractCfg)
	client.SetWhitelist("NSEW.1234567890MPH")
	client.SetBlacklist("abcdefghijklmnopqrstuvwxyz/\\")
	// https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#page-segmentation-method
	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// read the file
	client.SetImage(imgFile)
	text, err := client.Text()
	if err != nil {
		log.Fatalf("failed to read text: %v", err)
	}
	return text
}

// extractCoords expects an OCR-ed string which
// may or may not contain GPS coordinates, and
// returns its best guess at what the coords are
func extractCoords(text string) string {
	// strip all whitespace
	tidy := strings.Replace(text, " ", "", -1)
	// try to separate the text using the speed
	split := splitOnRegex(tidy, "MPH")
	// if we didn't find the speed, just exit
	if len(split) < 2 {
		return ""
	}
	// use only the second half (the GPS coordinates)
	coords := split[1]
	return coords
}

// googleMapsURL returns a google maps link to the coords provided
func googleMapsURL(coordsStr string) string {
	return fmt.Sprintf("https://www.google.com/maps?q=%s", coordsStr)
}

// splitOnRegex will is the equivalent of str.split(/regex/)
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

// fileExists simply returns true if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return err == nil
}
