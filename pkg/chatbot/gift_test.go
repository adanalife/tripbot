package chatbot

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
)

// The ladder must resolve every value, including one below the lowest rung —
// a gift is a gift, and a gift that silently did nothing would read as broken
// to the viewer who paid for it.
func TestEffectForResolvesEveryValue(t *testing.T) {
	for _, value := range []int{-1, 0, 1, 9, 10, 99, 100, 1_000_000} {
		if effectFor(value) == "" {
			t.Errorf("value %d resolved to no effect", value)
		}
	}
}

// effectFor takes the highest rung a gift reaches. Asserted against a local
// ladder rather than the production one so the property still holds after
// giftTiers is rebalanced (every rung is a timewarp today, which would make an
// assertion against the real table vacuous).
func TestEffectForTakesHighestRungReached(t *testing.T) {
	prev := giftTiers
	t.Cleanup(func() { giftTiers = prev })
	giftTiers = []giftTier{
		{MinValue: 0, Effect: "small"},
		{MinValue: 10, Effect: "medium"},
		{MinValue: 100, Effect: "large"},
	}

	cases := map[int]giftEffect{
		0: "small", 1: "small", 9: "small",
		10: "medium", 99: "medium",
		100: "large", 5000: "large",
	}
	for value, want := range cases {
		if got := effectFor(value); got != want {
			t.Errorf("effectFor(%d) = %q, want %q", value, got, want)
		}
	}
}

// The rungs must stay sorted ascending — effectFor walks the table in order and
// stops at the first rung above the gift's value, so an out-of-order rung would
// be unreachable.
func TestGiftTiersAreSortedAscending(t *testing.T) {
	if len(giftTiers) == 0 {
		t.Fatal("giftTiers is empty; effectFor indexes rung 0 unconditionally")
	}
	if giftTiers[0].MinValue != 0 {
		t.Errorf("first rung MinValue = %d, want 0 so every gift resolves", giftTiers[0].MinValue)
	}
	for i := 1; i < len(giftTiers); i++ {
		if giftTiers[i].MinValue <= giftTiers[i-1].MinValue {
			t.Errorf("rung %d (MinValue %d) does not exceed rung %d (MinValue %d)",
				i, giftTiers[i].MinValue, i-1, giftTiers[i-1].MinValue)
		}
	}
}

// newGiftTestApp wires the fakes a gift effect touches, with the gift flag on
// and the effect floor cleared so each test's gift is the first.
func newGiftTestApp(t *testing.T, search Search) (*App, *recordingPlayout) {
	t.Helper()
	enablePlayback(t)
	pinFindRandIntn(t, 0)
	prev := lastGiftEffectTime
	lastGiftEffectTime = time.Time{}
	t.Cleanup(func() { lastGiftEffectTime = prev })

	app, recPlayout, _ := newFindTestApp(t, search)
	app.Flags = &recordingFlags{Set: map[string]bool{giftFlagKey: true}}
	return app, recPlayout
}

var topRungGift = IncomingGift{User: "Gifter", UserID: "42", Name: "Galaxy", Count: 1, Value: 1000}

// A top-rung gift searches for what the gifter just said and jumps there.
func TestGiftFindUsesTheGiftersLastComment(t *testing.T) {
	search := &recordingSearch{Hits: []SearchHit{{Slug: "2018_0514_224801_013", TsSec: 163.5, Distance: 0.21}}}
	app, recPlayout := newGiftTestApp(t, search)

	app.HandleMessage(context.Background(), IncomingMessage{User: "Gifter", UserID: "42", Text: "an early comment"})
	app.HandleMessage(context.Background(), IncomingMessage{User: "Someone", UserID: "7", Text: "not the gifter"})
	app.HandleMessage(context.Background(), IncomingMessage{User: "Gifter", UserID: "42", Text: " a tunnel at sunset "})
	app.HandleMessage(context.Background(), IncomingMessage{User: "Gifter", UserID: "42", Text: "!skip"})
	app.HandleGatewayGift(context.Background(), topRungGift)

	if len(search.Queries) != 1 || search.Queries[0] != "a tunnel at sunset" {
		t.Fatalf("queries = %q, want the gifter's latest non-command comment", search.Queries)
	}
	want := `PlayFileAtTimestamp("2018_0514_224801_013.MP4", 151.5)`
	if len(recPlayout.Calls) != 1 || recPlayout.Calls[0] != want {
		t.Errorf("playout calls = %v, want %q", recPlayout.Calls, want)
	}

	// The comment is spent: a second gift has nothing to search for.
	lastGiftEffectTime = time.Time{}
	app.HandleGatewayGift(context.Background(), topRungGift)
	if len(search.Queries) != 1 {
		t.Errorf("queries after a second gift = %q, want the comment used once", search.Queries)
	}
}

// A gift always moves the stream: with nothing to search for, or nothing
// found, the top rung falls back to the timewarp.
func TestGiftFindFallsBackToTimewarp(t *testing.T) {
	hit := []SearchHit{{Slug: "x", TsSec: 30, Distance: 0.2}}
	tests := []struct {
		name    string
		search  *recordingSearch
		comment func(*App)
		queries int
	}{
		{"no comment", &recordingSearch{Hits: hit}, func(*App) {}, 0},
		{"comment too old", &recordingSearch{Hits: hit}, func(a *App) {
			a.recentComments = map[string]recentComment{"42": {text: "a tunnel", at: time.Now().Add(-giftFindWindow - time.Second)}}
		}, 0},
		{"nothing close enough", &recordingSearch{}, func(a *App) { a.noteComment("Gifter", "42", "unicorns") }, 1},
		{"search down", &recordingSearch{Err: errors.New("nats down")}, func(a *App) { a.noteComment("Gifter", "42", "a tunnel") }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, recPlayout := newGiftTestApp(t, tt.search)
			tt.comment(app)

			app.HandleGatewayGift(context.Background(), topRungGift)

			if len(tt.search.Queries) != tt.queries {
				t.Errorf("queries = %q, want %d", tt.search.Queries, tt.queries)
			}
			if len(recPlayout.Calls) != 1 || recPlayout.Calls[0] != "PlayRandom()" {
				t.Errorf("playout calls = %v, want the timewarp fallback", recPlayout.Calls)
			}
		})
	}
}

// A gift below the top rung never spends the gifter's comment on a search.
func TestSmallGiftStaysATimewarp(t *testing.T) {
	search := &recordingSearch{Hits: []SearchHit{{Slug: "x", TsSec: 30, Distance: 0.2}}}
	app, recPlayout := newGiftTestApp(t, search)
	app.noteComment("Gifter", "42", "a tunnel")

	app.HandleGatewayGift(context.Background(), IncomingGift{User: "Gifter", UserID: "42", Name: "Rose", Count: 1, Value: 1})

	if len(search.Queries) != 0 || len(recPlayout.Calls) != 1 || recPlayout.Calls[0] != "PlayRandom()" {
		t.Errorf("queries = %q, playout = %v; want a plain timewarp", search.Queries, recPlayout.Calls)
	}
}

// The comment memory sweeps out expired entries once it fills, so a busy chat
// can't grow it without bound.
func TestNoteCommentSweepsExpired(t *testing.T) {
	app := newTestApp(video.Video{})
	app.recentComments = make(map[string]recentComment)
	old := time.Now().Add(-giftFindWindow - time.Second)
	for i := range recentCommentsMax {
		app.recentComments[strconv.Itoa(i)] = recentComment{text: "stale", at: old}
	}

	app.noteComment("Fresh", "", "hello")

	if len(app.recentComments) != 1 {
		t.Fatalf("len = %d, want only the fresh comment left", len(app.recentComments))
	}
	if _, ok := app.takeComment("fresh", ""); !ok {
		t.Error("a name-keyed comment should be found whatever its casing")
	}
}
