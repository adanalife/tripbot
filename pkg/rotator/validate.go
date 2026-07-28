package rotator

import (
	"fmt"
	"strings"
)

// MaxMessagesPerPool bounds how many lines one pool can hold. Well past any
// plausible list (the shipped pools are 4-7 lines), low enough that a runaway
// client can't push an unbounded document into Postgres and onto the wire.
const MaxMessagesPerPool = 100

// MaxWeight caps a line's selection weight. A heavily weighted line is a
// legitimate choice — the live location line runs at 6 — but past this the pool
// is effectively one message, which is better expressed by deleting the others.
const MaxWeight = 100

// ValidationError reports copy the console shouldn't have been able to submit.
// The message names the offending line so the editor can point at it rather than
// just refusing the save.
type ValidationError struct {
	Side  Side
	Pool  string // "messages" or "promo_messages"
	Index int
	Text  string
	Msg   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s %s[%d] (%q): %s", e.Side, e.Pool, e.Index, e.Text, e.Msg)
}

// Sanitize normalizes a submitted config and rejects what can't be rendered.
//
// Normalizing (silent, because these are editor artifacts rather than mistakes
// worth an error): each line's text is trimmed, blank lines are dropped, a
// negative weight becomes 0 (which Weighted treats as 1), and Platforms scoping
// is cleared — stored copy is per-platform already, so the field has no meaning
// and keeping it would silently hide lines.
//
// Rejecting: a line past the corner's hard length limit, a weight past MaxWeight,
// or a pool past MaxMessagesPerPool. Length is checked against the corner the
// line actually renders in, which is the whole point of doing it here — the right
// corner's budget is 369px against the left's 564px, so the same line can pass on
// one side and fail on the other.
func Sanitize(cfg Config) (Config, error) {
	out := Config{RareMessage: strings.TrimSpace(cfg.RareMessage)}

	for _, c := range []struct {
		side Side
		in   Corner
		dst  *Corner
	}{
		{SideLeft, cfg.Left, &out.Left},
		{SideRight, cfg.Right, &out.Right},
	} {
		msgs, err := sanitizePool(c.side, "messages", c.in.Messages)
		if err != nil {
			return Config{}, err
		}
		promo, err := sanitizePool(c.side, "promo_messages", c.in.PromoMessages)
		if err != nil {
			return Config{}, err
		}
		c.dst.Messages = msgs
		c.dst.PromoMessages = promo
	}

	// The rare line renders in the left corner, so it answers to that budget.
	if b := BudgetFor(SideLeft); out.RareMessage != "" && b.TooLong(out.RareMessage) {
		return Config{}, &ValidationError{
			Side: SideLeft, Pool: "rare_message", Text: out.RareMessage,
			Msg: fmt.Sprintf("too long to render (over %d characters)", b.HardMaxRunes()),
		}
	}
	return out, nil
}

func sanitizePool(side Side, pool string, msgs []Message) ([]Message, error) {
	budget := BudgetFor(side)
	out := make([]Message, 0, len(msgs))
	for i, m := range msgs {
		m.Text = strings.TrimSpace(m.Text)
		if m.Text == "" {
			continue // an empty editor row is not an error, just nothing
		}
		if budget.TooLong(m.Text) {
			return nil, &ValidationError{
				Side: side, Pool: pool, Index: i, Text: m.Text,
				Msg: fmt.Sprintf("too long to render in the %s corner (over %d characters)",
					side, budget.HardMaxRunes()),
			}
		}
		if m.Weight > MaxWeight {
			return nil, &ValidationError{
				Side: side, Pool: pool, Index: i, Text: m.Text,
				Msg: fmt.Sprintf("weight %d is above the maximum of %d", m.Weight, MaxWeight),
			}
		}
		if m.Weight < 0 {
			m.Weight = 0
		}
		// Stored copy is per-platform, so per-message scoping has no meaning.
		m.Platforms = nil
		out = append(out, m)
	}
	if len(out) > MaxMessagesPerPool {
		return nil, &ValidationError{
			Side: side, Pool: pool,
			Msg: fmt.Sprintf("%d messages is above the maximum of %d", len(out), MaxMessagesPerPool),
		}
	}
	return out, nil
}
