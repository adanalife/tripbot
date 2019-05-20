package main

import (
	"time"

	"github.com/dmerrick/danalol-stream/pkg/config"
	"github.com/dmerrick/danalol-stream/pkg/store"
)

const (
	user = "olivecat50"
	dur  = "154h12m38.296097s"
)

func main() {
	datastore := store.FindOrCreate(config.DbPath)
	parsedDur, _ := time.ParseDuration(dur)
	datastore.GiveUserDuration(user, parsedDur)
}
