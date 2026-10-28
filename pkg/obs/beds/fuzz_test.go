package beds

import (
	"strings"
	"testing"
)

// FuzzParseStreamTitle feeds arbitrary ICY metadata blocks — bytes straight off
// a radio station's stream — and asserts the parser never panics and that a
// successful parse always names a title with no surrounding whitespace.
func FuzzParseStreamTitle(f *testing.F) {
	seeds := []string{
		"",
		"StreamTitle='Steve Cobby - Big Wow';",
		"StreamTitle='A - B - C';StreamUrl='';\x00\x00",
		"StreamTitle='Rock 'n' Roll - It's Fine';",
		"StreamTitle='unterminated\x00\x00",
		"StreamTitle=' - ';",
		"StreamTitle='';",
		"StreamUrl='x';",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, meta string) {
		artist, title, err := parseStreamTitle(meta)
		if err != nil {
			if artist != "" || title != "" {
				t.Fatalf("parseStreamTitle(%q) = %q, %q with error %v; want empty fields", meta, artist, title, err)
			}
			return
		}
		if title == "" {
			t.Fatalf("parseStreamTitle(%q) succeeded with an empty title", meta)
		}
		if strings.TrimSpace(artist) != artist || strings.TrimSpace(title) != title {
			t.Fatalf("parseStreamTitle(%q) = %q, %q; want both trimmed", meta, artist, title)
		}
	})
}
