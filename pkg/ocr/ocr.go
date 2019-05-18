package ocr

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/otiai10/gosseract"
)

func GetCurrentVideo() string {
	// run the shell script to get currently-playing video
	out, err := exec.Command(config.GetCurrentVidScript).Output()
	if err != nil {
		log.Printf("failed to run script: %v", err)
	}
	return string(out)
}

func ScreenshotPath(videoFile string) string {
	split := helpers.SplitOnRegex(videoFile, "\\.")
	if len(split) < 2 {
		log.Printf("you must provide a valid file name")
	}
	screencapFile := fmt.Sprintf("%s.png", split[0])
	return path.Join(config.ScreencapDir, screencapFile)
}

func CoordsFromImage(path string) (string, error) {
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

	return coordStr, nil
}

// cropImage cuts a dashcam screencap down to just the bottom right corner
func cropImage(srcFilename string) string {
	// exit early if the cropped file already exists
	croppedFile := filepath.Join(config.CroppedPath, path.Base(srcFilename))
	if helpers.FileExists(croppedFile) {
		return croppedFile
	}

	// open the image
	src, err := imaging.Open(srcFilename)
	if err != nil {
		log.Printf("failed to open image: %v", err)
	}

	// crop the image to just the bottom left text
	croppedImage := imaging.CropAnchor(src, 600, 60, imaging.BottomLeft)

	// apply some tweaks to make it easier to read
	croppedImage = imaging.Grayscale(croppedImage)
	croppedImage = imaging.AdjustContrast(croppedImage, 20)
	croppedImage = imaging.Sharpen(croppedImage, 2)
	croppedImage = imaging.Invert(croppedImage)

	// save the resulting image to the disk
	err = imaging.Save(croppedImage, croppedFile)
	if err != nil {
		log.Printf("failed to save image: %v", err)
	}
	return croppedFile
}

// readText uses OCR to read the text from an image file
func readText(imgFile string) string {
	client := gosseract.NewClient()
	defer client.Close()

	// set up tesseract to improve OCR accuracy
	client.SetConfigFile(path.Join(helpers.ProjectRoot(), "configs/tesseract.cfg"))
	// https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#page-segmentation-method
	//TODO: use single line
	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// read the file
	client.SetImage(imgFile)
	text, err := client.Text()
	if err != nil {
		log.Printf("failed to read text: %v", err)
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
	split := helpers.SplitOnRegex(tidy, "MPH")
	// if we didn't find the speed, just exit
	if len(split) < 2 {
		return ""
	}
	// use only the second half (the GPS coordinates)
	coords := split[1]
	return coords
}
