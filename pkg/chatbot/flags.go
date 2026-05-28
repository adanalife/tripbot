package chatbot

import (
	"context"
	"sync"

	"github.com/adanalife/tripbot/pkg/feature"
)

// realFlags is the production FlagClient adapter the App is wired with at
// package init. Delegates to flagClient, which cmd/tripbot replaces with a
// Postgres-backed client once the DB connection is up via SetFlagClient.
// Mirrors the realIRC / realOnscreens shape.
type realFlags struct{}

func (realFlags) Bool(ctx context.Context, key string, evalCtx feature.EvalContext) bool {
	flagMu.RLock()
	c := flagClient
	flagMu.RUnlock()
	return c.Bool(ctx, key, evalCtx)
}

// flagClient is the FlagClient realFlags delegates to. Initialised to an
// empty in-memory client so every key evaluates to false during the brief
// startup window before cmd/tripbot swaps in the Postgres-backed client —
// matches the unknown-key contract from pkg/feature.
var (
	flagMu     sync.RWMutex
	flagClient feature.FlagClient = feature.NewInMemoryClient(nil)
)

// SetFlagClient installs the FlagClient that realFlags delegates to.
// Called from cmd/tripbot once the Postgres-backed client is constructed
// and its initial snapshot has loaded.
func SetFlagClient(c feature.FlagClient) {
	flagMu.Lock()
	flagClient = c
	flagMu.Unlock()
}
