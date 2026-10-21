package beds

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadStations(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok := write("ok.json", `[{"id":"a","name":"A","url":"https://radio.example/a"},{"id":"b","url":"http://radio.example/b"}]`)
	got, err := readStations(ok)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].Name != "b" {
		t.Errorf("got %+v; want both stations in file order, a nameless one named by id", got)
	}

	for name, body := range map[string]string{
		"empty.json":   `[]`,
		"blank.json":   ``,
		"invalid.json": `{"id":"a"}`,
		"dupe.json":    `[{"id":"a","url":"https://x.example/a"},{"id":"a","url":"https://x.example/b"}]`,
		"noid.json":    `[{"url":"https://x.example/a"}]`,
		"badurl.json":  `[{"id":"a","url":"ftp://x.example/a"}]`,
	} {
		if _, err := readStations(write(name, body)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := readStations(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("missing file: want an error")
	}
}

// With no station list the radio bed refuses cleanly and the other beds still
// switch — a missing mount must not take the bed store down with it.
func TestNoStations_RadioRefusesOtherBedsWork(t *testing.T) {
	Stations = nil
	t.Cleanup(func() { Stations = testStations })
	LoadStations(filepath.Join(t.TempDir(), "missing.json"))

	o := &fakeOBS{}
	s := NewStore(o, CarHum, t.TempDir(), "twitch")
	if err := s.Set(context.Background(), Radio); !errors.Is(err, errNoStation) {
		t.Fatalf("Set(radio) err = %v; want errNoStation", err)
	}
	if o.network {
		t.Fatal("a refused radio switch reached OBS")
	}
	if err := s.Set(context.Background(), CarHum); err != nil {
		t.Fatalf("Set(carhum) with no stations: %v", err)
	}
	if err := s.SwapToRadio(context.Background()); !errors.Is(err, errNoStation) {
		t.Fatalf("SwapToRadio err = %v; want errNoStation", err)
	}
}
