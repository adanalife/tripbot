package chatbot

import (
	"context"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/gateway"
	"github.com/adanalife/tripbot/pkg/video"
)

// fakeMetadata is an in-memory gateway metadata store that counts writes.
type fakeMetadata struct {
	stored gateway.Metadata
	writes int
}

func (f *fakeMetadata) CurrentMetadata(context.Context) (gateway.Metadata, error) {
	return f.stored, nil
}

func (f *fakeMetadata) SetMetadata(_ context.Context, m gateway.Metadata) error {
	f.stored = m
	f.writes++
	return nil
}

// bishopAfternoon is a clip filmed in Bishop, California at 15:00 PDT in July.
var bishopAfternoon = video.Video{
	Lat: 37.3635, Lng: -118.3951, State: "California",
	DateFilmed: time.Date(2018, 7, 10, 22, 0, 0, 0, time.UTC),
}

func streamTitleApp(vid video.Video, at video.Moment, tracked bool, on bool) (*App, *fakeMetadata) {
	a := newTestApp(vid)
	a.Video = &recordingVideo{Vid: vid, Moment: at, PlayheadTracked: tracked}
	a.Flags = &recordingFlags{Set: map[string]bool{streamTitleFlagKey: on}}
	md := &fakeMetadata{stored: gateway.Metadata{Title: "24/7 dashcam", Tags: []string{"dashcam"}, Category: "Travel & Outdoors"}}
	a.Metadata = md
	return a, md
}

func TestUpdateStreamTitle_WritesThePlaceKeepingTheRest(t *testing.T) {
	at := video.Moment{Lat: 37.3635, Lng: -118.3951, State: "California", City: "Bishop"}
	a, md := streamTitleApp(bishopAfternoon, at, true, true)

	a.UpdateStreamTitle(context.Background())

	if want := "Driving through Bishop, California in the afternoon"; md.stored.Title != want {
		t.Errorf("title = %q, want %q", md.stored.Title, want)
	}
	if md.stored.Category != "Travel & Outdoors" || len(md.stored.Tags) != 1 {
		t.Errorf("the write dropped the stored tags/category: %+v", md.stored)
	}

	// The same moment again changes nothing, so it writes nothing.
	a.UpdateStreamTitle(context.Background())
	if md.writes != 1 {
		t.Errorf("writes = %d, want 1 — an unchanged title must not be rewritten", md.writes)
	}
}

func TestUpdateStreamTitle_FlagOffLeavesTheOperatorsTitle(t *testing.T) {
	a, md := streamTitleApp(bishopAfternoon, video.Moment{}, false, false)

	a.UpdateStreamTitle(context.Background())

	if md.writes != 0 || md.stored.Title != "24/7 dashcam" {
		t.Errorf("flag off wrote %q (%d writes)", md.stored.Title, md.writes)
	}
}

func TestUpdateStreamTitle_FlaggedClipKeepsTheLastTitle(t *testing.T) {
	vid := bishopAfternoon
	vid.Flagged = true
	a, md := streamTitleApp(vid, video.Moment{}, false, true)

	a.UpdateStreamTitle(context.Background())

	if md.writes != 0 {
		t.Errorf("a clip with no usable GPS wrote %q", md.stored.Title)
	}
}

func TestUpdateStreamTitle_NoGatewayIsANoop(t *testing.T) {
	a, _ := streamTitleApp(bishopAfternoon, video.Moment{}, false, true)
	a.Metadata = nil
	a.UpdateStreamTitle(context.Background()) // must not panic
}

func TestStreamTitle_Wording(t *testing.T) {
	untimed := video.Video{State: "Utah"}
	tests := []struct {
		name string
		vid  video.Video
		at   video.Moment
		want string
	}{
		{"state only", untimed, video.Moment{}, "Driving through Utah"},
		{"inside a city", untimed, video.Moment{City: "Moab", State: "Utah"}, "Driving through Moab, Utah"},
		{"near a city", untimed, video.Moment{City: "Moab", CityM: 5000, State: "Utah"}, "Driving near Moab, Utah"},
		{"nothing resolved", video.Video{}, video.Moment{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := streamTitleApp(tc.vid, tc.at, false, true)
			if got := a.streamTitle(context.Background()); got != tc.want {
				t.Errorf("streamTitle = %q, want %q", got, tc.want)
			}
		})
	}
}
