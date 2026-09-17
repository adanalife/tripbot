package main

import "testing"

func TestPrintVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"no args", []string{"tripbot"}, false},
		{"version flag", []string{"tripbot", "--version"}, true},
		{"other flag", []string{"tripbot", "--help"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := printVersion(tc.args); got != tc.want {
				t.Errorf("printVersion(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
