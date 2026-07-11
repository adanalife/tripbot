package video

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// expectVideoCount queues the playlist-length COUNT that bounds Next()'s walk.
func expectVideoCount(mock sqlmock.Sqlmock, count int64) {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "videos"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
}

// expectLoadByIdHit queues a successful loadById() returning the given
// id/flagged/next_vid row.
func expectLoadByIdHit(mock sqlmock.Sqlmock, id int64, flagged bool, nextVid int64) {
	mock.ExpectQuery(`SELECT \* FROM "videos" WHERE "videos"\."id" = `).
		WithArgs(id, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "flagged", "next_vid"}).
			AddRow(id, flagged, nextVid))
}

func TestVideoNext_ReturnsFirstUnflagged(t *testing.T) {
	mock := installMockDB(t)
	expectVideoCount(mock, 3)
	expectLoadByIdHit(mock, 2, true, 3)  // flagged, keep walking
	expectLoadByIdHit(mock, 3, false, 4) // unflagged, done

	v := Video{NextVid: sql.NullInt64{Int64: 2, Valid: true}}
	got, err := v.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v, want nil", err)
	}
	if got.ID != 3 || got.Flagged {
		t.Errorf("Next() = id %d flagged %v, want id 3 unflagged", got.ID, got.Flagged)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestVideoNext_BrokenChainReturnsError(t *testing.T) {
	mock := installMockDB(t)
	expectVideoCount(mock, 3)
	// loadById miss: next_vid points at a row that doesn't exist
	mock.ExpectQuery(`SELECT \* FROM "videos" WHERE "videos"\."id" = `).
		WithArgs(int64(42), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	v := Video{NextVid: sql.NullInt64{Int64: 42, Valid: true}}
	if _, err := v.Next(context.Background()); err == nil {
		t.Fatal("Next() with broken next_vid chain returned nil error, want error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestVideoNext_AllFlaggedCycleReturnsError(t *testing.T) {
	mock := installMockDB(t)
	// two flagged videos pointing at each other; the walk stops at count=2
	expectVideoCount(mock, 2)
	expectLoadByIdHit(mock, 1, true, 2)
	expectLoadByIdHit(mock, 2, true, 1)

	v := Video{NextVid: sql.NullInt64{Int64: 1, Valid: true}}
	if _, err := v.Next(context.Background()); err == nil {
		t.Fatal("Next() over an all-flagged cycle returned nil error, want error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
