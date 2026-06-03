package chatbot

// Cron is the background-scheduler surface the chatbot needs: stopping the
// scheduler during !shutdown. *background.Scheduler satisfies it directly, so
// cmd/tripbot assigns the constructed scheduler straight onto App.Cron.
type Cron interface {
	Stop() error
}

// noopCron swallows Stop. It's the App's Cron default (set in New()), covering
// the brief startup window before cmd/tripbot installs the real scheduler — and
// every test App.
type noopCron struct{}

func (noopCron) Stop() error { return nil }
