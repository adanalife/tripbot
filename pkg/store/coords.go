package store

import (
	"fmt"
	"strconv"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

func (s *Store) FetchSavedCoords(vidStr string) (float64, float64, error) {
	var lat, lon float64
	var err error

	err = s.db.View(func(tx *bolt.Tx) error {
		coordsBucket := tx.Bucket([]byte(config.CoordsBucket))

		// fetch the coords from the DB
		coordsStr := coordsBucket.Get([]byte(vidStr))
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
