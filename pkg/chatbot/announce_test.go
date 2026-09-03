package chatbot

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/video"
)

// captureSubscriberEvents swaps a recording publisher onto the event bus and
// returns an accessor for the subscriber events emitted through it — the
// console-facing half of the announce path, which leaves no other trace.
func captureSubscriberEvents(t *testing.T) func() []eventbus.SubscriberEvent {
	t.Helper()
	rec := &recordingNATS{}
	saved := eventbus.Default
	eventbus.SetPublisher(rec)
	t.Cleanup(func() { eventbus.Default = saved })
	return func() []eventbus.SubscriberEvent {
		var out []eventbus.SubscriberEvent
		want := eventbus.SubscriberEventSubject(testConf.Environment)
		for _, pub := range rec.Publishes {
			if pub.Subject != want {
				continue
			}
			var ev eventbus.SubscriberEvent
			if err := json.Unmarshal(pub.Payload, &ev); err != nil {
				t.Fatalf("unmarshal subscriber event: %v", err)
			}
			out = append(out, ev)
		}
		return out
	}
}

// subAnnounceApp returns a test App with recorders on the three surfaces a sub
// touches: chat, the events table, and the session state that hands out the
// bonus mile.
func subAnnounceApp(t *testing.T) (*App, *recordingChat, *recordingEvents) {
	t.Helper()
	app := newTestApp(video.Video{})
	app.Sessions = &recordingSessions{}
	chat := &recordingChat{}
	app.Chat = chat
	rec := &recordingEvents{}
	app.Events = rec
	return app, chat, rec
}

// A new follower lands one follow event naming them — the durable rows a
// "followers gained" recap counts, and the only place the EventSub notice
// survives past the chat shout.
func TestAnnounceNewFollower_RecordsFollowEvent(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec

	app.AnnounceNewFollower("viewer1")

	if len(rec.Follows) != 1 {
		t.Fatalf("follows = %d, want 1", len(rec.Follows))
	}
	if rec.Follows[0] != "viewer1" {
		t.Errorf("follow username = %q, want viewer1", rec.Follows[0])
	}
}

// TestAnnounceNewFollower_NudgesCommands asserts the follow thank-you points
// new followers at the discovery command — a follow is the first moment a
// viewer learns the bot is interactive. The eventbus emit rides along
// unpublished (no NATS conn in tests), so this stays focused on the chat copy.
func TestAnnounceNewFollower_NudgesCommands(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	app.AnnounceNewFollower("viewer1")

	if len(rec.Says) != 1 {
		t.Fatalf("expected exactly one Say() call, got %d: %v", len(rec.Says), rec.Says)
	}
	if !strings.Contains(rec.Says[0], "@viewer1") {
		t.Errorf("expected @username in follow thank-you, got %q", rec.Says[0])
	}
	if !strings.Contains(rec.Says[0], "!commands") {
		t.Errorf("expected a !commands nudge in follow thank-you, got %q", rec.Says[0])
	}
}

// --- RecordRaid ---

// An incoming raid records one raid event carrying the raiding channel and
// party size, stamped with the airing clip and playhead — the row a per-clip
// audience rollup uses to discount the spike. Nothing is said in chat: Twitch
// announces the raid natively.
func TestRecordRaid_RecordsRaidEventWithAiring(t *testing.T) {
	app := newTestApp(video.Video{ID: 77})
	rec := &recordingEvents{}
	app.Events = rec
	chat := &recordingChat{}
	app.Chat = chat

	app.RecordRaid("somechannel", 25)

	if len(rec.Raids) != 1 {
		t.Fatalf("raids = %d, want 1", len(rec.Raids))
	}
	got := rec.Raids[0]
	if got.From != "somechannel" || got.Viewers != 25 {
		t.Errorf("raid = %+v, want somechannel raiding with 25 viewers", got)
	}
	if got.VideoID != 77 {
		t.Errorf("video id = %d, want 77", got.VideoID)
	}
	if got.TsSec == nil {
		t.Error("ts_sec = nil, want the playhead stamped")
	}
	if len(chat.Says) != 0 {
		t.Errorf("chat says = %v, want none for a raid", chat.Says)
	}
}

// A raid arriving before the player has a current clip still records, with
// empty airing context — the raid happened whether or not the playhead is
// known.
func TestRecordRaid_NoPlayerStillRecords(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingEvents{}
	app.Events = rec
	app.Video = nil

	app.RecordRaid("somechannel", 25)

	if len(rec.Raids) != 1 {
		t.Fatalf("raids = %d, want 1", len(rec.Raids))
	}
	got := rec.Raids[0]
	if got.VideoID != 0 {
		t.Errorf("video id = %d, want 0 (writes NULL)", got.VideoID)
	}
	if got.TsSec != nil {
		t.Errorf("ts_sec = %v, want nil (writes NULL)", *got.TsSec)
	}
}

// --- AnnounceSubscriber ---

// A new sub gets thanked, opens a subscribed interval in the event log, and
// reaches the console tagged with its tier. The second chat line is the
// everyone-gets-a-mile announcement, which reports the live viewer count.
func TestAnnounceSubscriber_ThanksLogsAndReachesConsole(t *testing.T) {
	app, chat, rec := subAnnounceApp(t)
	events := captureSubscriberEvents(t)

	app.AnnounceSubscriber("viewer1", false, "1000")

	if len(chat.Says) != 2 {
		t.Fatalf("says = %d, want 2 (thank-you + bonus-mile announcement): %v", len(chat.Says), chat.Says)
	}
	if !strings.Contains(chat.Says[0], "@viewer1") {
		t.Errorf("thank-you = %q, want it to name @viewer1", chat.Says[0])
	}
	if !strings.Contains(chat.Says[1], "bonus mile") {
		t.Errorf("second line = %q, want the bonus-mile announcement", chat.Says[1])
	}
	if len(rec.Subs) != 1 || rec.Subs[0] != "viewer1" {
		t.Errorf("subscribe events = %v, want [viewer1]", rec.Subs)
	}
	got := events()
	if len(got) != 1 {
		t.Fatalf("subscriber events = %d, want 1", len(got))
	}
	if got[0].Kind != "sub" || got[0].Username != "viewer1" || got[0].Tier != "1000" {
		t.Errorf("event = %+v, want kind=sub username=viewer1 tier=1000", got[0])
	}
}

// A gift recipient's channel.subscribe still shouts and still opens their
// subscribed interval, but publishes nothing to the console — the gifter's own
// gift event is what the console celebrates, so a mass-gift shows one banner
// rather than one per recipient.
func TestAnnounceSubscriber_GiftRecipientSkipsConsoleEvent(t *testing.T) {
	app, chat, rec := subAnnounceApp(t)
	events := captureSubscriberEvents(t)

	app.AnnounceSubscriber("viewer1", true, "1000")

	if len(chat.Says) != 2 {
		t.Errorf("says = %d, want 2 — a gifted sub is still shouted: %v", len(chat.Says), chat.Says)
	}
	if len(rec.Subs) != 1 {
		t.Errorf("subscribe events = %v, want the interval opened for a gift recipient too", rec.Subs)
	}
	if got := events(); len(got) != 0 {
		t.Errorf("subscriber events = %+v, want none for a gift recipient", got)
	}
}

// The bonus-mile line reports how many viewers actually received the mile, so
// it reads honestly on a quiet stream rather than claiming a fixed audience.
func TestAnnounceSubscriber_BonusLineReportsLiveViewerCount(t *testing.T) {
	app, chat, _ := subAnnounceApp(t)

	app.AnnounceSubscriber("viewer1", false, "1000")

	if !strings.Contains(chat.Says[1], "0 current viewers") {
		t.Errorf("bonus line = %q, want the live logged-in count (0 here)", chat.Says[1])
	}
}

// --- AnnounceGiftSub ---

// A named gifter is thanked by name and reaches the console with the number of
// subs gifted, so the panel can size the celebration.
func TestAnnounceGiftSub_NamesGifterAndCount(t *testing.T) {
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat
	events := captureSubscriberEvents(t)

	app.AnnounceGiftSub("generousviewer", 5, "1000", false)

	if len(chat.Says) != 1 {
		t.Fatalf("says = %d, want 1: %v", len(chat.Says), chat.Says)
	}
	if !strings.Contains(chat.Says[0], "@generousviewer") || !strings.Contains(chat.Says[0], "5") {
		t.Errorf("thank-you = %q, want the gifter and the count", chat.Says[0])
	}
	got := events()
	if len(got) != 1 {
		t.Fatalf("subscriber events = %d, want 1", len(got))
	}
	if got[0].Kind != "gift" || got[0].Username != "generousviewer" || got[0].GiftCount != 5 || got[0].IsAnonymous {
		t.Errorf("event = %+v, want kind=gift username=generousviewer gift_count=5 not anonymous", got[0])
	}
}

// An anonymous gift must not leak an empty @ into chat — the thank-you says
// "an anonymous gifter" instead, and the console event carries the flag so the
// panel can render it the same way.
func TestAnnounceGiftSub_AnonymousGifterHasNoHandle(t *testing.T) {
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat
	events := captureSubscriberEvents(t)

	app.AnnounceGiftSub("", 1, "1000", true)

	if len(chat.Says) != 1 {
		t.Fatalf("says = %d, want 1: %v", len(chat.Says), chat.Says)
	}
	if strings.Contains(chat.Says[0], "@") {
		t.Errorf("thank-you = %q, want no @handle for an anonymous gifter", chat.Says[0])
	}
	if !strings.Contains(chat.Says[0], "anonymous") {
		t.Errorf("thank-you = %q, want it to say the gift was anonymous", chat.Says[0])
	}
	got := events()
	if len(got) != 1 {
		t.Fatalf("subscriber events = %d, want 1", len(got))
	}
	if !got[0].IsAnonymous || got[0].Username != "" {
		t.Errorf("event = %+v, want anonymous with no username", got[0])
	}
}

// --- AnnounceResub ---

// A resub is thanked with the viewer's cumulative months and reaches the
// console with the streak and the note they typed.
func TestAnnounceResub_ThanksWithMonthsAndCarriesStreak(t *testing.T) {
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat
	rec := &recordingEvents{}
	app.Events = rec
	events := captureSubscriberEvents(t)

	app.AnnounceResub("viewer1", 12, 6, "2000", "still here!")

	if len(chat.Says) != 1 {
		t.Fatalf("says = %d, want 1: %v", len(chat.Says), chat.Says)
	}
	if !strings.Contains(chat.Says[0], "12-month") || !strings.Contains(chat.Says[0], "@viewer1") {
		t.Errorf("thank-you = %q, want the month count and @viewer1", chat.Says[0])
	}
	got := events()
	if len(got) != 1 {
		t.Fatalf("subscriber events = %d, want 1", len(got))
	}
	if got[0].Kind != "resub" || got[0].Months != 12 || got[0].Streak != 6 || got[0].Message != "still here!" {
		t.Errorf("event = %+v, want kind=resub months=12 streak=6 with the message", got[0])
	}
	// A resub reuses an interval that is already open, so nothing new is logged.
	if len(rec.Subs) != 0 {
		t.Errorf("subscribe events = %v, want none — the viewer was already subscribed", rec.Subs)
	}
}

// A viewer who hides their month count gets a thank-you without one, rather
// than a "0-month resub".
func TestAnnounceResub_OmitsMonthsWhenHidden(t *testing.T) {
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat

	app.AnnounceResub("viewer1", 0, 0, "1000", "")

	if len(chat.Says) != 1 {
		t.Fatalf("says = %d, want 1: %v", len(chat.Says), chat.Says)
	}
	if strings.Contains(chat.Says[0], "0-month") {
		t.Errorf("thank-you = %q, want no month count when it is hidden", chat.Says[0])
	}
	if !strings.Contains(chat.Says[0], "@viewer1") {
		t.Errorf("thank-you = %q, want it to name @viewer1", chat.Says[0])
	}
}

// --- RecordUnsubscribe ---

// An unsub closes the viewer's subscribed interval and says nothing in chat —
// without the close, the interval would run forever and any subscriber-months
// rollup would over-count.
func TestRecordUnsubscribe_ClosesIntervalSilently(t *testing.T) {
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat
	rec := &recordingEvents{}
	app.Events = rec

	app.RecordUnsubscribe("viewer1", false, "1000")

	if len(rec.Unsubs) != 1 || rec.Unsubs[0] != "viewer1" {
		t.Errorf("unsubscribe events = %v, want [viewer1]", rec.Unsubs)
	}
	if len(chat.Says) != 0 {
		t.Errorf("chat says = %v, want none — unsubs are not announced", chat.Says)
	}
}

// failingEvents is an Events whose subscription writes all fail, so the
// announce path can be exercised with the durable log unavailable.
type failingEvents struct{ noopEvents }

func (failingEvents) Follow(_ context.Context, _ string) error      { return errors.New("db down") }
func (failingEvents) Subscribe(_ context.Context, _ string) error   { return errors.New("db down") }
func (failingEvents) Unsubscribe(_ context.Context, _ string) error { return errors.New("db down") }

// A failed event write must not cost the viewer their shout-out or their bonus
// mile: the durable log is best-effort here, and the announce path logs the
// error and carries on rather than aborting mid-announcement.
func TestAnnounce_EventWriteFailureStillAnnounces(t *testing.T) {
	for _, tc := range []struct {
		name     string
		announce func(*App)
		wantSays int
	}{
		{"follower", func(a *App) { a.AnnounceNewFollower("viewer1") }, 1},
		{"subscriber", func(a *App) { a.AnnounceSubscriber("viewer1", false, "1000") }, 2},
		{"unsubscribe", func(a *App) { a.RecordUnsubscribe("viewer1", false, "1000") }, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, chat, _ := subAnnounceApp(t)
			app.Events = failingEvents{}
			captureSubscriberEvents(t)

			tc.announce(app)

			if len(chat.Says) != tc.wantSays {
				t.Errorf("says = %d, want %d despite the failed event write: %v", len(chat.Says), tc.wantSays, chat.Says)
			}
		})
	}
}
