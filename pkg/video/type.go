package video

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strconv"
	"time"
)

// Provenance values for Video.CoordSource (videos.coord_source). See
// migration 020 and cmd/backfill-coords.
const (
	CoordSourceOCR          = "ocr"          // original dashcam-overlay OCR fix
	CoordSourceInterpolated = "interpolated" // synthesized from neighbouring clips
	CoordSourceRejected     = "rejected"     // OCR outlier discarded (coords cleared)
	CoordSourceMissing      = "missing"      // no GPS fix, none recoverable
)

// Videos represent a video file containing dashcam footage
type Video struct {
	ID          int `gorm:"primaryKey"`
	Slug        string
	Lat         float64
	Lng         float64
	NextVid     sql.NullInt64
	PrevVid     sql.NullInt64
	Flagged     bool
	State       string
	CoordSource string
	DateFilmed  time.Time
	// autoCreateTime stamps date_created on insert. A runtime-created clip (one
	// not already in the DB) is built without setting it, so without the tag
	// GORM writes the 0001-01-01 zero value over the column's DEFAULT
	// CURRENT_TIMESTAMP. See pkg/events for the full story.
	DateCreated time.Time `gorm:"autoCreateTime"`
}

// Location returns a lat/lng pair
// TODO: refactor out the error return value
func (v Video) Location() (float64, float64, error) {
	var err error
	if v.Flagged {
		err = errors.New("video is flagged")
	}
	return v.Lat, v.Lng, err
}

// String returns the slug, which callers (e.g. make-map's image filenames)
// rely on as a stable identity — don't enrich it with display fields.
// ex: 2018_0514_224801_013_a_opt
func (v Video) String() string {
	return v.Slug
}

// a DashStr is the string we get from the dashcam
// an example file: 2018_0514_224801_013.MP4
// an example dashstr: 2018_0514_224801_013
// ex: 2018_0514_224801_013
func (v Video) DashStr() string {
	// slugs shorter than 20 chars are malformed; return "" rather than
	// panic on the slice below
	if len(v.Slug) < 20 {
		return ""
	}
	return v.Slug[:20]
}

// ex: 2018_0514_224801_013.MP4
func (v Video) File() string {
	return fmt.Sprintf("%s.MP4", v.Slug)
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

// slug strips the path and extension off the file
func slug(file string) string {
	fileName := path.Base(file)
	return removeFileExtension(fileName)
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}
