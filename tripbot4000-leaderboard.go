package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/boltdb/bolt"
)

const userWatchedBucket = "user_watched"

func durationToMiles(d time.Duration) int {
	return int(d.Minutes() / 10)
}

func main() {
	db, err := bolt.Open("tripbot.db", 0666, &bolt.Options{ReadOnly: true})
	if err != nil {
		log.Fatal(err)
	}

	sortedValues := []int{}
	var reversedMap = make(map[int]string)

	db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))
		err := watchedBucket.ForEach(func(k, v []byte) error {
			user := string(k)
			if user == "tripbot4000" {
				return nil
			}
			duration, err := time.ParseDuration(string(v))
			if err != nil {
				return err
			}
			intDuration := int(duration)
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
