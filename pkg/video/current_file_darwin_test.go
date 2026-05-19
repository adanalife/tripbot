package video

import (
	"strings"
	"testing"
)

func TestParseLsofForMP4(t *testing.T) {
	// Trimmed sample of `lsof -p <pid>` output for OBS with a dashcam
	// MP4 open. Column layout: COMMAND PID USER FD TYPE DEVICE SIZE/OFF NODE NAME.
	sample := strings.Join([]string{
		"COMMAND  PID USER   FD   TYPE DEVICE  SIZE/OFF     NODE NAME",
		"OBS    12345 dana  cwd    DIR    1,5       640 12345678 /Users/dana",
		"OBS    12345 dana  txt    REG    1,5  12345678 87654321 /Applications/OBS.app/Contents/MacOS/OBS",
		"OBS    12345 dana   42r   REG    1,5 123456789 22334455 /Users/dana/dashcam/2018_0514_224801_013.MP4",
		"OBS    12345 dana   43u  IPv4 0xabcd       0t0      TCP localhost:1234->localhost:5678",
	}, "\n")

	got, err := parseLsofForMP4(sample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2018_0514_224801_013.MP4"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseLsofForMP4_NoMatch(t *testing.T) {
	sample := strings.Join([]string{
		"COMMAND  PID USER   FD   TYPE DEVICE  SIZE/OFF     NODE NAME",
		"OBS    12345 dana  cwd    DIR    1,5       640 12345678 /Users/dana",
		"OBS    12345 dana  txt    REG    1,5  12345678 87654321 /Applications/OBS.app/Contents/MacOS/OBS",
	}, "\n")

	if _, err := parseLsofForMP4(sample); err == nil {
		t.Fatal("expected error when no MP4 line is present")
	}
}

func TestParseLsofForMP4_EmptyInput(t *testing.T) {
	if _, err := parseLsofForMP4(""); err == nil {
		t.Fatal("expected error for empty lsof output")
	}
}

func TestParseLsofForMP4_LowercaseExtension(t *testing.T) {
	sample := strings.Join([]string{
		"COMMAND  PID USER   FD   TYPE DEVICE  SIZE/OFF     NODE NAME",
		"OBS    12345 dana   42r   REG    1,5 123456789 22334455 /Users/dana/clip.mp4",
	}, "\n")

	got, err := parseLsofForMP4(sample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "clip.mp4" {
		t.Fatalf("got %q, want %q", got, "clip.mp4")
	}
}
