package main

import (
	"slices"
	"strings"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/go-co-op/gocron/v2"
)

// Every background job falls into one of the groups below, and together they are
// the whole of scheduleBackgroundJobs. That completeness is what lets these tests
// assert the *exact* set a platform schedules rather than spot-checking a few
// names: a job added to the function without being added here fails the table,
// which is the point. The gates are early returns and conditionals partway down a
// ~90-line function, so adding a job in the wrong place is a one-line mistake
// with no other symptom than a feature quietly running where it can't work.

// neutralJobs run on every instance regardless of platform: each one plays
// video, posts the periodic chatter, and republishes the rotators' clip feed.
var neutralJobs = []string{
	"video.GetCurrentlyPlaying",
	"chatbot.Chatter",
	"video.LocationFeed",
}

// twitchOnlyJobs each need something only Twitch has. Session and presence
// tracking read Twitch chatters; the leaderboards need viewers who earn miles
// and guess, which needs a chatter with a persisted identity (the gateway
// platforms hand the command path a transient user); the subscriber and follower
// polls hit Helix; the token refresh dereferences the IRC client a non-Twitch
// instance never constructs; and only the Twitch instance holds tokens to report
// an auth status for (gateway-youtube owns the youtube oauth_tokens row).
var twitchOnlyJobs = []string{
	"users.UpdateSession",
	"users.UpdateLeaderboard",
	"rollups.Reconcile",
	"chatbot.ShowRotatingLeaderboard",
	"users.PrintCurrentSession",
	"twitch.GetSubscribers",
	"twitch.GetFollowerCount",
	"twitch.ReloadTokens",
	"twitch.EmitAuthStatus",
}

// The broadcast-discovery jobs are doubly gated: off Twitch, and only when that
// gateway's URL is configured. Discovery is not the chat read, so it runs
// regardless of whether inbound chat is enabled.
const (
	youtubeDiscoveryJob  = "youtube.BroadcastDiscovery"
	facebookDiscoveryJob = "facebook.BroadcastDiscovery"
)

// leaderboardJobs are the two jobs that put miles/guess leaderboards on screen:
// the periodic overlay rotation, and the rebuild that keeps the lifetime board's
// cached snapshot current. Both are twitchOnlyJobs — named separately because
// keeping half-working leaderboards off the gateway streams is a specific
// product decision, not just a consequence of where the code sits.
var leaderboardJobs = []string{"chatbot.ShowRotatingLeaderboard", "users.UpdateLeaderboard"}

// scheduledJobNames registers the background jobs the given config gets and
// returns their names. Nothing is started, so the nil deps on the Tripbot are
// never dereferenced — a method value on a nil pointer receiver is only a panic
// when called, and every job is either such a value or a closure. gateway.New is
// pure construction, so the discovery branches dial nothing either.
func scheduledJobNames(t *testing.T, cfg *c.TripbotConfig) []string {
	t.Helper()

	sched, err := gocron.NewScheduler()
	if err != nil {
		t.Fatalf("gocron.NewScheduler(): %v", err)
	}
	t.Cleanup(func() {
		if err := sched.Shutdown(); err != nil {
			t.Errorf("scheduler Shutdown(): %v", err)
		}
	})

	tb := &Tripbot{cfg: cfg, scheduler: sched}
	tb.scheduleBackgroundJobs()

	names := make([]string, 0, len(sched.Jobs()))
	for _, j := range sched.Jobs() {
		names = append(names, j.Name())
	}
	return names
}

// assertSameJobs compares the scheduled set against want, reporting what leaked
// in and what went missing. Order is irrelevant — gocron does not promise any.
func assertSameJobs(t *testing.T, got, want []string) {
	t.Helper()

	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if slices.Equal(g, w) {
		return
	}
	var extra, missing []string
	for _, name := range g {
		if !slices.Contains(w, name) {
			extra = append(extra, name)
		}
	}
	for _, name := range w {
		if !slices.Contains(g, name) {
			missing = append(missing, name)
		}
	}
	if len(extra) > 0 {
		t.Errorf("scheduled jobs that should not run here: %s", strings.Join(extra, ", "))
	}
	if len(missing) > 0 {
		t.Errorf("expected jobs that were not scheduled: %s", strings.Join(missing, ", "))
	}
}

// TestScheduleBackgroundJobs_PlatformGates pins which jobs each platform gets.
// Asserting the exact set (rather than "these are absent") means it catches a job
// leaking onto a platform it can't work on *and* a job silently disappearing from
// the platform that needs it — the second is what a bare absence check misses.
//
// Adding a job to scheduleBackgroundJobs will fail this test until the new name
// is added to the right group above. That is deliberate friction: choosing the
// group is the same decision as choosing where in the function the job goes, and
// this is the only place that decision gets checked.
func TestScheduleBackgroundJobs_PlatformGates(t *testing.T) {
	tests := []struct {
		name string
		cfg  *c.TripbotConfig
		want []string
	}{
		{
			name: "twitch",
			cfg:  &c.TripbotConfig{Platform: "twitch"},
			want: append(slices.Clone(neutralJobs), twitchOnlyJobs...),
		},
		{
			// platformIsTwitch treats an unset STREAM_PLATFORM as Twitch, so the
			// gates must not read it as a gateway platform and drop the Twitch work.
			name: "unset platform is twitch",
			cfg:  &c.TripbotConfig{},
			want: append(slices.Clone(neutralJobs), twitchOnlyJobs...),
		},
		{
			// The discovery gates are "not Twitch AND url set", so a Twitch
			// instance that happens to have gateway URLs configured still gets
			// neither discovery job.
			name: "twitch ignores configured gateway urls",
			cfg: &c.TripbotConfig{
				Platform:       "twitch",
				YouTubeAPIURL:  "http://gateway-youtube:8080",
				FacebookAPIURL: "http://gateway-facebook:8080",
			},
			want: append(slices.Clone(neutralJobs), twitchOnlyJobs...),
		},
		{
			name: "youtube without a gateway url",
			cfg:  &c.TripbotConfig{Platform: "youtube"},
			want: neutralJobs,
		},
		{
			name: "youtube with a gateway url",
			cfg:  &c.TripbotConfig{Platform: "youtube", YouTubeAPIURL: "http://gateway-youtube:8080"},
			want: append(slices.Clone(neutralJobs), youtubeDiscoveryJob),
		},
		{
			name: "facebook without a gateway url",
			cfg:  &c.TripbotConfig{Platform: "facebook"},
			want: neutralJobs,
		},
		{
			name: "facebook with a gateway url",
			cfg:  &c.TripbotConfig{Platform: "facebook", FacebookAPIURL: "http://gateway-facebook:8080"},
			want: append(slices.Clone(neutralJobs), facebookDiscoveryJob),
		},
		{
			// Both discovery gates key off their URL rather than the platform
			// name, so a YouTube instance handed a Facebook URL polls Facebook
			// too. Pinned because it is surprising, not because it is wanted:
			// the manifests only set a platform its own gateway URL.
			name: "discovery follows the url, not the platform name",
			cfg: &c.TripbotConfig{
				Platform:       "youtube",
				YouTubeAPIURL:  "http://gateway-youtube:8080",
				FacebookAPIURL: "http://gateway-facebook:8080",
			},
			want: append(slices.Clone(neutralJobs), youtubeDiscoveryJob, facebookDiscoveryJob),
		},
		{
			name: "instagram",
			cfg:  &c.TripbotConfig{Platform: "instagram"},
			want: neutralJobs,
		},
		{
			name: "tiktok",
			cfg:  &c.TripbotConfig{Platform: "tiktok"},
			want: neutralJobs,
		},
		{
			// An unrecognised platform is not Twitch, so it gets the neutral set
			// and nothing else — the conservative default, matching how
			// platformCommandScope treats an undeclared platform.
			name: "unknown platform gets only the neutral set",
			cfg:  &c.TripbotConfig{Platform: "kick"},
			want: neutralJobs,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertSameJobs(t, scheduledJobNames(t, tc.cfg), tc.want)
		})
	}
}

// TestLeaderboardJobsAreTwitchOnly states the product invariant directly, so it
// survives any regrouping of the table above: the leaderboard jobs live in the
// Twitch-only group, and nothing outside that group can schedule them. On a
// bot-less stream nobody has a persisted identity, so the boards would either sit
// empty or — once any stray row existed — advertise a half-working feature.
func TestLeaderboardJobsAreTwitchOnly(t *testing.T) {
	for _, job := range leaderboardJobs {
		if !slices.Contains(twitchOnlyJobs, job) {
			t.Errorf("%q must be a Twitch-only job", job)
		}
		if slices.Contains(neutralJobs, job) {
			t.Errorf("%q must not be a platform-neutral job", job)
		}
	}

	for _, platform := range []string{"youtube", "facebook", "instagram", "tiktok"} {
		names := scheduledJobNames(t, &c.TripbotConfig{Platform: platform})
		for _, job := range leaderboardJobs {
			if slices.Contains(names, job) {
				t.Errorf("%s must not schedule %q; got %v", platform, job, names)
			}
		}
	}
}
