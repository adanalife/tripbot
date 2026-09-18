// Package wikipedia answers "what is this place" for a town the stream is
// passing through, against the English Wikipedia REST summary endpoint.
//
// It returns the article's first sentence, which is the part that actually
// says what a place is ("Dillon is a home rule municipality in Summit County,
// Colorado") — the rest of a summary paragraph is history and demographics
// that don't fit in a chat message. Stdlib-only and side-effect free (no
// config, no DB, no init), like pkg/weather, so any caller can import it.
package wikipedia

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// summaryURL is the REST summary endpoint, one page per path segment. Free,
// keyless, and cached at the edge. A var, not a const, so tests can point it
// at an httptest server; nothing outside this package can reach it.
var summaryURL = "https://en.wikipedia.org/api/rest_v1/page/summary/"

// httpTimeout bounds the fetch so a hung response can't stall a chat handler.
const httpTimeout = 5 * time.Second

// userAgent identifies the caller, which Wikimedia's API policy requires and
// enforces: the default Go UA is rate-limited hard and can be refused outright.
const userAgent = "adanalife-tripbot/1.0 (https://github.com/adanalife/tripbot)"

// ErrNoArticle means Wikipedia has nothing usable under that title — no page,
// or a disambiguation stub that names other pages instead of describing a
// place. Distinct from a transport failure so a caller can tell "nothing to
// say about this town" from "the lookup broke".
var ErrNoArticle = fmt.Errorf("no article")

// REST queries the Wikipedia REST API over HTTP. The zero value is ready to
// use; there's nothing to configure.
type REST struct{}

// Summary returns the first sentence of the article at `title`, e.g.
// "Dillon, Colorado". Titles are matched exactly — US place articles are
// titled "<City>, <State>", which is what makes a caller's town/state pair
// enough to ask with.
func (REST) Summary(ctx context.Context, title string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	// Wikipedia titles carry spaces as underscores, and the rest of the title
	// still has to survive as one path segment ("Coeur d'Alene, Idaho").
	path := url.PathEscape(strings.ReplaceAll(title, " ", "_"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, summaryURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNoArticle
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("wikipedia summary: status %d", resp.StatusCode)
	}

	var body struct {
		Type    string `json:"type"`
		Extract string `json:"extract"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	// A disambiguation page decodes fine and its extract reads like prose
	// ("Dillon may refer to:"), so it has to be refused by type or it ships
	// to chat as an answer.
	if body.Type == "disambiguation" || strings.TrimSpace(body.Extract) == "" {
		return "", ErrNoArticle
	}
	return firstSentence(strings.TrimSpace(body.Extract)), nil
}

// abbrevs are the words whose trailing period does not end a sentence. Drawn
// from what US place summaries are full of: "U.S. Route 50", "St. Louis",
// "Mt. Shasta", "Washington Co." — each one would otherwise cut the sentence
// a few words in.
var abbrevs = []string{"U.S", "St", "Ste", "Mt", "Ft", "Rte", "No", "Jr", "Sr", "Co", "Inc", "Dr", "approx", "est"}

// firstSentence returns everything through the first sentence-ending period,
// or the whole text when it has none.
func firstSentence(text string) string {
	for i, r := range text {
		if r != '.' {
			continue
		}
		// A period with no space after it is inside something — a decimal
		// ("3.5 km"), an initialism ("U.S.C."), or the end of the text, which
		// the return below already covers.
		if rest := text[i+1:]; rest != "" && !strings.HasPrefix(rest, " ") {
			continue
		}
		if isAbbrev(text[:i]) {
			continue
		}
		return text[:i+1]
	}
	return text
}

// isAbbrev reports whether the word ending where a period was found keeps the
// sentence going: a single initial ("J. R. Smith") or one of the abbreviations
// above.
func isAbbrev(before string) bool {
	word := before[strings.LastIndexAny(before, " (\"")+1:]
	return len([]rune(word)) == 1 || slices.Contains(abbrevs, word)
}
