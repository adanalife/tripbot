package background

import (
	"log"

	"github.com/go-co-op/gocron/v2"
)

var Scheduler gocron.Scheduler

func StartCron() {
	log.Println("Starting cron")
	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("error creating gocron scheduler: %v", err)
	}
	Scheduler = s
	Scheduler.Start()
}

// StopCron shuts down the scheduler. In-flight job contexts are canceled,
// so any ctx-aware work in those jobs unwinds rather than running to
// completion. Cron jobs here are short idempotent ticks that retry on the
// next interval, so losing an in-flight execution is fine.
func StopCron() {
	log.Println("Stopping cron")
	if Scheduler == nil {
		return
	}
	if err := Scheduler.Shutdown(); err != nil {
		log.Printf("error shutting down gocron scheduler: %v", err)
	}
}
