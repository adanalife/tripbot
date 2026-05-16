package chatbot

import "fmt"

// noopVLC satisfies VLC for tests that don't care about playback transitions
// — it just swallows every call.
type noopVLC struct{}

func (noopVLC) PlayRandom() error                     { return nil }
func (noopVLC) PlayFileInPlaylist(_ string) error     { return nil }
func (noopVLC) Skip(_ int) error                      { return nil }
func (noopVLC) Back(_ int) error                      { return nil }

// recordingVLC captures every call made to it so tests can assert that
// the chatbot drove playback as expected. All call records are appended
// in order to Calls.
type recordingVLC struct {
	Calls []string
}

func (r *recordingVLC) PlayRandom() error {
	r.Calls = append(r.Calls, "PlayRandom()")
	return nil
}
func (r *recordingVLC) PlayFileInPlaylist(filename string) error {
	r.Calls = append(r.Calls, fmt.Sprintf("PlayFileInPlaylist(%q)", filename))
	return nil
}
func (r *recordingVLC) Skip(n int) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Skip(%d)", n))
	return nil
}
func (r *recordingVLC) Back(n int) error {
	r.Calls = append(r.Calls, fmt.Sprintf("Back(%d)", n))
	return nil
}
