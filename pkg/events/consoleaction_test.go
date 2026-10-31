package events

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

// A console_action row is a system event whose meta names the action, its
// target, optional free-text detail, and the optional caller identity. The exact JSON matters — audit
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
		ConsoleActionMeta{Action: "scale", Target: "obs-tiktok", Detail: "replicas 0→1"}); err != nil {
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
		ConsoleActionMeta{Action: "obs_refresh", Target: "obs-twitch"}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// The reported caller lands in meta as its own principal/tier fields, so an
// audit query can address it as meta->>'principal' rather than by scanning
// the free-text detail.
func TestConsoleActionRow_Principal(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"", "twitch", "console_action",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, nil,
			`{"action":"scale","target":"obs-tiktok","detail":"replicas 0→1","principal":"dana-iphone","tier":"owner"}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := ConsoleAction(context.Background(), &c.TripbotConfig{Platform: "twitch"},
		ConsoleActionMeta{
			Action: "scale", Target: "obs-tiktok", Detail: "replicas 0→1",
			Principal: "dana-iphone", Tier: "owner",
		}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
