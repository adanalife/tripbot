package video

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
)

// Videos represent a video file containing dashcam footage
type Video struct {
	Id          int           `db:"id"`
	Slug        string        `db:"slug"`
	Lat         float64       `db:"lat"`
	Lng         float64       `db:"lng"`
	NextVid     sql.NullInt64 `db:"next_vid"`
	PrevVid     sql.NullInt64 `db:"prev_vid"`
	Flagged     bool          `db:"flagged"`
	DateFilmed  time.Time     `db:"date_filmed"`
	DateCreated time.Time     `db:"date_created"`
}

// Location returns a lat/lng pair
//TODO: refactor out the error return value
func (v Video) Location() (float64, float64, error) {
	var err error
	if v.Flagged {
		err = errors.New("video is flagged")
	}
	return v.Lat, v.Lng, err
}

// ex: 2018_0514_224801_013_a_opt
func (v Video) String() string {
	return v.Slug
}

// a DashStr is the string we get from the dashcam
// an example file: 2018_0514_224801_013.MP4
// an example dashstr: 2018_0514_224801_013
// ex: 2018_0514_224801_013
func (v Video) DashStr() string {
	//TODO: this never should have happened, but it did and it crashed the bot
	if len(v.Slug) < 20 {
		return ""
	}
	return v.Slug[:20]
}

// ex: 2018_0514_224801_013.MP4
func (v Video) File() string {
	return fmt.Sprintf("%s.MP4", v.Slug)
}

// ex: /Volumes/.../2018_0514_224801_013.MP4
func (v Video) Path() string {
	return path.Join(config.VideoDir(), v.File())
}

// toDate parses the vidStr and returns a time.Time object for the video
func (v Video) toDate() time.Time {
	vidStr := v.String()
	year, _ := strconv.Atoi(vidStr[:4])
	month, _ := strconv.Atoi(vidStr[5:7])
	day, _ := strconv.Atoi(vidStr[7:9])
	hour, _ := strconv.Atoi(vidStr[10:12])
	minute, _ := strconv.Atoi(vidStr[12:14])
	second, _ := strconv.Atoi(vidStr[14:16])

	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	return t
}

// ocrCoords will use OCR to read the coordinates from a screenshot (seriously)
func (v Video) ocrCoords() (float64, float64, error) {
	for _, timestamp := range timestampsToTry {
		lat, lon, err := ocr.CoordsFromImage(v.screencap(timestamp))
		if err == nil {
			return lat, lon, err
		}
	}
	return 0, 0, errors.New("none of the screencaps had valid coords")
}

// slug strips the path and extension off the file
func slug(file string) string {
	fileName := path.Base(file)
	return removeFileExtension(fileName)
}

// these are different timestamps we have screenshots prepared for
// the "000" corresponds to 0m0s, "130" corresponds to 1m30s
var timestampsToTry = []string{
	"000",
	"015",
	"030",
	"045",
	"100",
	"115",
	"130",
	"145",
	"200",
	"215",
	"230",
	"245",
}

// timestamp is something like 000, 030, 100, etc
func (v Video) screencap(timestamp string) string {
	screencapFile := fmt.Sprintf("%s-%s.png", v.DashStr(), timestamp)
	return path.Join(config.ScreencapDir(), timestamp, screencapFile)
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
