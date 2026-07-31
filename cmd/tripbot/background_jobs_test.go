package main

import (
	"slices"
	"testing"

	"github.com/adanalife/tripbot/pkg/background"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// leaderboardJobs are the two jobs that put miles/guess leaderboards on screen:
// the periodic overlay rotation, and the rebuild that keeps the lifetime board's
// cached snapshot current. Both depend on viewers earning miles and guessing,
// which only happens where chatters have a persisted identity.
var leaderboardJobs = []string{"chatbot.ShowRotatingLeaderboard", "users.UpdateLeaderboard"}

// scheduledJobNames registers the background jobs a platform gets and returns
// their names. Nothing is started, so the nil deps on the Tripbot are never
// dereferenced — a method value on a nil pointer receiver is only a panic when
// called, and every job here is either such a value or a closure.
func scheduledJobNames(t *testing.T, platform string) []string {
	t.Helper()

	sched, err := background.New()
	if err != nil {
		t.Fatalf("background.New(): %v", err)
	}
	t.Cleanup(func() {
		if err := sched.Stop(); err != nil {
			t.Errorf("scheduler Stop(): %v", err)
		}
	})

	// No YouTubeAPIURL / FacebookAPIURL, so the broadcast-discovery branches
	// stay out and no gateway client is constructed.
	tb := &Tripbot{cfg: &c.TripbotConfig{Platform: platform}, scheduler: sched}
	tb.scheduleBackgroundJobs()

	names := make([]string, 0)
	for _, j := range sched.Jobs() {
		names = append(names, j.Name())
	}
	return names
}

// TestScheduleBackgroundJobs_LeaderboardsAreTwitchOnly pins the gate that keeps
// leaderboards off the gateway platforms. On a bot-less YouTube stream nobody
// has a persisted identity, so the boards would either sit empty or — once any
// stray row existed — advertise a half-working feature on stream.
//
// The gate itself is an early return partway down scheduleBackgroundJobs, so it
// is easy to defeat by accident: adding a job below it is correct, adding one
// above it silently schedules that job everywhere. This test is what fails when
// that happens. The Twitch case is the positive control — without it the
// assertions would also pass if the job names changed or registration broke
// outright.
func TestScheduleBackgroundJobs_LeaderboardsAreTwitchOnly(t *testing.T) {
	for _, platform := range []string{"youtube", "facebook", "instagram", "tiktok"} {
		t.Run(platform, func(t *testing.T) {
			names := scheduledJobNames(t, platform)
			for _, job := range leaderboardJobs {
				if slices.Contains(names, job) {
					t.Errorf("%s must not schedule %q; got %v", platform, job, names)
				}
			}
			// Registration really happened — the platform-neutral jobs are
			// above the gate, so an empty result would mean this test proved
			// nothing.
			if !slices.Contains(names, "video.GetCurrentlyPlaying") {
				t.Fatalf("%s scheduled no platform-neutral jobs; got %v", platform, names)
			}
		})
	}

	t.Run("twitch", func(t *testing.T) {
		names := scheduledJobNames(t, "twitch")
		for _, job := range leaderboardJobs {
			if !slices.Contains(names, job) {
				t.Errorf("twitch must schedule %q; got %v", job, names)
			}
		}
	})

	// An unset STREAM_PLATFORM is Twitch (platformIsTwitch treats "" as Twitch),
	// so the gate must not read it as a gateway platform and drop the boards.
	t.Run("unset platform is twitch", func(t *testing.T) {
		names := scheduledJobNames(t, "")
		for _, job := range leaderboardJobs {
			if !slices.Contains(names, job) {
				t.Errorf("unset platform must schedule %q; got %v", job, names)
			}
		}
	})
}
