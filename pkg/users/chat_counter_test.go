package users

import "testing"

// stubChatCounter drains a canned tally, like the real counter does.
type stubChatCounter struct{ n int }

func (s *stubChatCounter) Drain() int {
	d := s.n
	s.n = 0
	return d
}

// With no counter wired, the sample tick reports nil — a NULL chat_messages
// row, distinct from a wired-and-silent 0.
func TestChatMessagesWithoutCounter(t *testing.T) {
	if got := New(testConf, noopChatterSource{}).chatMessages(); got != nil {
		t.Errorf("chatMessages() = %d, want nil with no counter wired", *got)
	}
}

// A wired counter is drained once per tick: the first read takes the window's
// tally, the next starts from zero, so no message is counted twice.
func TestChatMessagesDrainsCounter(t *testing.T) {
	s := New(testConf, noopChatterSource{})
	s.SetChatCounter(&stubChatCounter{n: 7})

	if got := s.chatMessages(); got == nil || *got != 7 {
		t.Fatalf("chatMessages() = %v, want 7", got)
	}
	if got := s.chatMessages(); got == nil || *got != 0 {
		t.Errorf("second tick chatMessages() = %v, want 0 (drained)", got)
	}
}
