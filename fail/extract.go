package fail

import "github.com/cockroachdb/errors"

// Extract attempts to extract a *Fail from any error.
// It traverses the error chain using errors.As.
//
// Returns the *Fail and true if found, or nil and false otherwise.
//
//	f, ok := fail.Extract(err)
//	if ok {
//	    logger.Error(f.OriginalError())
//	    respondJSON(w, f.GetFailure(), f.Data())
//	}
func Extract(err error) (*Fail, bool) {
	if err == nil {
		return nil, false
	}
	var f *Fail
	if errors.As(err, &f) {
		return f, true
	}
	return nil, false
}

// MustExtract extracts a *Fail from any error, falling back to a synthetic
// Fail wrapping the original error with ErrInternalServer if the error is
// not a *Fail. This means you always get a usable *Fail back.
//
// This is the recommended function to use at your handler layer
// (HTTP, gRPC, etc.) so you always have a consistent response shape.
//
//	f := fail.MustExtract(err)
//	logger.Error(f.OriginalError())
//	respondJSON(w, f.GetFailure(), f.Data())
func MustExtract(err error) *Fail {
	if err == nil {
		return nil
	}
	if f, ok := Extract(err); ok {
		return f
	}
	// Wrap plain errors into a Fail so callers always get consistent behaviour.
	return Wrap(err)
}

// IsFailure reports whether the error chain contains a *Fail
// paired with the given Failure.
//
//	if fail.IsFailure(err, fail.ErrNotFound) {
//	    // handle not found
//	}
func IsFailure(err error, pf *Failure) bool {
	f, ok := Extract(err)
	if !ok {
		return false
	}
	return f.GetFailure() == pf
}

// FailureOf returns the Failure (public-safe) from the error chain.
// Falls back to ErrInternalServer if the error is not a *Fail or
// has no failure set.
//
//	pf := fail.FailureOf(err)
//	w.WriteHeader(pf.HTTPStatus)
func FailureOf(err error) *Failure {
	if err == nil {
		return nil
	}
	f, ok := Extract(err)
	if !ok {
		return ErrInternalServer
	}
	return f.GetFailure()
}
