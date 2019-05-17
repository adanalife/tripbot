package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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

	lines := []string{}

	db.View(func(tx *bolt.Tx) error {
		watchedBucket := tx.Bucket([]byte(userWatchedBucket))
		err := watchedBucket.ForEach(func(k, v []byte) error {
			duration, err := time.ParseDuration(string(v))
			lines = append(lines, fmt.Sprintf("%s has %s miles.\n", k, durationToMiles(duration)))
			return err
		})
		return err
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, strings.Join(lines, ""))
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
