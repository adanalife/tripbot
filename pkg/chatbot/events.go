package chatbot

import (
	"context"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/events"
)

// Events records viewer-lifecycle events to the append-only events table —
// the durable history behind "when was this viewer subscribed" and "where did
// these miles come from". Tests inject a fake; production uses realEvents.
//
// The table is append-only by design, so every method here is a write that is
// never revised — nothing updates or deletes a row. Each returns
// its error rather than logging: a lost event is a hole in that history, and
// the caller is the one that knows whether it can say so in chat.
type Events interface {
	Subscribe(ctx context.Context, username string) error
	Unsubscribe(ctx context.Context, username string) error
	// Correction records a manual miles adjustment (delta may be negative),
	// which is what makes a hand-corrected total auditable afterwards.
	Correction(ctx context.Context, username string, delta float64) error
	// CommandRefused records a command the bot declined to run. Unlike the
	// viewer-lifecycle writers above, a refusal is the only record that the
	// attempt happened at all — nothing else in the system remembers a command
	// that didn't run.
	CommandRefused(ctx context.Context, r events.CommandRefusal) error
	// CommandRan records a command that dispatched and ran. Paired with
	// CommandRefused, every command attempt lands in exactly one of the two
	// kinds — the split any refusal rate or usage rollup is computed over.
	CommandRan(ctx context.Context, r events.CommandRun) error
}

// realEvents is the production Events adapter, holding only the config
// pkg/events needs; the DB handle is its.
type realEvents struct {
	cfg *c.TripbotConfig
}

func (r realEvents) Subscribe(ctx context.Context, username string) error {
	return events.Subscribe(ctx, r.cfg, username)
}

func (r realEvents) Unsubscribe(ctx context.Context, username string) error {
	return events.Unsubscribe(ctx, r.cfg, username)
}

func (r realEvents) Correction(ctx context.Context, username string, delta float64) error {
	return events.Correction(ctx, r.cfg, username, delta)
}

func (r realEvents) CommandRefused(ctx context.Context, ref events.CommandRefusal) error {
	return events.CommandRefused(ctx, r.cfg, ref)
}

func (r realEvents) CommandRan(ctx context.Context, run events.CommandRun) error {
	return events.CommandRan(ctx, r.cfg, run)
}
