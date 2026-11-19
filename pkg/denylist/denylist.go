// Package denylist redacts operator-listed terms from text the bot is about to
// put in front of viewers — a chat line, an overlay. The list is for words that
// must never appear on a stream that broadcasts a GPS trail (a street name, a
// surname), whoever supplied them: a viewer's username, text a command echoes,
// or a reverse-geocoded address.
//
// The terms are private by nature, so nothing here logs a term or the text it
// was found in — only that a redaction happened.
package denylist

import (
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"unicode"
)

// Mask replaces every match.
const Mask = "***"

// minLetters is the shortest term taken. A one- or two-letter term would match
// inside most words and shred ordinary output.
const minLetters = 3

// List is a compiled set of terms. A nil List redacts nothing.
type List struct {
	re *regexp.Regexp
}

// New compiles terms into a List, or returns nil when none are usable.
//
// A term matches case-insensitively and anywhere in the text, inside a longer
// word included, so a term buried in a username is still caught — the cost is
// that a short term also masks innocent words that contain it. The words of a
// multi-word term match with any run of spaces or punctuation between them, or
// none: "Maple Tree" also catches "maple-tree" and "MapleTree".
//
// shortcut: spelled-out evasions (m.a.p.l.e, digits for letters) pass, upgrade
// if someone ever tries one.
func New(terms []string) *List {
	var pats []string
	for _, term := range terms {
		words := strings.FieldsFunc(term, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if len([]rune(strings.Join(words, ""))) < minLetters {
			if strings.TrimSpace(term) != "" {
				slog.Warn("denylist term shorter than the minimum ignored", "min_letters", minLetters)
			}
			continue
		}
		// words hold letters and digits only, so a term is never read as a pattern.
		pats = append(pats, strings.Join(words, `[^\pL\pN]*`))
	}
	if len(pats) == 0 {
		return nil
	}
	// Longest first: the regexp takes the first alternative that matches, and a
	// term that extends another should be masked whole.
	sort.SliceStable(pats, func(i, j int) bool { return len(pats[i]) > len(pats[j]) })
	return &List{re: regexp.MustCompile(`(?i)(?:` + strings.Join(pats, "|") + `)`)}
}

// Redact returns s with every listed term replaced by Mask.
func (l *List) Redact(s string) string {
	if l == nil || s == "" {
		return s
	}
	hits := 0
	out := l.re.ReplaceAllStringFunc(s, func(string) string {
		hits++
		return Mask
	})
	if hits > 0 {
		slog.Warn("denylisted term redacted from bot output", "hits", hits)
	}
	return out
}

var def atomic.Pointer[List]

// SetDefault installs the process-wide List the output surfaces redact with.
// main calls it once, before anything is sent.
func SetDefault(l *List) { def.Store(l) }

// Redact applies the process-wide List. With none installed, s comes back as is.
func Redact(s string) string { return def.Load().Redact(s) }
