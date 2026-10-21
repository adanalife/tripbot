package bedprefs

import (
	"testing"

	"github.com/adanalife/tripbot/pkg/database/testdb"
)

func TestGetBeforeAnySaveIsNotFound(t *testing.T) {
	s := New(testdb.New(t))
	if _, _, found, err := s.Get(t.Context(), "twitch"); err != nil || found {
		t.Fatalf("Get = found %v, err %v; want not found", found, err)
	}
}

func TestSaveUpsertsPerPlatform(t *testing.T) {
	s := New(testdb.New(t))
	if err := s.Save(t.Context(), "twitch", "streambeats", true); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// The whole share is "", which must round-trip as a saved choice, not "nothing saved".
	if err := s.Save(t.Context(), "twitch", "", false); err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if err := s.Save(t.Context(), "youtube", "streambeats-lofi", true); err != nil {
		t.Fatalf("Save youtube: %v", err)
	}
	album, shuffle, found, err := s.Get(t.Context(), "twitch")
	if err != nil || !found || album != "" || shuffle {
		t.Errorf("twitch = %q shuffle %v found %v err %v; want \"\" false true nil", album, shuffle, found, err)
	}
	album, shuffle, found, err = s.Get(t.Context(), "youtube")
	if err != nil || !found || album != "streambeats-lofi" || !shuffle {
		t.Errorf("youtube = %q shuffle %v found %v err %v", album, shuffle, found, err)
	}
}
