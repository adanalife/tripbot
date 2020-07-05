package ocr

import (
	"errors"
	"log"
	"path"
	"path/filepath"
	"strings"

	terrors "github.com/adanalife/tripbot/pkg/errors"

	"github.com/disintegration/imaging"
	"github.com/adanalife/tripbot/pkg/config"
	"github.com/adanalife/tripbot/pkg/helpers"
	"github.com/otiai10/gosseract"
)

func CoordsFromImage(path string) (float64, float64, error) {
	// crop the image
	croppedImage, err := cropImage(path)
	if err != nil {
		return 0, 0, err
	}
	// read off the text
	textFromImage, err := readText(croppedImage)
	if err != nil {
		return 0, 0, err
	}
	// pull out the coords
	coordStr, err := extractCoords(textFromImage)
	if err != nil {
		return 0, 0, err
	}

	lat, lon, err := helpers.ParseLatLng(coordStr)
	return lat, lon, err
}

// cropImage cuts a dashcam screencap down to just the bottom right corner
func cropImage(srcFilename string) (string, error) {

	//TODO: if config.CroppedCornersDir isn't accessible, because we can't proceed

	// exit early if the cropped file already exists
	croppedFile := filepath.Join(config.CroppedCornersDir, path.Base(srcFilename))
	if helpers.FileExists(croppedFile) {
		return croppedFile, nil
	}

	// open the image
	src, err := imaging.Open(srcFilename)
	if err != nil {
		if strings.Contains(err.Error(), "too many open files") {
			// this is fatal intentionally, else you get lots of similar errors
			log.Fatal("too many open files")
		}
		//TODO: automatically create a screencap file here
		if strings.Contains(err.Error(), "no such file or directory") {
			err = errors.New("screencap file was not present")
		}
		terrors.Log(err, "failed to open image")
		return "", err
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
		// terrors.Log(err, "failed to save image")
		return "", err
	}
	return croppedFile, err
}

// readText uses OCR to read the text from an image file
func readText(imgFile string) (string, error) {
	//TODO: check that we even have a valid file to open

	client := gosseract.NewClient()
	// defer client.Close()

	// set up tesseract to improve OCR accuracy
	client.SetConfigFile(path.Join(helpers.ProjectRoot(), "configs/tesseract.cfg"))
	// https://github.com/tesseract-ocr/tesseract/wiki/ImproveQuality#page-segmentation-method
	//TODO: use single line
	client.SetPageSegMode(gosseract.PSM_SINGLE_BLOCK)

	// read the file
	client.SetImage(imgFile)
	text, err := client.Text()
	client.Close()
	if err != nil {
		// terrors.Log(err, "failed to read text")
		return "", err
	}
	return text, err
}

// extractCoords expects an OCR-ed string which
// may or may not contain GPS coordinates, and
// returns its best guess at what the coords are
func extractCoords(text string) (string, error) {
	// strip all whitespace
	tidy := strings.Replace(text, " ", "", -1)
	// try to separate the text using the speed
	split := helpers.SplitOnRegex(tidy, "MPH")
	// if we didn't find the speed, just exit
	if len(split) < 2 {
		return "", errors.New("didn't find MPH in string")
	}
	// use only the second half (the GPS coordinates)
	coords := split[1]
	return coords, nil
}
