package errors

import "errors"

// ErrNoFootageForState is returned when no video in the corpus matches a
// requested state. !jump reports it to chat as "no footage yet" rather than as
// a failure, so it's part of that command's normal flow.
var ErrNoFootageForState = errors.New("no matches found")

// ErrReadOnly is returned by writers that refuse to touch the DB because the
// instance is in read-only mode.
var ErrReadOnly = errors.New("read-only mode")

// ErrNoDaytimeFound is returned when no clip later in the corpus falls in
// daylight within the scanned window.
var ErrNoDaytimeFound = errors.New("no daytime footage found ahead")
