package server

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// cmd/tripbot installs the flag client, bed store and rotator deps after the
// HTTP server is already answering, so a setter can run while a handler reads.
// Under -race (CI's test:cover) an unguarded field fails this.
func TestLateDepsSettersRaceHandlers(t *testing.T) {
	s := New(testConf)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 50 {
			s.SetFlags(nil)
			s.SetBeds(nil)
			s.SetRotators(nil, nil)
		}
	}()
	for range 50 {
		for _, h := range []http.HandlerFunc{s.flagsHandler, s.audioHandler, s.rotatorsGetHandler} {
			h(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
		}
	}
	wg.Wait()
}
