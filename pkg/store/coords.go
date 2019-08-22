package store

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/video"
)

// these are videos where the OCR glitched for one reason or another
var knownBadVids = []string{
	"2018_1009_162218_025",
	"2018_1009_172519_046",
	"2018_1009_175519_056",
	"2018_1009_185504_007",
}

func (s *Store) CoordsFor(vid video.Video) (float64, float64, error) {
	videoStr := vid.String()

	// check to see if we should skip this video
	for _, vd := range knownBadVids {
		if videoStr == vd {
			return 0, 0, errors.New("skipping known bad point")
		}
	}

	// okay now look up the coords in the DB
	lat, lon, err := s.fetchSavedCoords(videoStr)
	if err == nil {
		// cool, they were in the DB already
		return lat, lon, err
	}

	// okay we need to pull them from the video file
	lat, lon, err = vid.Location()
	if err != nil {
		// something went wrong reading the coords
		return lat, lon, err
	}
	// now save these coords in the DB for next time
	err = s.storeCoords(videoStr, lat, lon)
	return lat, lon, err
}

func (s *Store) fetchSavedCoords(vidStr string) (float64, float64, error) {
	var lat, lon float64
	var err error

	err = s.db.View(func(tx *bolt.Tx) error {
		coordsBucket := tx.Bucket([]byte(config.CoordsBucket))

		// fetch the coords from the DB
		coordsStr := coordsBucket.Get([]byte(vidStr))
		if coordsStr == nil {
			return errors.New("no coords found in bucket")
		}
		coordsSlice := helpers.SplitOnRegex(string(coordsStr), ",")
		lat, err = strconv.ParseFloat(coordsSlice[0], 64)
		lon, err = strconv.ParseFloat(coordsSlice[1], 64)
		return err
	})
	return lat, lon, err
}

func (s *Store) storeCoords(vidStr string, lat, lon float64) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		coordsBucket := tx.Bucket([]byte(config.CoordsBucket))

		// convert to a string
		coordsStr := fmt.Sprintf("%f,%f", lat, lon)

		// insert into bucket
		err := coordsBucket.Put([]byte(vidStr), []byte(coordsStr))
		return err
	})
	return err
}
