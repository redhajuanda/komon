package fail

import (
	"github.com/cockroachdb/errors"
)

// Fail is the central error type in this package.
// It wraps an original error (with stack trace) and optionally pairs it
// with a Failure for safe customer-facing responses.
//
// Always use New or Wrap to construct a Fail so the stack trace
// is captured at the correct call site.
type Fail struct {
	public        *Failure
	originalError error // always has a stack trace via cockroachdb/errors
	data          any   // optional extra data (e.g. validation errors)
}

// -----------------------------------------------------------------
// Constructors
// -----------------------------------------------------------------

// New creates a new Fail from a message string.
// The stack trace is captured at the call site.
//
//	return fail.New("user record is corrupted")
func New(msg string) *Fail {
	return &Fail{
		originalError: errors.WithStackDepth(errors.New(msg), 1),
	}
}

// Newf creates a new Fail from a formatted message string.
// The stack trace is captured at the call site.
//
//	return fail.Newf("user %d not found", userID)
func Newf(format string, args ...any) *Fail {
	return &Fail{
		originalError: errors.WithStackDepth(errors.Newf(format, args...), 1),
	}
}

// Wrap wraps an existing error into a Fail and captures the stack trace.
// Returns nil if err is nil.
//
//	return fail.Wrap(err)
func Wrap(err error) *Fail {
	if err == nil {
		return nil
	}

	// If it's already a *Fail, re-wrap preserving the public/data.
	// Only add a new stack frame, don't double-wrap the inner error.
	if f, ok := err.(*Fail); ok {

		if f.originalError != nil {
			return f
		}

		return &Fail{
			public:        f.public,
			originalError: errors.WithStackDepth(f.originalError, 1),
			data:          f.data,
		}
	}

	return &Fail{
		originalError: errors.WithStackDepth(err, 1),
	}
}

// Wrapf wraps an existing error with an additional message and captures the stack trace.
// Returns nil if err is nil.
//
//	return fail.Wrapf(err, "failed to load user %d", userID)
func Wrapf(err error, format string, args ...any) *Fail {
	if err == nil {
		return nil
	}

	if f, ok := err.(*Fail); ok {

		if f.originalError != nil {
			return f
		}

		return &Fail{
			public:        f.public,
			originalError: errors.WrapWithDepthf(1, f.originalError, format, args...),
			data:          f.data,
		}
	}

	return &Fail{
		originalError: errors.WrapWithDepthf(1, err, format, args...),
	}
}

// -----------------------------------------------------------------
// Builder methods (chainable)
// -----------------------------------------------------------------

// WithFailure pairs the Fail with a Failure (public-safe response).
// This determines what is returned to the customer.
//
//	return fail.Wrap(err).WithFailure(fail.ErrNotFound)
func (f *Fail) WithFailure(pf *Failure) *Fail {
	f.public = pf
	return f
}

// WithData attaches arbitrary extra data to the Fail.
// Useful for validation errors, field-level details, etc.
//
//	return fail.Wrap(err).
//	    WithFailure(fail.ErrUnprocessable).
//	    WithData(validationErrs)
func (f *Fail) WithData(data any) *Fail {
	f.data = data
	return f
}

// -----------------------------------------------------------------
// Accessors
// -----------------------------------------------------------------

// OriginalError returns the underlying error with its full stack trace.
// Use this for internal logging — never expose it directly to end users.
func (f *Fail) OriginalError() error {
	return f.originalError
}

// Data returns the optional extra data attached to the Fail.
// Returns nil if no data was set.
func (f *Fail) Data() any {
	return f.data
}

// HasFailure reports whether a Failure (public-safe) was explicitly set.
// Useful when you want to distinguish "explicitly set internal server error"
// from "no predefined error set, fell back to internal server error".
func (f *Fail) HasFailure() bool {
	return f.public != nil
}

// GetFailure returns the paired Failure (public-safe response).
// If none was set, it falls back to the built-in ErrInternalServer.
// This is always safe to call and will never return nil.
func (f *Fail) GetFailure() *Failure {
	if f.public == nil {
		return ErrInternalServer
	}
	return f.public
}

// -----------------------------------------------------------------
// error interface
// -----------------------------------------------------------------

// Error implements the error interface.
// Returns the original error message (with context from Wrapf, etc.).
func (f *Fail) Error() string {
	return f.originalError.Error()
}

// Unwrap allows errors.Is and errors.As to traverse the chain.
func (f *Fail) Unwrap() error {
	return f.originalError
}
