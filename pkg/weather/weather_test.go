package weather

import "testing"

func TestCodeText(t *testing.T) {
	cases := map[int]string{
		0:   "Clear sky",
		2:   "Partly cloudy",
		45:  "Foggy",
		63:  "Rain",
		75:  "Snow",
		95:  "Thunderstorm",
		999: "Unknown conditions",
	}
	for code, want := range cases {
		if got := codeText(code); got != want {
			t.Errorf("codeText(%d) = %q, want %q", code, got, want)
		}
	}
}

// Every phrase codeText can produce has an icon, except the one that says it
// knows nothing — a new code family added without one would read as bare text.
func TestIconCoversEveryPhrase(t *testing.T) {
	codes := []int{0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 99}
	for _, code := range codes {
		if desc := codeText(code) + ", 71°F"; Icon(desc) == "" {
			t.Errorf("Icon(%q) is empty for code %d", desc, code)
		}
	}
	if got := Icon("Unknown conditions, 71°F"); got != "" {
		t.Errorf("Icon(unknown) = %q, want empty", got)
	}
}
