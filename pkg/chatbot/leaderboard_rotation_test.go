package chatbot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/video"
)

func TestPickLeaderboard(t *testing.T) {
	tests := []struct {
		roll float64
		want leaderboardKind
	}{
		{0.0, totalMilesLeaderboard},
		{0.0499, totalMilesLeaderboard},
		{0.05, guessLeaderboard},
		{0.5249, guessLeaderboard},
		{0.525, monthlyMilesLeaderboard},
		{0.9999, monthlyMilesLeaderboard},
	}
	for _, tt := range tests {
		if got := pickLeaderboard(tt.roll); got != tt.want {
			t.Errorf("pickLeaderboard(%v) = %v, want %v", tt.roll, got, tt.want)
		}
	}
}

func TestShowRotatingLeaderboard_MonthlyMiles(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 12.5).
		AddRow("viewer2", 3.2)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	app.showRotatingLeaderboard(context.Background(), 0.99) // monthly miles

	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Monthly Miles", 2 rows)`) {
		t.Errorf("expected one Monthly Miles overlay call, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestShowRotatingLeaderboard_Guess(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	rows := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 7.0).
		AddRow("viewer2", 0.0) // zero-scorer, filtered
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(rows)

	app.showRotatingLeaderboard(context.Background(), 0.3) // guess

	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Correct Guesses This Month", 1 rows)`) {
		t.Errorf("expected one guess overlay call with the zero-scorer filtered, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// An empty guess board (early in the month) falls back to monthly miles
// rather than skipping the rotation slot.
func TestShowRotatingLeaderboard_EmptyGuess_FallsBackToMonthlyMiles(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	empty := sqlmock.NewRows([]string{"username", "value"})
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(empty)
	miles := sqlmock.NewRows([]string{"username", "value"}).
		AddRow("viewer1", 12.5)
	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(miles)

	app.showRotatingLeaderboard(context.Background(), 0.3) // guess → empty → miles

	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Monthly Miles", 1 rows)`) {
		t.Errorf("expected fallback to Monthly Miles overlay, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestShowRotatingLeaderboard_TotalMiles_TruncatesToSize(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	var lifetime [][]string
	for i := 0; i < leaderboardSize+5; i++ {
		lifetime = append(lifetime, []string{fmt.Sprintf("viewer%d", i), "100.0"})
	}
	app.Sessions = &recordingSessions{Leaderboard: lifetime}

	app.showRotatingLeaderboard(context.Background(), 0.01) // total miles

	want := fmt.Sprintf(`ShowLeaderboard("Total Miles", %d rows)`, leaderboardSize)
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected truncated Total Miles overlay (%s), got %v", want, rec.Calls)
	}
}

// If both the pick and the monthly-miles fallback are empty, no overlay is
// published at all.
func TestShowRotatingLeaderboard_AllEmpty_SkipsOverlay(t *testing.T) {
	mock := installMockDB(t)
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	mock.ExpectQuery(`SELECT users\.username, scores\.value FROM "scores"`).
		WillReturnRows(sqlmock.NewRows([]string{"username", "value"}))

	// total miles via noopSessions returns nil; the miles fallback is empty too
	app.showRotatingLeaderboard(context.Background(), 0.01)

	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call when every board is empty, got %v", rec.Calls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
