package events

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// A console_action row is a system event whose meta names the action, its
// target, and optional free-text detail. The exact JSON matters — audit
// queries address it as meta->>'action' / meta->>'target'.
func TestConsoleActionRow(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"",               // username: system event, no actor
			"twitch",         // platform
			"console_action", // event
			sqlmock.AnyArg(), // session_id
			sqlmock.AnyArg(), // date_created
			nil,              // extra_miles_earned
			nil,              // video_id: no airing context
			nil,              // video_ts_sec
			`{"action":"scale","target":"obs-tiktok","detail":"replicas 0→1"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := ConsoleAction(context.Background(), &c.TripbotConfig{Platform: "twitch"},
		"scale", "obs-tiktok", "replicas 0→1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// An empty detail is omitted from the meta document rather than stored as "".
func TestConsoleActionRow_EmptyDetailOmitted(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"", "twitch", "console_action",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, nil,
			`{"action":"obs_refresh","target":"obs-twitch"}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := ConsoleAction(context.Background(), &c.TripbotConfig{Platform: "twitch"},
		"obs_refresh", "obs-twitch", ""); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
