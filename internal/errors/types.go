package errors

type NoFootageForStateError struct {
	Msg string
}

func (e *NoFootageForStateError) Error() string {
	return e.Msg
}
