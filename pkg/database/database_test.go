package database

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
)

// GormDB is reached from every goroutine in the process — the crons, the IRC
// read loop, the HTTP handlers — and its lazy init writes a package global.
// Without the lock, two callers arriving before the handle exists both build
// one, which is a data race on gormConn and leaves one of the two connection
// pools orphaned. Run with -race for the former; the connect count catches the
// latter either way.
func TestGormDB_ConcurrentFirstUseConnectsOnce(t *testing.T) {
	want := &gorm.DB{}
	var connects atomic.Int32

	restore := connectGormFn
	connectGormFn = func() *gorm.DB {
		connects.Add(1)
		// Widen the window an unlocked check-then-set would race in. The real
		// connect dials postgres and can retry for seconds, so this is the
		// generous case, not a contrived one.
		time.Sleep(10 * time.Millisecond)
		return want
	}
	t.Cleanup(func() {
		connectGormFn = restore
		SetGormDB(nil)
	})
	SetGormDB(nil)

	const callers = 32
	got := make([]*gorm.DB, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func() {
			defer wg.Done()
			got[i] = GormDB()
		}()
	}
	wg.Wait()

	if n := connects.Load(); n != 1 {
		t.Errorf("connected %d times, want exactly 1", n)
	}
	for i, db := range got {
		if db != want {
			t.Errorf("caller %d got %p, want the shared handle %p", i, db, want)
		}
	}
}

// SetGormDB(nil) is what every test's teardown does, so the handle has to be
// rebuildable after a reset. This is why the guard is a mutex and not a
// sync.Once: a Once that already fired would hand back nil forever.
func TestGormDB_RebuildsAfterReset(t *testing.T) {
	first, second := &gorm.DB{}, &gorm.DB{}
	next := []*gorm.DB{first, second}

	restore := connectGormFn
	connectGormFn = func() *gorm.DB {
		db := next[0]
		next = next[1:]
		return db
	}
	t.Cleanup(func() {
		connectGormFn = restore
		SetGormDB(nil)
	})

	SetGormDB(nil)
	if got := GormDB(); got != first {
		t.Fatalf("first GormDB() = %p, want %p", got, first)
	}
	SetGormDB(nil)
	if got := GormDB(); got != second {
		t.Errorf("after reset GormDB() = %p, want a fresh handle %p", got, second)
	}
}

// A handle installed by SetGormDB (a sqlmock-backed gorm.DB in practice) wins
// over the connect path, which is what keeps the mock-DB tests in pkg/chatbot
// and pkg/events off a live postgres.
func TestGormDB_InstalledHandleSkipsConnect(t *testing.T) {
	mock := &gorm.DB{}

	restore := connectGormFn
	connectGormFn = func() *gorm.DB {
		t.Error("connected despite an installed handle")
		return &gorm.DB{}
	}
	t.Cleanup(func() {
		connectGormFn = restore
		SetGormDB(nil)
	})

	SetGormDB(mock)
	if got := GormDB(); got != mock {
		t.Errorf("GormDB() = %p, want the installed handle %p", got, mock)
	}
}
