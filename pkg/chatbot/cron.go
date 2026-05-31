package chatbot

import "sync"

// Cron is the background-scheduler surface the chatbot needs: stopping the
// scheduler during !shutdown. *background.Scheduler satisfies it directly.
type Cron interface {
	Stop() error
}

// realCron is the production Cron adapter the App is wired with at package
// init. Delegates to scheduler, which cmd/tripbot replaces with the
// constructed *background.Scheduler via SetScheduler once cron has started.
// Mirrors the realFlags shape.
type realCron struct{}

func (realCron) Stop() error {
	cronMu.RLock()
	s := scheduler
	cronMu.RUnlock()
	return s.Stop()
}

// scheduler is the Cron realCron delegates to. Initialised to a no-op so the
// brief startup window before cmd/tripbot installs the real scheduler — and
// every test App — has a safe Stop().
var (
	cronMu    sync.RWMutex
	scheduler Cron = noopCron{}
)

// noopCron swallows Stop. Used before SetScheduler runs and in tests.
type noopCron struct{}

func (noopCron) Stop() error { return nil }

// SetScheduler installs the Cron that realCron delegates to. Called from
// cmd/tripbot once the background scheduler is constructed and started.
func SetScheduler(s Cron) {
	cronMu.Lock()
	scheduler = s
	cronMu.Unlock()
}
