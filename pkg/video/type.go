package video

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/database"
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

func LoadOrCreate(path string) (Video, error) {
	slug := slug(path)
	log.Println("slug is:", slug)

	// try to find the slug in the DB
	videos := []Video{}
	query := fmt.Sprintf("SELECT * FROM videos WHERE slug='%s'", slug)
	err := database.DBCon.Select(&videos, query)
	if err != nil {
		log.Println("error fetching vid from DB:", err)
		// create a new video
		newVid, err := create(slug)
		return newVid, err
	}

	// did we find anything in the DB?
	if len(videos) == 0 {
		log.Println("no matches, creating a new Video")
		newVid, err := create(slug)
		if err != nil {
			log.Println("error creating new vid:", err)
		}
		return newVid, err
	}
	return videos[0], nil
}

// create will create a new Video from a slug
func create(file string) (Video, error) {
	var newVid Video
	var blankDate time.Time

	if file == "" {
		return newVid, errors.New("no file provided")
	}
	slug := slug(file)

	// validate the dash string
	err := validate(slug)
	if err != nil {
		return newVid, err
	}

	// create new (mostly) empty vid
	newVid = Video{
		Slug:        slug,
		Lat:         0,
		Lng:         0,
		Flagged:     false,
		DateFilmed:  blankDate,
		DateCreated: blankDate,
	}

	// store the video in the DB
	err = newVid.save()

	return newVid, err
}

func (v Video) save() error {
	flagged := false
	// try to get at least one good coords pair
	lat, lng, err := v.LatLng()
	if err != nil {
		log.Println("error fetching coords:", err)
		flagged = true
	}

	tx := database.DBCon.MustBegin()
	tx.MustExec(
		"INSERT INTO videos (slug, lat, lng, date_filmed, flagged) VALUES ($1, $2, $3, $4, $5)",
		v.Slug,
		lat,
		lng,
		v.Date(),
		flagged,
	)
	return tx.Commit()
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

// Date returns a time.Time object for the video
func (v Video) Date() time.Time {
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

// LatLng will use OCR to read the coordinates from a screenshot (seriously)
func (v Video) LatLng() (float64, float64, error) {
	for _, timestamp := range timestampsToTry {
		lat, lon, err := ocr.CoordsFromImage(v.screencap(timestamp))
		if err == nil {
			return lat, lon, err
		}
	}
	return 0, 0, errors.New("none of the screencaps had valid coords")
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

func validate(dashStr string) error {
	if len(dashStr) < 20 {
		return errors.New("dash string too short")
	}
	shortened := dashStr[:20]

	if strings.HasPrefix(".", shortened) {
		return errors.New("dash string can't be a hidden file")
	}

	//TODO: this should probably live in an init()
	var validDashStr = regexp.MustCompile(`^[_0-9]{20}$`)
	if !validDashStr.MatchString(shortened) {
		return errors.New("dash string did not match regex")
	}
	return nil
}

func removeFileExtension(filename string) string {
	ext := path.Ext(filename)
	return filename[0 : len(filename)-len(ext)]
}

// slug strips the path and extension off the file
func slug(file string) string {
	fileName := path.Base(file)
	return removeFileExtension(fileName)
}
