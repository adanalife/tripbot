package store

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/config"
)

// this stores the current datastore
var currentDatastore *Store

type Store struct {
	path string
	db   *bolt.DB
}

func NewStore(path string) *Store {
	return &Store{
		path: path,
	}
}

func FindOrCreate(dbFile string) *Store {
	// use the pre-existing datastore if we have one
	if currentDatastore != nil {
		return currentDatastore
	}

	// initialize the database
	datastore := NewStore(dbFile)

	if err := datastore.Open(); err != nil {
		panic(err)
	}

	// catch CTRL-Cs and run datastore.Close()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		datastore.Close()
		os.Exit(1)
	}()

	// save the current datastore for use later
	currentDatastore = datastore

	return datastore
}

// Open opens and initializes the database.
func (s *Store) Open() error {
	// Open underlying data store.
	db, err := bolt.Open(s.path, 0666, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return err
	}
	s.db = db

	// Initialize all the required buckets.
	if err := s.db.Update(func(tx *bolt.Tx) error {
		tx.CreateBucketIfNotExists([]byte(config.UserJoinsBucket))
		tx.CreateBucketIfNotExists([]byte(config.UserWatchedBucket))
		return nil
	}); err != nil {
		s.Close()
		return err
	}

	return nil
}

func (s *Store) Close() error {
	if s.db != nil {
		var onlineUsers = []string{}

		// first we make a list of all of the online users
		s.db.View(func(tx *bolt.Tx) error {
			joinedBucket := tx.Bucket([]byte(config.UserJoinsBucket))
			err := joinedBucket.ForEach(func(k, _ []byte) error {
				user := string(k)
				onlineUsers = append(onlineUsers, user)
				return nil
			})
			return err
		})

		// then we loop over it and record the current watched duration
		for _, user := range onlineUsers {
			log.Println("logging out", user)
			s.RecordUserPart(string(user))
		}

		s.db.Close()
	}
	return nil
}
