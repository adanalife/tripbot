package main

import (
	"log"
	"sort"
	"time"

	"github.com/boltdb/bolt"
)

const userWatchedBucket = "user_watched"

func durationToMiles(d time.Duration) int {
	return int(d.Minutes() / 15)
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
			duration, err := time.ParseDuration(string(v))
			if err != nil {
				return err
			}
			intDuration := int(duration)
			sortedValues = append(sortedValues, intDuration)
			reversedMap[intDuration] = string(k)
			return err
		})
		return err
	})

	sort.Sort(sort.Reverse(sort.IntSlice(sortedValues)))
	// spew.Dump(sortedValues)

	for i := 0; i < 5; i++ {
		// duration, err := time.ParseDuration(sortedValues[i].(time.Duration))
		// if err != nil {
		// 	panic(err)
		// }
		// var duration time.Duration
		// duration = sortedValues[i]
		duration := time.Duration(sortedValues[i])
		user := reversedMap[sortedValues[i]]

		log.Println(user, "has", duration)
	}
}
