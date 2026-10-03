// Package knownbots names the Twitch accounts a public registry lists as bots,
// so a view-bot lurking in the chatter list can be told apart from a person
// without someone running !makebot on it first. Bots inflate every count keyed
// off the humans in chat.
//
// The registry is twitchinsights' /bots/all: every account it has seen sitting
// in many channels at once, with the channel count and a last-seen time. Its
// /bots/online sibling is the same shape filtered to accounts in chat right
// now, which is often empty — /all is the list.
package knownbots

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// URL is the registry's full list.
const URL = "https://api.twitchinsights.net/v1/bots/all"

// client bounds a refresh: the body is ~250 KB, so a request still running
// after this is a stalled one.
var client = &http.Client{Timeout: 30 * time.Second}

// List is the set of names the registry last reported. The zero List names
// nobody, so a registry that has never answered classifies no one.
type List struct {
	url   string
	mu    sync.RWMutex
	names map[string]struct{}
}

// New returns an empty List that Refresh fills from url.
func New(url string) *List {
	return &List{url: url}
}

// IsKnownBot reports whether the registry lists username. Case-insensitive,
// since chatter lists and the registry both use logins but not always the
// same case.
func (l *List) IsKnownBot(username string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.names[strings.ToLower(username)]
	return ok
}

// Len is how many names the list holds.
func (l *List) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.names)
}

// Refresh replaces the list with the registry's current one. On any failure,
// or an answer naming nobody, the previous list stays: a registry outage must
// not quietly turn every bot back into a person.
func (l *List) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("knownbots: registry answered %s", resp.Status)
	}

	// Each entry is [name, channel count, last-seen unix time]; only the name
	// is read, so the other two stay raw.
	var body struct {
		Bots [][]json.RawMessage `json:"bots"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("knownbots: decode: %w", err)
	}
	names := make(map[string]struct{}, len(body.Bots))
	for _, entry := range body.Bots {
		var name string
		if len(entry) == 0 || json.Unmarshal(entry[0], &name) != nil || name == "" {
			continue
		}
		names[strings.ToLower(name)] = struct{}{}
	}
	if len(names) == 0 {
		return errors.New("knownbots: registry listed no bots, keeping the previous list")
	}

	l.mu.Lock()
	l.names = names
	l.mu.Unlock()
	return nil
}
