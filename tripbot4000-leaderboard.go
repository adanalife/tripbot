package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/boltdb/bolt"
)

const (
	userWatchedBucket = "user_watched"
	userJoinsBucket   = "user_joins"
)

var ignoredUsers = []string{
	"adanalife_",
	"tripbot4000",
	"nightbot",
}

func durationToMiles(d time.Duration) int {
	return int(d.Minutes() / 10)
}

func main() {
	db, err := bolt.Open("tripbot-copy.db", 0666, &bolt.Options{ReadOnly: true})
	if err != nil {
		log.Fatal(err)
	}

	sortedValues := []int{}
	var reversedMap = make(map[int]string)

	db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))
		joinedBucket := tx.Bucket([]byte(userJoinsBucket))

		err := watchedBucket.ForEach(func(k, v []byte) error {
			user := string(k)
			// don't include these in leaderboard
			for _, ignored := range ignoredUsers {
				if user == ignored {
					return nil
				}
			}

			// fetch the current view duration
			var joinTime time.Time
			var currentDuration time.Duration
			err := joinTime.UnmarshalText(joinedBucket.Get([]byte(user)))
			if err != nil {
				currentDuration = 0
			} else {
				currentDuration = time.Since(joinTime)
			}

			duration, err := time.ParseDuration(string(v))
			if err != nil {
				return err
			}
			intDuration := int(duration) + int(currentDuration)
			sortedValues = append(sortedValues, intDuration)
			reversedMap[intDuration] = user
			return err
		})

		return err
	})

	// sort and reverse the values
	sort.Sort(sort.Reverse(sort.IntSlice(sortedValues)))

	fmt.Println("Odometer Leaderboard")
	// print the top 5
	for i := 0; i < 5; i++ {
		duration := time.Duration(sortedValues[i])
		user := reversedMap[sortedValues[i]]

		fmt.Println(durationToMiles(duration), "miles:", user)
	}
}
