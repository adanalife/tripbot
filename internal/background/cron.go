package background

import (
	"log"

	"github.com/robfig/cron"
)

var Cron *cron.Cron

func StartCron() {
	log.Println("Starting cron")
	Cron = cron.New()
	Cron.Start()
}

func StopCron() {
	log.Println("Stopping cron")
	Cron.Stop()
}
