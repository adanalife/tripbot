package rotator

import (
	"reflect"
	"strings"
	"testing"
)

// FuzzSanitize asserts Sanitize is idempotent: whatever it accepts, it accepts
// again unchanged. The console saves a config, reads it back, and re-submits it
// on the next edit, so a normalization that keeps changing its own output would
// churn the stored copy on every round trip.
//
// The other invariants are the ones the doc comment promises about accepted
// output: text is trimmed, blank lines are gone, weights are within [0,
// MaxWeight], per-message Platforms scoping is cleared, and no pool is over
// MaxMessagesPerPool.
func FuzzSanitize(f *testing.F) {
	seeds := []struct {
		left, right, rare string
		weight            int
	}{
		{"Join us on `!discord`", "Where are we? (`!location`)", DefaultRareMessage, 2},
		{"", "", "", 0},
		{"  padded  ", "\t\n", " ", -1},
		{"$location", "$nothing", "$date", MaxWeight},
		{"\u202eRTL", "\x00\x01\x02", "\u200b", MaxWeight + 1},
		{strings.Repeat("x", 500), strings.Repeat("é", 200), strings.Repeat("$location", 40), 1},
	}
	for _, s := range seeds {
		f.Add(s.left, s.right, s.rare, s.weight)
	}
	f.Fuzz(func(t *testing.T, left, right, rare string, weight int) {
		// Two lines per pool: enough for the per-index error messages and the
		// blank-line drop to be exercised without an unbounded input.
		cfg := Config{
			Left: Corner{
				Messages:      []Message{{Text: left, Weight: weight}, {Text: right}},
				PromoMessages: []Message{{Text: right, Weight: weight, Platforms: []string{PlatformTwitch}}},
			},
			Right: Corner{
				Messages:      []Message{{Text: right, Weight: weight}},
				PromoMessages: []Message{{Text: left, Platforms: []string{PlatformTwitch}}, {Text: ""}},
			},
			RareMessage: rare,
		}

		got, err := Sanitize(cfg)
		if err != nil {
			if !reflect.DeepEqual(got, Config{}) {
				t.Errorf("Sanitize returned %+v alongside error %v, want the zero Config", got, err)
			}
			return
		}

		if got.RareMessage != strings.TrimSpace(got.RareMessage) {
			t.Errorf("rare message %q is not trimmed", got.RareMessage)
		}
		for _, pool := range [][]Message{
			got.Left.Messages, got.Left.PromoMessages,
			got.Right.Messages, got.Right.PromoMessages,
		} {
			if len(pool) > MaxMessagesPerPool {
				t.Errorf("pool has %d messages, over the max of %d", len(pool), MaxMessagesPerPool)
			}
			for _, m := range pool {
				if m.Text == "" || m.Text != strings.TrimSpace(m.Text) {
					t.Errorf("message text %q is blank or untrimmed", m.Text)
				}
				if m.Weight < 0 || m.Weight > MaxWeight {
					t.Errorf("weight %d is outside [0, %d]", m.Weight, MaxWeight)
				}
				if m.Platforms != nil {
					t.Errorf("message %q kept Platforms %v, want them cleared", m.Text, m.Platforms)
				}
			}
		}

		again, err := Sanitize(got)
		if err != nil {
			t.Fatalf("Sanitize rejected its own output %+v: %v", got, err)
		}
		if !reflect.DeepEqual(got, again) {
			t.Errorf("Sanitize is not idempotent:\n first: %+v\nsecond: %+v", got, again)
		}
	})
}
