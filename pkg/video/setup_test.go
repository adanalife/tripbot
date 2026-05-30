package video

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adanalife/tripbot/pkg/database"
	onscreensClient "github.com/adanalife/tripbot/pkg/onscreens-client"
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

// recordedCalls captures URL paths hit on a fake server.
type recordedCalls struct {
	paths []string
}

func (r *recordedCalls) add(p string) { r.paths = append(r.paths, p) }

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
	// vlcClient.New(host) builds the URL as "http://" + host, so strip the
	// scheme from the httptest URL before handing it over.
	return vlcClient.New(strings.TrimPrefix(srv.URL, "http://"))
}

// fakeOnscreensServer stands up an httptest.Server that records calls to the
// GPS show/hide endpoints. Tests assert on rec.paths to verify which overlay
// transitions fired.
//
// Returns a *onscreens-client.Client configured to talk to the fake and a
// pointer to the recorded-calls struct.
func fakeOnscreensServer(t *testing.T) (*onscreensClient.Client, *recordedCalls) {
	t.Helper()
	rec := &recordedCalls{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/onscreens/gps/show", "/onscreens/gps/hide":
			rec.add(r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	// nil publisher: this rig exercises the HTTP path only; NATS is off.
	return onscreensClient.New(strings.TrimPrefix(srv.URL, "http://"), nil, "test"), rec
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
