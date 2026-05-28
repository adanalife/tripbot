package feature

import "context"

// InMemoryClient is a FlagClient backed by an in-process map. Useful for
// unit tests and for the fallback path before a real backing store is
// wired in. The map is set at construction and not mutated.
type InMemoryClient struct {
	flags map[string]Flag
}

// NewInMemoryClient builds a client serving the given snapshot. A nil map
// is fine — it just means every key evaluates to its default (false).
func NewInMemoryClient(flags map[string]Flag) *InMemoryClient {
	if flags == nil {
		flags = map[string]Flag{}
	}
	return &InMemoryClient{flags: flags}
}

// Bool evaluates the named flag against the given context.
func (c *InMemoryClient) Bool(_ context.Context, key string, evalCtx EvalContext) bool {
	f, ok := c.flags[key]
	if !ok {
		return false
	}
	return evaluate(f, evalCtx)
}
