package xerr

import pkgerrors "github.com/cockroachdb/errors"

type PredefinedError struct {
	err Error
}

func Is(err error, e *PredefinedError) bool {
	return pkgerrors.Is(err, &e.err)
}

func (e *PredefinedError) New(msg string) *Error {
	return e.err.withOriginalErrorDepth(pkgerrors.New(msg), 2)
}

func (e *PredefinedError) Wrap(err error) *Error {
	if es, ok := err.(*Error); ok {
		e.err.OriginalError = es.OriginalError
		return &e.err
	}
	return e.err.withOriginalErrorDepth(err, 2)
}

func (e *PredefinedError) Unwrap(err error) *Error {
	if es, ok := err.(*Error); ok {
		e.err.OriginalError = es.OriginalError
		return &e.err
	}

	e.err.Message = err.Error()
	return e.err.withOriginalErrorDepth(err, 2)
}

// Standard predefined errors for common scenarios
// These errors can be used throughout the application to handle specific error cases.
// They can be extended or modified as needed.
var (
	ErrInternalServer  = &PredefinedError{err: Error{Code: "500000", Message: "The server encountered an internal error", HTTPStatus: 500}}
	ErrNotFound        = &PredefinedError{err: Error{Code: "404000", Message: "Your requested resource was not found", HTTPStatus: 404}}
	ErrBadRequest      = &PredefinedError{err: Error{Code: "400000", Message: "Your request is invalid", HTTPStatus: 400}}
	ErrUnauthorized    = &PredefinedError{err: Error{Code: "401000", Message: "Unauthorized access", HTTPStatus: 401}}
	ErrForbidden       = &PredefinedError{err: Error{Code: "403000", Message: "Forbidden access", HTTPStatus: 403}}
	ErrConflict        = &PredefinedError{err: Error{Code: "409000", Message: "Conflict", HTTPStatus: 409}}
	ErrGatewayTimeout  = &PredefinedError{err: Error{Code: "504000", Message: "Gateway timeout", HTTPStatus: 504}}
	ErrTooManyRequests = &PredefinedError{err: Error{Code: "429000", Message: "Too many requests", HTTPStatus: 429}}
)
