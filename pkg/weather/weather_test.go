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
