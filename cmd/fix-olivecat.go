package main

import (
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/store"
)

const (
	user = "downonluk"
	dur  = "1h56m30.323223s"
)

func main() {
	datastore := store.FindOrCreate(config.DbPath)
	parsedDur, _ := time.ParseDuration(dur)
	datastore.GiveUserDuration(user, parsedDur)
}
