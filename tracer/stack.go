package tracer

import (
	pkgerrors "github.com/pkg/errors"
)

var (
	StackSourceFileName     = "source"
	StackSourceLineName     = "line"
	StackSourceFunctionName = "func"
)

type StackTracer interface {
	StackTrace() pkgerrors.StackTrace
}

type state struct {
	b    []byte
	plus bool
}

// Write implements fmt.Formatter interface.
func (s *state) Write(b []byte) (n int, err error) {
	s.b = append(s.b, b...) // Append instead of assign
	return len(b), nil
}

// Width implements fmt.Formatter interface.
func (s *state) Width() (wid int, ok bool) {
	return 0, false
}

// Precision implements fmt.Formatter interface.
func (s *state) Precision() (prec int, ok bool) {
	return 0, false
}

// Flag implements fmt.Formatter interface.
func (s *state) Flag(c int) bool {
	if c == '+' {
		s.plus = true
		return true
	}
	return false
}

// frameField formats the frame according to the provided rune.
func frameField(f pkgerrors.Frame, s *state, c rune) string {
	s.b = s.b[:0]  // Clear the buffer
	s.plus = false // Reset the plus flag
	f.Format(s, c)
	if s.plus {
		f.Format(s, '+') // Apply the plus flag
	}
	return string(s.b)
}

// MarshalStack implements pkg/errors stack trace marshaling.
func MarshalStack(err error) interface{} {

	sterr, ok := err.(StackTracer)
	if !ok {
		return nil
	}
	st := sterr.StackTrace()
	s := &state{}
	out := make([]map[string]string, 0, len(st))
	for _, frame := range st {
		out = append(out, map[string]string{
			StackSourceFileName:     frameField(frame, s, 's'),
			StackSourceLineName:     frameField(frame, s, 'd'),
			StackSourceFunctionName: frameField(frame, s, 'n'),
		})
	}
	return out
}
