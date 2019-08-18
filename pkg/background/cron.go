package background

import (
	"log"

	"github.com/robfig/cron"
)

var Cron *cron.Cron

func StartCron() {
	log.Println("Starting cron")
	Cron = cron.New()
	Cron.AddFunc("@every 1m30s", Chatter)
	Cron.Start()
}

func StopCron() {
	Cron.Stop()
}
