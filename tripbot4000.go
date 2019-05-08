package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/boltdb/bolt"
	twitch "github.com/gempir/go-twitch-irc"
)

const (
	clientUsername    = "TripBot4000"
	channelToJoin     = "adanalife_"
	userJoinsBucket   = "user_joins"
	userWatchedBucket = "user_watched"
)

// these are other bots who shouldn't get points
var ignoredUsers = []string{
	"anotherttvviewer",
	"commanderroot",
	"electricallongboard",
	"logviewer",
}

type Store struct {
	path string
	db   *bolt.DB
}

func NewStore(path string) *Store {
	return &Store{
		path: path,
	}
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
		tx.CreateBucketIfNotExists([]byte(userJoinsBucket))
		tx.CreateBucketIfNotExists([]byte(userWatchedBucket))
		return nil
	}); err != nil {
		s.Close()
		return err
	}

	return nil
}

func (s *Store) Close() error {
	if s.db != nil {
		s.db.Close()
	}
	return nil
}

// datastore for user joins
// var userJoins map[string]time.Time = make(map[string]time.Time)
// var userWatched map[string]time.Duration = make(map[string]time.Duration)

// returns true if a given user should be ignored
func userIsIgnored(user string) bool {
	for _, ignored := range ignoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}

func (s *Store) recordUserJoin(user string) {
	log.Println(user, "joined the channel")

	s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(userJoinsBucket))
		currentTime, err := time.Now().MarshalText()
		err = b.Put([]byte(user), []byte(currentTime))
		return err
	})
}

func (s *Store) recordUserPart(user string) {
	var joinTime time.Time
	var previousDurationWatched time.Duration

	// first find the time the user joined the channel
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(userJoinsBucket))
		joinTime = time.Now()
		err := joinTime.UnmarshalText(b.Get([]byte(user)))
		return err
	})

	// if we didn't find anything, we can't continue
	// if joinTime == nil {
	// 	return err
	// }

	// seems like we did find a time, so calculate the duration watched
	durationWatched := time.Since(joinTime)

	// fetch the previous duration watched from the DB
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(userWatchedBucket))
		previousDurationWatched, err := time.ParseDuration(string(b.Get([]byte(user))))
		return err
	})

	// maybe they're a new user
	// if previousDurationWatched == 0 {
	// 	previousDurationWatched = 0
	// }

	// calculate total duration watched
	totalDurationWatched := previousDurationWatched + durationWatched

	// update the DB with the total duration watched
	s.db.Update(func(tx *bolt.Tx) error {
		// convert Duration to []byte
		// c.p. https://stackoverflow.com/a/23004209
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		err := enc.Encode(totalDurationWatched)
		if err != nil {
			return err
		}
		b := tx.Bucket([]byte(userWatchedBucket))
		err = b.Put([]byte(user), []byte(buf.Bytes()))
		return err
	})

	log.Println(user, "left the channel, total watched:", fmtDuration(totalDurationWatched))
}

// helper func to make Durations prettier
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	return fmt.Sprintf("%02d:%02d", h, m)
}

func main() {
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	datastore := NewStore("tripbot.db")
	if err := datastore.Open(); err != nil {
		panic(err)
	}

	client := twitch.NewClient(clientUsername, clientAuthenticationToken)

	client.OnUserJoinMessage(func(joinMessage twitch.UserJoinMessage) {
		if !userIsIgnored(joinMessage.User) {
			datastore.recordUserJoin(joinMessage.User)
			// log.Println(joinMessage.Raw)
		}
	})

	client.OnUserPartMessage(func(partMessage twitch.UserPartMessage) {
		if !userIsIgnored(partMessage.User) {
			datastore.recordUserPart(partMessage.User)
			// log.Println(partMessage.Raw)
		}
	})

	client.Join(channelToJoin)
	log.Println("Joined channel", channelToJoin)

	err := client.Connect()
	if err != nil {
		panic(err)
	}
}
