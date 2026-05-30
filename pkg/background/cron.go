package background

import (
	"log/slog"

	"github.com/go-co-op/gocron/v2"
)

// Scheduler owns the gocron scheduler. Construct one with New(), Start() it,
// register work with NewJob(), and Stop() it during shutdown. Replaces the
// former package-level Scheduler global so the scheduler is constructed in
// main() and injected rather than reached through package state.
type Scheduler struct {
	sched gocron.Scheduler
}

// New constructs a Scheduler. The underlying gocron scheduler is created but
// not started; call Start once jobs are registered.
func New() (*Scheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	return &Scheduler{sched: s}, nil
}

// Start begins executing registered jobs.
func (s *Scheduler) Start() {
	slog.Info("starting cron")
	s.sched.Start()
}

// NewJob registers a job. It mirrors gocron's NewJob signature so callers keep
// using gocron.DurationJob / gocron.NewTask directly.
func (s *Scheduler) NewJob(jd gocron.JobDefinition, task gocron.Task, opts ...gocron.JobOption) (gocron.Job, error) {
	return s.sched.NewJob(jd, task, opts...)
}

// Stop shuts down the scheduler. In-flight job contexts are canceled, so any
// ctx-aware work in those jobs unwinds rather than running to completion. Cron
// jobs here are short idempotent ticks that retry on the next interval, so
// losing an in-flight execution is fine. Safe to call on a nil receiver or
// before Start, so shutdown paths can call it unconditionally.
func (s *Scheduler) Stop() error {
	if s == nil || s.sched == nil {
		return nil
	}
	slog.Info("stopping cron")
	return s.sched.Shutdown()
}
