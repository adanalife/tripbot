package chatbot

import "strings"

// noopIRC satisfies IRC for tests that don't care about chat output — it
// swallows everything. Tests that assert on output inject a recordingIRC
// instead (see captureSay / the *_ViaIRC tests).
type noopIRC struct{}

func (noopIRC) Say(_ string)        {}
func (noopIRC) Whisper(_, _ string) {}

// recordingIRC captures every Say/Whisper call so tests can assert on
// chat output. All call records are appended in order.
type recordingIRC struct {
	Says     []string          // ordered list of Say() messages
	Whispers []recordedWhisper // ordered list of Whisper() calls
}

type recordedWhisper struct {
	Username string
	Msg      string
}

func (r *recordingIRC) Say(msg string) {
	r.Says = append(r.Says, msg)
}

func (r *recordingIRC) Whisper(username, msg string) {
	r.Whispers = append(r.Whispers, recordedWhisper{Username: username, Msg: msg})
}

// Output returns all Say() messages joined by newline, mirroring the
// shape of captureSay()'s output() helper for easy migration.
func (r *recordingIRC) Output() string {
	return strings.Join(r.Says, "\n")
}
