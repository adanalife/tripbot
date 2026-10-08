package onscreensServer

import "testing"

func TestRenderInlineMarkdown(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "just text", "just text"},
		{"code span", "use `!find` to search", "use <code>!find</code> to search"},
		{"two code spans", "`!miles` and `!guess`", "<code>!miles</code> and <code>!guess</code>"},
		{"bold", "this is **important**", "this is <strong>important</strong>"},
		{"italic", "a *subtle* hint", "a <em>subtle</em> hint"},
		{"bold not eaten by italic", "**bold**", "<strong>bold</strong>"},
		// Code wins over emphasis: asterisks inside backticks stay literal.
		{"code wins over emphasis", "`a*b*c`", "<code>a*b*c</code>"},
		// Escaping: a stray angle bracket can't inject markup.
		{"escapes html", "1 < 2 & <b>x</b>", "1 &lt; 2 &amp; &lt;b&gt;x&lt;/b&gt;"},
		// Escaping happens inside code spans too.
		{"escapes inside code", "`<script>`", "<code>&lt;script&gt;</code>"},
		// An unterminated backtick is left literal (no code span).
		{"unterminated backtick", "oops `!find", "oops `!find"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderInlineMarkdown(tc.in); got != tc.want {
				t.Errorf("renderInlineMarkdown(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
