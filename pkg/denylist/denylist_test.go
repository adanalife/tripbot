package denylist

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	l := New([]string{"Maple Tree", "maple tree lane", "Smithson", " ", "ab"})
	for _, tc := range []struct{ name, in, want string }{
		{"plain text untouched", "thanks for the follow!", "thanks for the follow!"},
		{"case-insensitive", "hello SMITHSON", "hello ***"},
		{"inside a username", "welcome xX_smithson_Xx", "welcome xX_***_Xx"},
		{"words joined", "MapleTree", "***"},
		{"words separated by punctuation", "at 12 maple-tree, ok", "at 12 ***, ok"},
		{"longer term masked whole", "Maple Tree Lane", "***"},
		{"every occurrence", "smithson and Smithson", "*** and ***"},
		{"too-short term ignored", "about a cab", "about a cab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := l.Redact(tc.in); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNewWithNothingUsableIsNil(t *testing.T) {
	for _, terms := range [][]string{nil, {""}, {"  ", "a", "--"}} {
		if l := New(terms); l != nil {
			t.Errorf("New(%q) = %v, want nil", terms, l)
		}
	}
	var l *List
	if got := l.Redact("anything"); got != "anything" {
		t.Errorf("nil List redacted to %q", got)
	}
}

// A term is data, never a pattern: regexp metacharacters in one must not
// change what matches.
func TestTermIsNotARegexp(t *testing.T) {
	l := New([]string{"a.c+d"})
	if got := l.Redact("abcd abccd"); got != "abcd abccd" {
		t.Errorf("metacharacters matched as a pattern: %q", got)
	}
	if got := l.Redact("a.c+d"); got != Mask {
		t.Errorf("literal term not redacted: %q", got)
	}
}

func FuzzRedact(f *testing.F) {
	f.Add("smithson", "hello Smithson")
	f.Add("maple tree", "MAPLE\ttree")
	f.Add("a.c+d", "(?i)[")
	f.Fuzz(func(t *testing.T, term, text string) {
		l := New([]string{term})
		out := l.Redact(text)
		if l == nil {
			if out != text {
				t.Fatalf("nil list changed %q to %q", text, out)
			}
			return
		}
		if out != text && !strings.Contains(out, Mask) {
			t.Fatalf("%q changed to %q without a mask", text, out)
		}
	})
}
