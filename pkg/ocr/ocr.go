package ocr

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
	split := splitOnRegex(videoFile, "\\.")
	if len(split) < 2 {
		log.Printf("you must provide a valid file name")
	}
	screencapFile := fmt.Sprintf("%s.png", split[0])
	return path.Join(config.ScreencapDir, screencapFile)
}

func CoordsFromImage(path string) (string, error) {
	// crop the image
	croppedImage := CropImage(path)
	// read off the text
	textFromImage := ReadText(croppedImage)
	// pull out the coords
	coordStr := ExtractCoords(textFromImage)

	// don't do anything if we didn't get good coords
	if coordStr == "" {
		return coordStr, errors.New("error reading coords from file")
	}

	// fmt.Println(coordStr)
	// fmt.Println(googleMapsURL(coordStr))
	// url := googleMapsURL(coordStr)
	return coordStr, nil
}

// cropImage cuts a dashcam screencap down to just the bottom right corner
func CropImage(srcFilename string) string {
	// exit early if the cropped file already exists
	croppedFile := filepath.Join(config.CroppedPath, path.Base(srcFilename))
	if fileExists(croppedFile) {
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
func ReadText(imgFile string) string {
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
func ExtractCoords(text string) string {
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
	result[len(indexes)] = text[laststart:]
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
