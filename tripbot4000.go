package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
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
		var onlineUsers = []string{}

		// first we make a list of all of the online users
		s.db.View(func(tx *bolt.Tx) error {
			joinedBucket := tx.Bucket([]byte(userJoinsBucket))
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
			s.recordUserPart(string(user))
		}

		s.db.Close()
	}
	return nil
}

func (s *Store) printStats() {
	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))
		err := watchedBucket.ForEach(func(k, v []byte) error {
			duration, err := time.ParseDuration(string(v))
			log.Printf("%s has watched %s.\n", k, duration)
			return err
		})
		return err
	})
}

func (s *Store) recordUserJoin(user string) {
	log.Println(user, "joined the channel")

	s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		currentTime, err := time.Now().MarshalText()
		err = joinedBucket.Put([]byte(user), []byte(currentTime))
		return err
	})
}

func (s *Store) recordUserPart(user string) {
	var joinTime time.Time
	var durationWatched time.Duration
	var previousDurationWatched time.Duration

	s.db.View(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))

		// first find the time the user joined the channel
		err := joinTime.UnmarshalText(joinedBucket.Get([]byte(user)))
		if err != nil {
			return err
		}

		// seems like we did find a time, so calculate the duration watched
		durationWatched = time.Since(joinTime)

		// fetch the previous duration watched from the DB
		previousDurationWatched, err = time.ParseDuration(string(watchedBucket.Get([]byte(user))))
		return err
	})

	// calculate total duration watched
	totalDurationWatched := previousDurationWatched + durationWatched

	// update the DB with the total duration watched
	s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))

		err := watchedBucket.Put([]byte(user), []byte(totalDurationWatched.String()))
		if err != nil {
			return err
		}
		// remove the user from the joined bucket
		err = joinedBucket.Delete([]byte(user))
		return err
	})

	log.Println(user, "left the channel, total watched:", totalDurationWatched)
}

// helper func to make Durations prettier
// func fmtDuration(d time.Duration) string {
// 	d = d.Round(time.Minute)
// 	h := d / time.Hour
// 	d -= h * time.Hour
// 	m := d / time.Minute
// 	return fmt.Sprintf("%02d:%02d", h, m)
// }

func main() {
	clientAuthenticationToken, ok := os.LookupEnv("TWITCH_AUTH_TOKEN")
	if !ok {
		panic("You must set TWITCH_AUTH_TOKEN")
	}

	datastore := NewStore("tripbot.db")
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

	datastore.printStats()

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

// returns true if a given user should be ignored
func userIsIgnored(user string) bool {
	for _, ignored := range ignoredUsers {
		if user == ignored {
			return true
		}
	}
	return false
}
