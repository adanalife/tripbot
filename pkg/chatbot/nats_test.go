package chatbot

import (
	"context"
	"sync"
)

// recordingNATS captures every publish call so tests can assert on the
// subject + payload pair. Goroutine-safe so concurrent commands don't
// race the slice. It satisfies eventbus.Publisher.
type recordingNATS struct {
	mu        sync.Mutex
	Publishes []recordedPublish
}

type recordedPublish struct {
	Subject string
	Payload []byte
}

func (r *recordingNATS) Publish(_ context.Context, subject string, payload []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]byte, len(payload))
	copy(cp, payload)
	r.Publishes = append(r.Publishes, recordedPublish{Subject: subject, Payload: cp})
}
