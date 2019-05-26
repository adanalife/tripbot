package store

import (
	"log"
	"sort"
	"time"

	"github.com/boltdb/bolt"
	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

func (s *Store) MilesForUser(user string) int {
	prevDuration := s.DurationForUser(user)
	currDuration := s.CurrentViewDuration(user)
	return helpers.DurationToMiles(prevDuration + currDuration)
}

func (s *Store) DurationForUser(user string) time.Duration {
	var previousDurationWatched *time.Duration

	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(config.UserWatchedBucket))

		// fetch the previous duration watched from the DB
		duration, err := time.ParseDuration(string(watchedBucket.Get([]byte(user))))
		previousDurationWatched = &duration
		return err
	})
	return *previousDurationWatched
}

// fetch the current view duration
func (s *Store) CurrentViewDuration(user string) time.Duration {
	var joinTime time.Time
	err := s.db.View(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(config.UserJoinsBucket))
		joinStr := string(joinedBucket.Get([]byte(user)))
		if joinStr == "" {
			log.Printf("join time was empty for %s", user)
			return nil
		}
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
	var topUsers = []string{}
	var reversedMap = make(map[int]string)
	var sortedValues = []int{}

	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(config.UserWatchedBucket))

		err := watchedBucket.ForEach(func(k, v []byte) error {
			user := string(k)
			// don't include these in leaderboard
			if helpers.UserIsIgnored(user) {
				return nil
			}

			// fetch current view duration...
			currDuration := s.CurrentViewDuration(user)
			// ...and the previous view duration
			prevDuration, err := time.ParseDuration(string(v))
			if err != nil {
				return err
			}
			// add them together
			intDuration := int(currDuration) + int(prevDuration)
			sortedValues = append(sortedValues, intDuration)
			reversedMap[intDuration] = user
			return err
		})

		return err
	})

	// sort and reverse
	sort.Sort(sort.Reverse(sort.IntSlice(sortedValues)))

	// include only the specified-number of users
	for i := 0; i < size; i++ {
		topUsers = append(topUsers, reversedMap[sortedValues[i]])
	}

	return topUsers
}

func (s *Store) RecordUserJoin(user string) {
	log.Println(user, "joined the channel")

	s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(config.UserJoinsBucket))
		currentTime, err := time.Now().MarshalText()
		if err != nil {
			return err
		}
		err = joinedBucket.Put([]byte(user), []byte(currentTime))
		return err
	})
}

func (s *Store) RecordUserPart(user string) {
	// calculate total duration watched
	previousDurationWatched := s.DurationForUser(user)
	currentDurationWatched := s.CurrentViewDuration(user)
	totalDurationWatched := previousDurationWatched + currentDurationWatched

	// update the DB with the total duration watched
	err := s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(config.UserJoinsBucket))
		watchedBucket := tx.Bucket([]byte(config.UserWatchedBucket))

		// remove the user from the joined bucket
		err := joinedBucket.Delete([]byte(user))
		if err != nil {
			return err
		}
		err = watchedBucket.Put([]byte(user), []byte(totalDurationWatched.String()))
		return err
	})
	if err != nil {
		log.Printf("there was an error, user=%s, total=%s, current=%s", user, totalDurationWatched, currentDurationWatched)
	}

	log.Printf("%s left the channel (total: %s, session: %s)", user, totalDurationWatched, currentDurationWatched)
}

func (s *Store) GiveUserDuration(user string, durToAdd time.Duration) {
	var err error

	previousDurationWatched := s.DurationForUser(user)

	// calculate total duration watched
	totalDurationWatched := previousDurationWatched + durToAdd

	// update the DB with the total duration watched
	s.db.Update(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(config.UserWatchedBucket))

		err = watchedBucket.Put([]byte(user), []byte(totalDurationWatched.String()))
		return err
	})

	log.Printf("gave %s %s, new total: %s)", user, durToAdd, totalDurationWatched)
}

func (s *Store) ClearJoinBucket() error {
	err := s.db.Update(func(tx *bolt.Tx) error {
		joinedBucket := tx.Bucket([]byte(config.UserJoinsBucket))
		err := joinedBucket.ForEach(func(k, v []byte) error {
			user := string(k)
			// remove the user from the joined bucket
			err := joinedBucket.Delete([]byte(user))
			return err
		})
		return err
	})
	return err
}
