package tracer

import (
	"context"

	"github.com/oklog/ulid/v2"
)

type contextKey int

const (
	RequestIDKey contextKey = iota
	CorrelationIDKey
)

var (
	// RequestIDHeader is the name of the HTTP Header which contains the request id.
	// Exported so that it can be changed by developers
	RequestIDHeader     = "X-Request-ID"
	CorrelationIDHeader = "X-Correlation-ID"
)

// SetRequestID sets a request ID in the given context if one is not present.
// If the request ID is not provided, it will be generated using the ulid package.
func SetRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		requestID = ulid.Make().String()
	}
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// SetCorrelationID sets a correlation ID in the given context if one is not present.
// If the correlation ID is not provided, it will be generated using the ulid package.
func SetCorrelationID(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		correlationID = ulid.Make().String()
	}
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}

// GetRequestID returns a request ID from the given context if one is present.
// Returns the empty string if a request ID cannot be found.
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// GetCorrelationID returns a correlation ID from the given context if one is present.
// Returns the empty string if a correlation ID cannot be found.
func GetCorrelationID(ctx context.Context) string {
	if corrID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return corrID
	}
	return ""
}
