package chatbot

// Cron is the background-scheduler surface the chatbot needs: stopping the
// scheduler during !shutdown. gocron.Scheduler satisfies it directly, so
// cmd/tripbot assigns the constructed scheduler straight onto App.Cron.
type Cron interface {
	Shutdown() error
}

// noopCron swallows Shutdown. It's the App's Cron default (set in New()),
// covering the brief startup window before cmd/tripbot installs the real
// scheduler — and every test App.
type noopCron struct{}

func (noopCron) Shutdown() error { return nil }
