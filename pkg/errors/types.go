package errors

type NoFootageForStateError struct {
	Msg string
}

func (e *NoFootageForStateError) Error() string {
	return e.Msg
}

type ReadOnlyError struct {
	Msg string
}

func (e *ReadOnlyError) Error() string {
	return e.Msg
}
