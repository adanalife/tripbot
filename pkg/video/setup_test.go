package video

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/database"
	vlcClient "github.com/adanalife/tripbot/pkg/vlc-client"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// installMockDB stands up a sqlmock-backed *gorm.DB and installs it as the
// process-wide singleton via database.SetGormDB. db.go's load/create/save
// helpers (which read from database.GormDB() directly) will route to the mock
// instead of attempting a real postgres connection.
//
// SkipDefaultTransaction stops GORM from wrapping every write in BEGIN/COMMIT,
// which would otherwise force every test to mock those bookends.
//
// Mirrors the pattern from pkg/chatbot/mockdb_test.go.
func installMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	database.SetGormDB(gdb)
	t.Cleanup(func() {
		database.SetGormDB(nil)
		_ = sqlDB.Close()
	})
	return mock
}

// recordingOnscreens is an interface fake satisfying the Player's onscreens
// dependency, capturing each GPS overlay call by method name. Replaces the
// old httptest-backed rig now that Player takes an interface.
type recordingOnscreens struct {
	calls []string
}

func (r *recordingOnscreens) ShowGPSImage(_ context.Context, _ time.Duration) error {
	r.calls = append(r.calls, "ShowGPSImage")
	return nil
}

func (r *recordingOnscreens) HideGPSImage(_ context.Context) error {
	r.calls = append(r.calls, "HideGPSImage")
	return nil
}

// fakeVLCServer stands up an httptest.Server that responds to /vlc/current
// with the value pointed to by current. Tests mutate *current to change what
// the next Player.GetCurrentlyPlaying call observes.
//
// Returns a *vlc-client.Client configured to talk to the fake.
func fakeVLCServer(t *testing.T, current *string) *vlcClient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/vlc/current" {
			_, _ = w.Write([]byte(*current))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	// vlcClient.New builds the URL as "http://" + host, so strip the scheme
	// from the httptest URL before handing it over. nil publisher disables the
	// NATS mirror — this rig exercises the HTTP path only.
	return vlcClient.New(strings.TrimPrefix(srv.URL, "http://"), nil, "test")
}

// expectLoadHit queues a sqlmock expectation for a successful load() — i.e.
// db.go's `SELECT ... WHERE slug = ?` returning one row with the given
// id/slug/flagged. State and other columns are left zero.
func expectLoadHit(mock sqlmock.Sqlmock, id int, slug string, flagged bool) {
	mock.ExpectQuery(`SELECT \* FROM "videos" WHERE slug = `).
		WithArgs(slug, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "flagged"}).
			AddRow(id, slug, flagged))
}
