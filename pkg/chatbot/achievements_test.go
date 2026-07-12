package chatbot

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/video"
)

func TestAchievementsCmd_NoneYet(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	out := captureSay(t, app)

	mock.ExpectQuery(`SELECT title FROM achievements`).
		WithArgs("twitch", "caller").
		WillReturnRows(sqlmock.NewRows([]string{"title"}))

	app.achievementsCmd(context.Background(), newTestUser("caller"), nil)

	want := "@caller has no achievements yet — keep watching!"
	if got := out(); got != want {
		t.Errorf("chat output = %q, want %q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}

func TestAchievementsCmd_ListsNewestFirstAndCaps(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	out := captureSay(t, app)

	rows := sqlmock.NewRows([]string{"title"})
	for _, title := range []string{
		"Saw Old Faithful", "100th visit to California", "10th visit to California",
		"First visit to California", "First visit to Wyoming", "First visit to Utah",
	} {
		rows.AddRow(title)
	}
	mock.ExpectQuery(`SELECT title FROM achievements`).
		WithArgs("twitch", "caller").
		WillReturnRows(rows)

	app.achievementsCmd(context.Background(), newTestUser("caller"), nil)

	want := "@caller has 6 🏆: Saw Old Faithful, 100th visit to California, " +
		"10th visit to California, First visit to California, First visit to Wyoming, +1 more"
	if got := out(); got != want {
		t.Errorf("chat output = %q, want %q", got, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
