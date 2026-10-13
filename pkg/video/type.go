package video

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"regexp"
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

// Corpus values for Video.Corpus (videos.corpus): which dashcam trip a clip
// belongs to, and so which playlist it can be on. See migration 057.
const (
	CorpusS1     = "s1"     // 2018, real time
	CorpusS2     = "s2"     // 2026, real time
	CorpusS2Fast = "s2fast" // 2026, the camera's 6x time-lapse mode
)

// Slug shapes, one per season. s1Slug is the camera's date, time and sequence
// (the trim/encode suffixes that follow are not checked). s2Slug is the
// season-2 name: the original card file's 14-digit stem and sequence, then
// the piece's offset into that original in seconds. Each capture group is one
// field of the timestamp, in time.Date order.
var (
	s1Slug = regexp.MustCompile(`^(\d{4})_(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})_\d{3}`)
	s2Slug = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})_\d{6}_s(\d{4})$`)
)

// Videos represent a video file containing dashcam footage
type Video struct {
	ID      int `gorm:"primaryKey"`
	Slug    string
	Lat     float64
	Lng     float64
	NextVid sql.NullInt64
	PrevVid sql.NullInt64
	Flagged bool
	State   string
	// City and CityM are the clip's own coordinate named offline by the pipeline,
	// the same way a Moment is. They are what answers a clip whose per-moment
	// track isn't trustworthy enough to read from; empty until the geocode pass
	// has reached the row. CityM is metres from City, 0 meaning inside it.
	City        string
	CityM       *float64
	CoordSource string
	// CoordConfidence is how much this clip's per-moment video_coords track is
	// worth believing, 0..1. NULL — a nil pointer here — means the coords stage
	// hasn't looked at the clip yet, which is not the same as looking and
	// finding nothing, so it can't be flattened to 0. See CoordAt.
	CoordConfidence *float64
	// Corpus is the trip the clip belongs to (CorpusS1 and friends). Speed is
	// how many real seconds pass per second of clip: 1 for real-time footage,
	// 6 for s2fast. Both are the pipeline's to write at ingest; a row minted at
	// runtime gets the corpus its slug's shape implies and the column default
	// speed, because the name of a season-2 piece doesn't say how fast it runs.
	// The default tags let a Video built without them take the column defaults
	// instead of writing zero values.
	Corpus     string `gorm:"default:'s1'"`
	Speed      int    `gorm:"default:1"`
	DateFilmed time.Time
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

// String returns the slug, which callers rely on as a stable identity — don't
// enrich it with display fields.
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

// toDate reads the camera timestamp out of the slug, in the season's shape. A
// season-2 piece is stamped at its offset into the original, so consecutive
// pieces of one original three minutes apart read three minutes apart. A slug
// in neither shape has no date; validate rejects it before anything is saved.
func (v Video) toDate() time.Time {
	if m := s1Slug.FindStringSubmatch(v.Slug); m != nil {
		return stamp(m[1:7], 0)
	}
	if m := s2Slug.FindStringSubmatch(v.Slug); m != nil {
		offset, _ := strconv.Atoi(m[7])
		return stamp(m[1:7], offset)
	}
	return time.Time{}
}

// stamp builds a UTC time from six digit-only fields (year, month, day, hour,
// minute, second) plus an offset in seconds.
func stamp(fields []string, offsetSec int) time.Time {
	var n [6]int
	for i, f := range fields {
		n[i], _ = strconv.Atoi(f)
	}
	return time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5]+offsetSec, 0, time.UTC)
}

// slugCorpus is the corpus a slug's shape implies, or "" when it is in no
// season's shape. A season-2 name is read as s2: the fast corpus has the same
// shape, and only the pipeline's measurement of the clock tells them apart.
func slugCorpus(slug string) string {
	switch {
	case s1Slug.MatchString(slug):
		return CorpusS1
	case s2Slug.MatchString(slug):
		return CorpusS2
	}
	return ""
}

// FilmedAt is when the moment clip seconds into the video was filmed: the
// clip's start plus the driving time those seconds stand for. A video with
// no film date stays zero, so callers keep treating it as unknown.
func (v Video) FilmedAt(clip time.Duration) time.Time {
	if v.DateFilmed.IsZero() {
		return v.DateFilmed
	}
	return v.DateFilmed.Add(v.realElapsed(clip))
}

// realElapsed converts a span of clip time to the driving time it stands for.
// A second of an s2fast clip is six seconds of road, so anything that
// differences over clip time — a speed, an elapsed time — goes through here.
// A Speed of 0 is a Video that never came from the DB and reads as real time.
func (v Video) realElapsed(clip time.Duration) time.Duration {
	if v.Speed > 1 {
		return clip * time.Duration(v.Speed)
	}
	return clip
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
