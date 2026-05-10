package background

import (
	"log"

	"github.com/go-co-op/gocron/v2"
)

// Scheduler is the gocron v2 scheduler used to register and run cron jobs.
// It replaces the previous robfig/cron-based scheduler so that jobs can
// receive a context.Context for OTel trace propagation (see #431 for the
// span wrapper that needed ctx threading).
var Scheduler gocron.Scheduler

// StartCron initializes the scheduler and starts it. Call before scheduling
// any jobs.
func StartCron() {
	log.Println("Starting cron")
	s, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("error creating gocron scheduler: %v", err)
	}
	Scheduler = s
	Scheduler.Start()
}

// StopCron shuts down the scheduler. In-flight jobs have their contexts
// cancelled (vs. robfig/cron's behavior of letting jobs finish on shutdown).
func StopCron() {
	log.Println("Stopping cron")
	if Scheduler == nil {
		return
	}
	if err := Scheduler.Shutdown(); err != nil {
		log.Printf("error shutting down gocron scheduler: %v", err)
	}
}
