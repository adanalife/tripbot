package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/video"
	"github.com/adanalife/tripbot/pkg/wikipedia"
)

// noopEncyclopedia is the default Encyclopedia in newTestApp: every lookup
// answers with a fixed sentence, so a test that doesn't care about !wiki never
// reaches the network.
type noopEncyclopedia struct{}

func (noopEncyclopedia) Summary(_ context.Context, _ string) (string, error) {
	return "A place in the United States.", nil
}

// recordingEncyclopedia captures the titles asked for and answers with a
// scripted result, so a test can assert both what was looked up and what chat
// heard about it.
type recordingEncyclopedia struct {
	Titles []string
	Result string
	Err    error
}

func (r *recordingEncyclopedia) Summary(_ context.Context, title string) (string, error) {
	r.Titles = append(r.Titles, title)
	return r.Result, r.Err
}

// townApp is a test App playing a clip whose coordinate the geocode pass has
// already named, cityM metres from the named place (0 = inside it).
func townApp(t *testing.T, city, state string, cityM float64) (*App, *recordingChat, *recordingEncyclopedia) {
	t.Helper()
	app := newTestApp(newTestVideo(state, 43.0, -108.0, time.Date(2018, 3, 7, 15, 0, 0, 0, time.UTC)))
	app.Video = &recordingVideo{
		Vid:             newTestVideo(state, 43.0, -108.0, time.Date(2018, 3, 7, 15, 0, 0, 0, time.UTC)),
		Moment:          video.Moment{Lat: 43.0, Lng: -108.0, State: state, City: city, CityM: cityM},
		PlayheadTracked: true,
	}
	chat := &recordingChat{}
	wiki := &recordingEncyclopedia{Result: "Dubois is a town in Fremont County, Wyoming."}
	app.Chat, app.Encyclopedia = chat, wiki
	return app, chat, wiki
}

// The title is the whole contract with Wikipedia: US place articles are titled
// "<City>, <State>", and asking with a bare city name lands on a
// disambiguation page for most of them.
func TestTownCmd_AsksForTheCityAndStateArticle(t *testing.T) {
	app, chat, wiki := townApp(t, "Dubois", "Wyoming", 0)

	app.townCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(wiki.Titles) != 1 || wiki.Titles[0] != "Dubois, Wyoming" {
		t.Fatalf("looked up %v, want one lookup of %q", wiki.Titles, "Dubois, Wyoming")
	}
	if len(chat.Says) != 1 || chat.Says[0] != "Dubois is a town in Fremont County, Wyoming." {
		t.Errorf("chat heard %v, want the summary verbatim", chat.Says)
	}
}

// The corpus is mostly interstate: 57% of moments sit outside every
// incorporated place. Naming the nearest one anyway would tell chat about a
// town 60 km away as if it were on screen, so the same distance bar !location
// applies before it will say "near X" has to gate the lookup too.
func TestTownCmd_SaysNothingIsNearbyBeyondTheDistanceBar(t *testing.T) {
	app, chat, wiki := townApp(t, "Dubois", "Wyoming", 60000)

	app.townCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(wiki.Titles) != 0 {
		t.Errorf("looked up %v, want no lookup at all", wiki.Titles)
	}
	if len(chat.Says) != 1 {
		t.Fatalf("chat heard %v, want exactly one message", chat.Says)
	}
	if !strings.Contains(chat.Says[0], "no town") {
		t.Errorf("message %q should say there's no town nearby", chat.Says[0])
	}
}

// A town Wikipedia has no page for and a broken lookup are different things to
// a viewer: one is an answer, the other is an apology. Collapsing them would
// have the bot apologize for every unincorporated place it passes.
func TestTownCmd_TellsNoArticleApartFromAFailedLookup(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"no article", wikipedia.ErrNoArticle, "nothing to say about Dubois, Wyoming"},
		{"lookup broke", errors.New("status 500"), "couldn't look up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, chat, wiki := townApp(t, "Dubois", "Wyoming", 0)
			wiki.Result, wiki.Err = "", tc.err

			app.townCmd(context.Background(), newTestUser("viewer1"), nil)

			if len(chat.Says) != 1 {
				t.Fatalf("chat heard %v, want exactly one message", chat.Says)
			}
			if !strings.Contains(chat.Says[0], tc.want) {
				t.Errorf("message %q should contain %q", chat.Says[0], tc.want)
			}
		})
	}
}

// With no per-moment place — the geocode pass hasn't reached the row, or the
// clip answers from its single fix — the clip's own town is the fallback, the
// same order !location reads in.
func TestTownCmd_FallsBackToTheClipsOwnTown(t *testing.T) {
	app, chat, wiki := townApp(t, "", "Wyoming", 0)
	cityM := 0.0
	vid := newTestVideo("Wyoming", 43.0, -108.0, time.Date(2018, 3, 7, 15, 0, 0, 0, time.UTC))
	vid.City, vid.CityM = "Dubois", &cityM
	app.Video = &recordingVideo{Vid: vid, Moment: video.Moment{Lat: 43.0, Lng: -108.0, State: "Wyoming"}, PlayheadTracked: true}

	app.townCmd(context.Background(), newTestUser("viewer1"), nil)

	if len(wiki.Titles) != 1 || wiki.Titles[0] != "Dubois, Wyoming" {
		t.Fatalf("looked up %v, want one lookup of %q", wiki.Titles, "Dubois, Wyoming")
	}
	if len(chat.Says) != 1 {
		t.Errorf("chat heard %v, want exactly one message", chat.Says)
	}
}
