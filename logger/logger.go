package logger

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/redhajuanda/komon/tracer"

	"github.com/sirupsen/logrus"
)

// Params type, used to pass to `WithParams`.
type Params map[string]any

// Logger represent common interface for logging function
type Logger interface {
	// WithContext adds request_id and correlation_id that are available in the context to the log entry. If request_id or correlation_id is not available, it will skip adding it to the log entry
	WithContext(ctx context.Context) Logger
	// WithStack adds the stack trace to the log entry, if err is not nil
	WithStack(err error) Logger
	// WithParam adds a single key-value pair to the log entry
	WithParam(key string, value any) Logger
	// WithParams adds multiple key-value pairs to the log entry
	WithParams(params Params) Logger
	// SkipSource skips adding source field to the log entry
	SkipSource() Logger
	// Errorf formats and writes an error message to the log entry
	Errorf(format string, args ...any)
	// Error writes an error message to the log entry
	Error(args ...any)
	// Fatalf formats and writes a fatal message to the log entry
	Fatalf(format string, args ...any)
	// Fatal writes a fatal message to the log entry
	Fatal(args ...any)
	// Infof formats and writes an info message to the log entry
	Infof(format string, args ...any)
	// Info writes an info message to the log entry
	Info(args ...any)
	// Warnf formats and writes a warning message to the log entry
	Warnf(format string, args ...any)
	// Warn writes a warning message to the log entry
	Warn(args ...any)
	// Debugf formats and writes a debug message to the log entry
	Debugf(format string, args ...any)
	// Debug writes a debug message to the log entry
	Debug(args ...any)
	// Panicf formats and writes a panic message to the log entry
	Panicf(format string, args ...any)
	// Panic writes a panic message to the log entry
	Panic(args ...any)
}

type logger struct {
	entry          *logrus.Entry
	skipSource     bool
	redactedFields []string
}

var logStore *logger

type Options struct {
	RedactedFields []string
}

// New returns a new wrapper log
func New(serviceName string, options ...Options) Logger {

	redactedFields := []string{}
	if len(options) > 0 {
		redactedFields = options[0].RedactedFields
	}

	logStore = &logger{entry: logrus.New().WithFields(logrus.Fields{"service": serviceName}), skipSource: false, redactedFields: redactedFields}
	return logStore

}

// SetOutput sets the logger output.
func SetOutput(output io.Writer) {
	logStore.entry.Logger.SetOutput(output)
}

// SetFormatter sets the logger formatter.
func SetFormatter(formatter logrus.Formatter) {
	logStore.entry.Logger.SetFormatter(formatter)
}

// SetLevel sets the logger level.
func SetLevel(level logrus.Level) {
	logStore.entry.Logger.SetLevel(level)
}

// WithContext reads requestId and correlationId from context and adds to log field
func (l *logger) WithContext(ctx context.Context) Logger {

	entry := l.entry

	if ctx != nil {
		requestID := tracer.GetRequestID(ctx)
		if requestID != "" {
			entry = entry.WithField("request_id", requestID)
		}

		correlationID := tracer.GetCorrelationID(ctx)
		if correlationID != "" {
			entry = entry.WithField("correlation_id", correlationID)
		}
	}
	return &logger{entry: entry, skipSource: l.skipSource, redactedFields: l.redactedFields}
}

// WithStack adds the stack trace to the log entry, if err is not nil
func (l *logger) WithStack(err error) Logger {

	stack := tracer.MarshalStack(err)
	return &logger{entry: l.entry.WithField("stack", stack), skipSource: l.skipSource, redactedFields: l.redactedFields}
}

// WithParam adds a single key-value pair to the log entry
func (l *logger) WithParam(key string, value any) Logger {
	redactedValue := l.maskValue(key, value)
	return &logger{entry: l.entry.WithField(key, redactedValue), skipSource: l.skipSource, redactedFields: l.redactedFields}
}

// WithParams adds multiple key-value pairs to the log entry
func (l *logger) WithParams(params Params) Logger {
	redactedParams := l.maskParams(params)
	return &logger{entry: l.entry.WithFields(logrus.Fields(redactedParams)), skipSource: l.skipSource, redactedFields: l.redactedFields}
}

// SkipSource skips the source generation, it will not add the source to the log entry
func (l *logger) SkipSource() Logger {
	return &logger{entry: l.entry, skipSource: true, redactedFields: l.redactedFields}
}

// maskParams masks sensitive fields in params
func (l *logger) maskParams(params Params) Params {
	if len(l.redactedFields) == 0 {
		return params
	}

	redacted := make(Params, len(params))
	for key, value := range params {
		redacted[key] = l.maskValue(key, value)
	}
	return redacted
}

// maskValue masks the value if the key is in redactedFields, handles recursive masking
func (l *logger) maskValue(key string, value any) any {
	if len(l.redactedFields) == 0 {
		return value
	}

	// Check if this key should be redacted
	for _, field := range l.redactedFields {
		if strings.EqualFold(key, field) {
			return "[REDACTED]"
		}
	}

	// Recursively mask nested structures
	return l.maskRecursive(value)
}

// maskRecursive recursively masks sensitive fields in nested structures
func (l *logger) maskRecursive(value any) any {
	if len(l.redactedFields) == 0 {
		return value
	}

	switch v := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(v))
		for key, val := range v {
			redacted[key] = l.maskValue(key, val)
		}
		return redacted

	case Params:
		return l.maskParams(v)

	case []any:
		redacted := make([]any, len(v))
		for i, item := range v {
			redacted[i] = l.maskRecursive(item)
		}
		return redacted

	case []map[string]any:
		redacted := make([]map[string]any, len(v))
		for i, item := range v {
			redacted[i] = l.maskRecursive(item).(map[string]any)
		}
		return redacted

	default:
		return value
	}
}

// Error writes an error message to the log entry
func (l *logger) Error(args ...any) {
	l.appendSource()
	l.entry.Error(args...)
}

// Errorf formats and writes an error message to the log entry
func (l *logger) Errorf(format string, args ...any) {
	l.appendSource()
	l.entry.Errorf(format, args...)
}

// Fatal writes a fatal message to the log entry
func (l *logger) Fatal(args ...any) {
	l.appendSource()
	l.entry.Fatal(args...)
}

// Fatalf formats and writes a fatal message to the log entry
func (l *logger) Fatalf(format string, args ...any) {
	l.appendSource()
	l.entry.Fatalf(format, args...)
}

// Info writes an info message to the log entry
func (l *logger) Info(args ...any) {
	l.appendSource()
	l.entry.Info(args...)
}

// Infof formats and writes an info message to the log entry
func (l *logger) Infof(format string, args ...any) {
	l.appendSource()
	l.entry.Infof(format, args...)
}

// Warn writes a warning message to the log entry
func (l *logger) Warn(args ...any) {
	l.appendSource()
	l.entry.Warn(args...)
}

// Warnf formats and writes a warning message to the log entry
func (l *logger) Warnf(format string, args ...any) {
	l.appendSource()
	l.entry.Warnf(format, args...)
}

// Debug writes a debug message to the log entry
func (l *logger) Debug(args ...any) {
	l.appendSource()
	l.entry.Debug(args...)
}

// Debugf formats and writes a debug message to the log entry
func (l *logger) Debugf(format string, args ...any) {
	l.appendSource()
	l.entry.Debugf(format, args...)
}

// Panic writes a panic message to the log entry
func (l *logger) Panic(args ...any) {
	l.appendSource()
	l.entry.Panic(args...)
}

// Panicf formats and writes a panic message to the log entry
func (l *logger) Panicf(format string, args ...any) {
	l.appendSource()
	l.entry.Panicf(format, args...)
}

// appendSource appends the source to the log entry
func (l *logger) appendSource() {
	if l.skipSource {
		return
	}
	source := getSource(3)
	l.entry = l.entry.WithField("source", source)
}

// getSource generates a source string with format: "file:line (operation)"
func getSource(skip int) string {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown"
	}

	f := runtime.FuncForPC(pc).Name()
	// Split by "/" to get the last part of the package path
	parts := strings.Split(f, "/")
	lastPart := parts[len(parts)-1]

	// Split by "." and take everything after the first dot (package name)
	fs := strings.SplitN(lastPart, ".", 2)
	var operation string
	if len(fs) >= 2 {
		// Has package.function format
		operation = fs[1]
	} else {
		// Fallback: use the whole function name
		operation = lastPart
	}

	replacer := strings.NewReplacer("(", "", ")", "", "*", "")
	operation = replacer.Replace(operation)

	return fmt.Sprintf(" %s:%d (%s)", file, line, operation)
}
