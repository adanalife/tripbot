package store

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
	"github.com/dmerrick/danalol-stream/pkg/ocr"
)

func (s *Store) CoordsFromVideoPath(videoPath string) (float64, float64, error) {
	videoStr := helpers.ToVidStr(videoPath)
	// first look up the coords in the DB
	lat, lon, err := s.FetchSavedCoords(videoStr)
	if err == nil {
		fmt.Println("coords found for", videoStr, lat, lon)
		// cool, they were in the DB already
		return lat, lon, err
	}
	// okay we need to pull them from the video file
	lat, lon, err = ocr.CoordsFromVideoWithRetry(videoStr)
	if err != nil {
		// something went wrong reading the coords
		return lat, lon, err
	}
	// now save these coords in the DB for next time
	err = s.StoreCoords(videoStr, lat, lon)
	fmt.Println("storing coords for", videoStr, lat, lon)
	return lat, lon, err
}

func (s *Store) FetchSavedCoords(vidStr string) (float64, float64, error) {
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

func (s *Store) StoreCoords(vidStr string, lat, lon float64) error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		coordsBucket := tx.Bucket([]byte(config.CoordsBucket))

		// convert to a string
		coordsStr := fmt.Sprintf("%d,%d", lat, lon)

		// insert into bucket
		err := coordsBucket.Put([]byte(vidStr), []byte(coordsStr))
		return err
	})
	return err
}
