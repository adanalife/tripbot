package onscreensServer

import (
	"html"
	"regexp"
	"strings"
)

// Inline-markdown markers recognised by renderInlineMarkdown. Deliberately a
// tiny subset — these overlays are single lines over video, not documents, so
// only the inline emphasis that reads at a glance is supported. html.EscapeString
// leaves these marker bytes (`, *) untouched, so matching the escaped string is
// safe.
var (
	// `code` — the motivating case: monospace !command references.
	codeSpanRe = regexp.MustCompile("`([^`]+)`")
	// **bold** — run before italicRe so the doubled markers aren't eaten as
	// two adjacent emphasis spans.
	boldRe = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	// *italic*
	italicRe = regexp.MustCompile(`\*([^*]+)\*`)
)

// renderInlineMarkdown converts a small, safe subset of inline Markdown to
// HTML for the text overlays:
//
//	`code`   -> <code>…</code>     (monospace; the motivating case is !command refs)
//	**bold** -> <strong>…</strong>
//	*italic* -> <em>…</em>
//
// The input is HTML-escaped first, so the only markup that reaches the browser
// is the handful of tags emitted here — a stray '<' in chat-sourced text can't
// inject DOM. Code spans win over emphasis: text inside backticks is taken
// literally (so `a*b*c` keeps its asterisks rather than italicising), because
// emphasis is only applied to the segments between code spans.
func renderInlineMarkdown(s string) string {
	s = html.EscapeString(s)

	var b strings.Builder
	last := 0
	for _, m := range codeSpanRe.FindAllStringSubmatchIndex(s, -1) {
		// m[0]:m[1] is the whole `…` match; m[2]:m[3] is the captured inner text.
		b.WriteString(applyEmphasis(s[last:m[0]]))
		b.WriteString("<code>")
		b.WriteString(s[m[2]:m[3]])
		b.WriteString("</code>")
		last = m[1]
	}
	b.WriteString(applyEmphasis(s[last:]))
	return b.String()
}

// applyEmphasis converts bold then italic markers in a code-free segment.
// Bold goes first so **x** isn't consumed as two empty italic spans.
func applyEmphasis(s string) string {
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	return s
}
