package events

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
)

// Deploy lives outside the shared writers list: its dedup SELECT precedes the
// INSERT, so the single-expectation stamp test can't drive it. These tests
// cover the same read-only and stamping contracts, plus the dedup itself.

// expectLastDeployVersion stages the dedup read. rows nil means "no prior
// deploy event".
func expectLastDeployVersion(mock sqlmock.Sqlmock, platform, component string, rows *sqlmock.Rows) {
	if rows == nil {
		rows = sqlmock.NewRows([]string{"version"})
	}
	mock.ExpectQuery(`SELECT meta->>'version' FROM "events"`).
		WithArgs(platform, "deploy", component, 1).
		WillReturnRows(rows)
}

// expectDeployInsert stages the INSERT a recorded deploy produces: a system
// event (empty username) whose meta names the component and version.
func expectDeployInsert(mock sqlmock.Sqlmock, platform, meta string) {
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"",               // username: system event, no actor
			platform,         // platform
			"deploy",         // event
			sqlmock.AnyArg(), // session_id
			sqlmock.AnyArg(), // date_created
			nil,              // extra_miles_earned
			nil,              // video_id: no airing context
			nil,              // video_ts_sec
			meta,             // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
}

// A read-only instance writes nothing — not even the dedup read runs (no mock
// DB is installed, so any DB touch would fail on the nil singleton).
func TestDeploy_ReadOnly(t *testing.T) {
	recorded, err := Deploy(context.Background(), &c.TripbotConfig{ReadOnly: true, Platform: "twitch"}, "tripbot", "v3.0.0")
	if !errors.Is(err, terrors.ErrReadOnly) {
		t.Fatalf("err = %v, want terrors.ErrReadOnly", err)
	}
	if recorded {
		t.Error("recorded = true, want false on a read-only instance")
	}
}

// A component with no prior deploy event records one.
func TestDeploy_FirstDeployRecords(t *testing.T) {
	mock := installMockDB(t)
	expectLastDeployVersion(mock, "twitch", "tripbot", nil)
	expectDeployInsert(mock, "twitch", `{"component":"tripbot","version":"v3.0.0"}`)

	recorded, err := Deploy(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "tripbot", "v3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !recorded {
		t.Error("recorded = false, want true for a first deploy")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A restart on the same build is not a deploy: the version matches the most
// recent deploy event for the component+platform, so no row is written.
func TestDeploy_SameVersionSkips(t *testing.T) {
	mock := installMockDB(t)
	expectLastDeployVersion(mock, "twitch", "tripbot",
		sqlmock.NewRows([]string{"version"}).AddRow("v3.0.0"))

	recorded, err := Deploy(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "tripbot", "v3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if recorded {
		t.Error("recorded = true, want false when the version is unchanged")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A version change is a deploy, however old the previous one.
func TestDeploy_NewVersionRecords(t *testing.T) {
	mock := installMockDB(t)
	expectLastDeployVersion(mock, "tiktok", "tripbot",
		sqlmock.NewRows([]string{"version"}).AddRow("v2.9.9"))
	expectDeployInsert(mock, "tiktok", `{"component":"tripbot","version":"v3.0.0"}`)

	recorded, err := Deploy(context.Background(), &c.TripbotConfig{Platform: "tiktok"}, "tripbot", "v3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !recorded {
		t.Error("recorded = false, want true for a new version")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// The dedup read failing must not lose the deploy row — record anyway. The
// duplicate is a nuisance; the hole in the timeline is unrecoverable.
func TestDeploy_DedupReadFailureStillRecords(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`SELECT meta->>'version' FROM "events"`).
		WillReturnError(errors.New("relation gone"))
	expectDeployInsert(mock, "twitch", `{"component":"tripbot","version":"v3.0.0"}`)

	recorded, err := Deploy(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "tripbot", "v3.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !recorded {
		t.Error("recorded = false, want true when the dedup read fails")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A watchdog_restart row is a system event whose meta names the watchdog and
// the restart's outcome. The exact JSON matters — Grafana and the rollups
// address it as meta->>'watchdog' / meta->>'outcome'.
func TestWatchdogRestartRow(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"",                 // username: system event, no actor
			"tiktok",           // platform
			"watchdog_restart", // event
			sqlmock.AnyArg(),   // session_id
			sqlmock.AnyArg(),   // date_created
			nil,                // extra_miles_earned
			nil,                // video_id
			nil,                // video_ts_sec
			`{"watchdog":"tiktok","outcome":"failed"}`, // meta
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := WatchdogRestart(context.Background(), &c.TripbotConfig{Platform: "tiktok"}, "tiktok", WatchdogOutcomeFailed); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// A watchdog_recovered row carries no outcome — holding is the outcome.
func TestWatchdogRecoveredRow(t *testing.T) {
	mock := installMockDB(t)
	mock.ExpectQuery(`INSERT INTO "events"`).
		WithArgs(
			"", "twitch", "watchdog_recovered",
			sqlmock.AnyArg(), sqlmock.AnyArg(), nil,
			nil, nil,
			`{"watchdog":"twitch"}`,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	if err := WatchdogRecovered(context.Background(), &c.TripbotConfig{Platform: "twitch"}, "twitch"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
