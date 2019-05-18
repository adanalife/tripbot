package store

import (
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

const (
	userJoinsBucket   = "user_joins"
	userWatchedBucket = "user_watched"
)

func (s *Store) MilesForUser(user string) int {
	prevDuration := s.DurationForUser(user)
	currDuration := s.CurrentViewDuration(user)
	return helpers.DurationToMiles(prevDuration + currDuration)
}

// fetch the current view duration
func (s *Store) CurrentViewDuration(user string) time.Duration {
	var joinTime time.Time
	err := s.db.View(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		err := joinTime.UnmarshalText(joinedBucket.Get([]byte(user)))
		return err
	})
	if err != nil {
		log.Printf("encountered error getting current view duration: %s", err)
		return 0
	}
	return time.Since(joinTime)
}

func (s *Store) TopUsers(size int) []string {
	var reversedMap = make(map[int]string)
	var topUsers = []string{}

	sortedValues := []int{}

	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))
		//TODO: do I need this?
		// joinedBucket := tx.Bucket([]byte(userJoinsBucket))

		err := watchedBucket.ForEach(func(k, v []byte) error {
			user := string(k)
			// don't include these in leaderboard
			//TODO: this should be a func
			for _, ignored := range helpers.IgnoredUsers {
				if user == ignored {
					return nil
				}
			}

			// fetch current view duration...
			// currentDuration := s.CurrentViewDuration(user)
			// ...and the previous view duration
			duration, err := time.ParseDuration(string(v))
			if err != nil {
				return err
			}
			// add them together
			intDuration := int(duration) // + int(currentDuration)
			sortedValues = append(sortedValues, intDuration)
			reversedMap[intDuration] = user
			return err
		})

		return err
	})

	// sort and reverse
	sort.Sort(sort.Reverse(sort.IntSlice(sortedValues)))

	// print the top 5
	for i := 0; i < size; i++ {
		topUsers = append(topUsers, reversedMap[sortedValues[i]])
	}

	return topUsers
}

func (s *Store) DurationForUser(user string) time.Duration {
	var previousDurationWatched *time.Duration

	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))

		// fetch the previous duration watched from the DB
		duration, err := time.ParseDuration(string(watchedBucket.Get([]byte(user))))
		previousDurationWatched = &duration
		return err
	})
	return *previousDurationWatched
}

func (s *Store) PrintStats() {
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

func (s *Store) RecordUserJoin(user string) {
	log.Println(user, "joined the channel")

	s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		currentTime, err := time.Now().MarshalText()
		if err != nil {
			return err
		}
		err = joinedBucket.Put([]byte(user), []byte(currentTime))
		return err
	})
}

func (s *Store) RecordUserPart(user string) {
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
