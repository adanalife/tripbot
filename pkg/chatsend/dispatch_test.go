package chatsend

import (
	"context"
	"errors"
	"testing"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
)

func TestDispatch(t *testing.T) {
	ctx := context.Background()

	t.Run("bot routes to botSay", func(t *testing.T) {
		var botGot string
		bcastCalled := false
		Dispatch(ctx,
			chatEvents.Send{Identity: chatEvents.IdentityBot, Text: "hi"},
			func(s string) { botGot = s },
			func(context.Context, string) error { bcastCalled = true; return nil },
		)
		if botGot != "hi" {
			t.Errorf("botSay got %q, want %q", botGot, "hi")
		}
		if bcastCalled {
			t.Error("broadcasterSay should not be called for the bot identity")
		}
	})

	t.Run("broadcaster routes to broadcasterSay", func(t *testing.T) {
		botCalled := false
		var bcastGot string
		Dispatch(ctx,
			chatEvents.Send{Identity: chatEvents.IdentityBroadcaster, Text: "yo"},
			func(string) { botCalled = true },
			func(_ context.Context, s string) error { bcastGot = s; return nil },
		)
		if bcastGot != "yo" {
			t.Errorf("broadcasterSay got %q, want %q", bcastGot, "yo")
		}
		if botCalled {
			t.Error("botSay should not be called for the broadcaster identity")
		}
	})

	t.Run("empty text is dropped", func(t *testing.T) {
		called := false
		Dispatch(ctx,
			chatEvents.Send{Identity: chatEvents.IdentityBot, Text: ""},
			func(string) { called = true },
			func(context.Context, string) error { called = true; return nil },
		)
		if called {
			t.Error("no sender should be called for empty text")
		}
	})

	t.Run("unknown identity is dropped", func(t *testing.T) {
		called := false
		Dispatch(ctx,
			chatEvents.Send{Identity: "stranger", Text: "hi"},
			func(string) { called = true },
			func(context.Context, string) error { called = true; return nil },
		)
		if called {
			t.Error("no sender should be called for an unknown identity")
		}
	})

	t.Run("broadcaster send error does not panic", func(t *testing.T) {
		// The error path just logs; assert it routes and swallows the error.
		Dispatch(ctx,
			chatEvents.Send{Identity: chatEvents.IdentityBroadcaster, Text: "boom"},
			func(string) { t.Error("botSay should not be called") },
			func(context.Context, string) error { return errors.New("scope missing") },
		)
	})
}
