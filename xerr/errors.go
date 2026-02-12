package xerr

// To create a new error, use the following function:
// `xerr.New(msg string)` => to create a new error

// To wrap error using xerr package, use the following function:
// `xerr.Wrap(err)` => to wrap existing error

// To create a new error with predefined error, use the following function:
// `xerr.ErrNotFound.New(msg string)` => to create a new error with predefined error

// To wrap error using predefined error, use the following function:
// `xerr.ErrNotFound.Wrap(err)` => to wrap existing error with predefined error

// To add data to the error, use the following function:
// `xerr.New(msg string).WithData(data any)` => to create a new error with data
// `xerr.Wrap(err).WithData(data any)` => to wrap existing error with data

import (
	"fmt"

	pkgerrors "github.com/cockroachdb/errors"
)

// New creates a new error with the given message that will contain the stack trace of the error
func New(msg string) *Error {
	return ErrInternalServer.err.withOriginalErrorDepth(pkgerrors.New(msg), 2)
}

// Wrap wraps an existing error and will contain the stack trace of the error
func Wrap(err error) *Error {
	if e, ok := err.(*Error); ok {

		// if error is already contains original error, just wrap it with the new error
		if e.OriginalError != nil {
			return e
		}

		// if error is not wrapped by xerr, wrap it with xerr
		return e.withOriginalErrorDepth(err, 2)
	}
	// return ErrInternalServer.withOriginalErrorDepth(err, 2)
	return ErrInternalServer.err.withOriginalErrorDepth(err, 2)
}

type Error struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	HTTPStatus    int    `json:"http_status"`
	Data          any    `json:"data,omitempty"`
	OriginalError error  `json:"-"`
}

// Error returns the error message
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] => %s", e.Message, e.OriginalError.Error())
}

// WithData adds data to the error, this will override the existing data if any
func (e *Error) WithData(data any) *Error {
	e.Data = data
	return e
}

func (e *Error) WithMessage(msg any) *Error {
	if v, ok := msg.(*Error); ok {
		e.Message = v.OriginalError.Error()
		return e
	}
	e.Message = fmt.Sprintf("%v", msg)
	return e
}

// withOriginalErrorDepth adds original error to the error and will contain the stack trace of the error
func (e *Error) withOriginalErrorDepth(err error, depth int) *Error {
	e.OriginalError = pkgerrors.WithStackDepth(err, depth)
	return e
}
