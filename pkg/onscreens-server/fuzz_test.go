package onscreensServer

import (
	"strings"
	"testing"
)

// emittedTags is every tag renderInlineMarkdown is allowed to put in its
// output. Everything else it writes has been through html.EscapeString.
var emittedTags = []string{
	"<code>", "</code>",
	"<strong>", "</strong>",
	"<em>", "</em>",
}

// tagPairs are the emitted tags grouped so the balance check can compare each
// opener against its closer.
var tagPairs = [][2]string{
	{"<code>", "</code>"},
	{"<strong>", "</strong>"},
	{"<em>", "</em>"},
}

// FuzzRenderInlineMarkdown is the standing guard on the overlay renderer's
// escaping. Its input is operator- and chat-sourced text and its output goes
// into the overlay DOM as markup, so the property worth pinning isn't which
// tags come out for a given input — TestRenderInlineMarkdown covers that — it's
// that *no* angle bracket the caller supplied can ever survive as markup.
//
// Both invariants hold by construction today: html.EscapeString runs before any
// tag is inserted, and the regexes only ever substitute matched pairs. That's
// the point of fuzzing them rather than enumerating cases — the test fails the
// moment a future marker (a link syntax, an inline image) is added in a way
// that writes to the output without escaping, which is the one bug in this
// function that would matter.
func FuzzRenderInlineMarkdown(f *testing.F) {
	seeds := []string{
		// The example table's cases, so the corpus starts from known-good shapes.
		"", " ", "plain text",
		"use `!find` to search",
		"`!miles` and `!guess`",
		"this is **important**",
		"a *subtle* hint",
		"`a*b*c`",
		"1 < 2 & <b>x</b>",
		"`<script>`",
		"oops `!find",
		// Injection shapes: every one of these must come out inert.
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		"`<img src=x onerror=alert(1)>`",
		"**<script>**",
		"*<a href='javascript:x'>*",
		`"><script>x</script>`,
		"<!--", "-->", "<![CDATA[x]]>",
		// Marker pathologies — unterminated, doubled, interleaved.
		"*", "**", "***", "****", "*****",
		"`", "``", "```",
		"*a**b*", "**a*b**", "**a**b**",
		"`a`b`c`", "`a`b`",
		"*`a`*", "`*a*`", "**`a`**",
		// Non-ASCII, control bytes, and a bidi override.
		"\x00", "\U000e0000", "é*ü*ñ", "‮*rtl*",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		got := renderInlineMarkdown(s)

		// Strip the tags we mean to emit; any '<' or '>' left in the remainder
		// is markup we didn't intend, which is the entire XSS surface here.
		// Order is safe: none of the six tags is a substring of another.
		stripped := got
		for _, tag := range emittedTags {
			stripped = strings.ReplaceAll(stripped, tag, "")
		}
		if i := strings.IndexAny(stripped, "<>"); i >= 0 {
			t.Errorf("renderInlineMarkdown(%q) = %q\nleaked an unescaped angle bracket at offset %d of the tag-stripped output %q",
				s, got, i, stripped)
		}

		// An unbalanced count means a marker got consumed in a way that leaves
		// the overlay's DOM malformed.
		for _, pair := range tagPairs {
			openTag, closeTag := pair[0], pair[1]
			if opens, closes := strings.Count(got, openTag), strings.Count(got, closeTag); opens != closes {
				t.Errorf("renderInlineMarkdown(%q) = %q\n%s count %d != %s count %d",
					s, got, openTag, opens, closeTag, closes)
			}
		}
	})
}
