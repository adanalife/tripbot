package chatbot

import (
	"context"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// directionApp wires a recordingVideo staging one heading reading, plus a
// recordingChat to read the answer back off.
func directionApp(t *testing.T, bearing float64, moving, tracked bool) (*App, *recordingChat) {
	t.Helper()
	app := newTestApp(video.Video{})
	chat := &recordingChat{}
	app.Chat = chat
	app.Video = &recordingVideo{Bearing: bearing, Moving: moving, HeadingTracked: tracked}
	return app, chat
}

func TestDirectionCmd_NamesTheHeading(t *testing.T) {
	app, chat := directionApp(t, 315, true, true)

	app.directionCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(chat.Says) != 1 {
		t.Fatalf("expected exactly one chat message, got %d: %v", len(chat.Says), chat.Says)
	}
	if !strings.Contains(chat.Says[0], "northwest") {
		t.Errorf("message %q does not name the heading", chat.Says[0])
	}
}

// A stopped van and an untrackable clip are different answers, and the one
// thing neither may do is invent a direction.
func TestDirectionCmd_StoppedAndUntrackedNameNoDirection(t *testing.T) {
	tests := []struct {
		name    string
		moving  bool
		tracked bool
	}{
		{"stopped", false, true},
		{"no track", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A bearing is staged deliberately: the command must ignore it
			// when moving or ok is false rather than read it anyway.
			app, chat := directionApp(t, 90, tc.moving, tc.tracked)

			app.directionCmd(context.Background(), newTestUser("viewer1"), nil)

			if len(chat.Says) != 1 {
				t.Fatalf("expected exactly one chat message, got %d: %v", len(chat.Says), chat.Says)
			}
			for _, point := range []string{"north", "east", "south", "west"} {
				if strings.Contains(chat.Says[0], point) {
					t.Errorf("message %q names a direction it does not have", chat.Says[0])
				}
			}
		})
	}
}

func TestSpeedCmd_NamesSpeedAndHeading(t *testing.T) {
	app, chat := directionApp(t, 90, true, true)
	app.Video.(*recordingVideo).SpeedMPS = 27.5 // 61.5 mph, 99 km/h

	app.speedCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(chat.Says) != 1 {
		t.Fatalf("expected exactly one chat message, got %d: %v", len(chat.Says), chat.Says)
	}
	for _, want := range []string{"62 mph", "99 km/h", "east"} {
		if !strings.Contains(chat.Says[0], want) {
			t.Errorf("message %q lacks %q", chat.Says[0], want)
		}
	}
}

func TestSpeedCmd_StoppedAndUntrackedNameNoSpeed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		moving  bool
		tracked bool
	}{
		{"stopped", false, true},
		{"no track", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, chat := directionApp(t, 90, tc.moving, tc.tracked)
			app.Video.(*recordingVideo).SpeedMPS = 30

			app.speedCmd(context.Background(), newTestUser("viewer1"), nil)

			if len(chat.Says) != 1 {
				t.Fatalf("expected exactly one chat message, got %d: %v", len(chat.Says), chat.Says)
			}
			if strings.Contains(chat.Says[0], "mph") {
				t.Errorf("message %q quotes a speed it does not have", chat.Says[0])
			}
		})
	}
}

// !speed is for the broadcaster and mods while the derived speed is being
// checked against the corpus; the registry must say so.
func TestSpeedCmd_IsModOnly(t *testing.T) {
	app := newTestApp(video.Video{})
	cmd, ok := app.singleWordLookup["!speed"]
	if !ok {
		t.Fatal("!speed does not resolve to a command")
	}
	if !cmd.RequiresMod {
		t.Error("!speed is not mod-gated")
	}
}

func TestDirectionCmd_IsRegistered(t *testing.T) {
	app := newTestApp(video.Video{})
	for _, trigger := range []string{"!direction", "!heading", "!compass", "!bearing"} {
		if _, ok := app.singleWordLookup[trigger]; !ok {
			t.Errorf("%s does not resolve to a command", trigger)
		}
	}
}
