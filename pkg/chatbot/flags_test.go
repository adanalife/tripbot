package chatbot

import (
	"context"

	"github.com/adanalife/tripbot/pkg/feature"
)

// noopFlags satisfies feature.FlagClient for tests that don't care which
// flags get evaluated. Every key returns false — matches the unknown-key
// contract from pkg/feature, so a test that doesn't pre-set a flag sees
// the same behaviour the bot does on a fresh deploy.
type noopFlags struct{}

func (noopFlags) Bool(_ context.Context, _ string, _ feature.EvalContext) bool {
	return false
}

// recordingFlags captures every Bool() call so tests can assert on which
// flags a command evaluated. Set populates per-key return values; keys
// absent from Set evaluate to false.
type recordingFlags struct {
	Evals []string
	Set   map[string]bool
}

func (r *recordingFlags) Bool(_ context.Context, key string, _ feature.EvalContext) bool {
	r.Evals = append(r.Evals, key)
	return r.Set[key]
}
