package store

import (
	"log"
	"sort"
	"time"

	"github.com/boltdb/bolt"
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
		watchedBucket := tx.Bucket([]byte(helpers.UserWatchedBucket))

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
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))
		err := joinTime.UnmarshalText(joinedBucket.Get([]byte(user)))
		return err
	})
	if err != nil {
		// log.Printf("encountered error getting current view duration: %s", err)
		return 0
	}
	return time.Since(joinTime)
}

func (s *Store) TopUsers(size int) []string {
	var reversedMap = make(map[int]string)
	var topUsers = []string{}

	sortedValues := []int{}

	s.db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(helpers.UserWatchedBucket))
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
		joinedBucket := tx.Bucket([]byte(helpers.UserJoinsBucket))
		watchedBucket := tx.Bucket([]byte(helpers.UserWatchedBucket))

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
		joinedBucket := tx.Bucket([]byte(helpers.UserJoinsBucket))
		watchedBucket := tx.Bucket([]byte(helpers.UserWatchedBucket))

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
