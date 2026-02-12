package tracer

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// SetTraceProvider sets the global trace provider for the application.
// It returns an error if the trace provider cannot be set.
func SetTraceProvider(url string, service string, exporter string, sampleRate float64) error {
	tp, err := tracerProvider(url, service, exporter, sampleRate)
	if err != nil {
		return err
	}
	otel.SetTracerProvider(tp)
	return nil
}

// tracerProvider returns an OpenTelemetry TracerProvider configured to use
// the Jaeger exporter that will send spans to the provided url. The returned
// TracerProvider will also use a Resource configured with all the information
// about the application.
func tracerProvider(url string, service string, exporter string, sampleRate float64) (*tracesdk.TracerProvider, error) {

	var (
		exp tracesdk.SpanExporter
		err error
	)
	switch exporter {
	case "jaeger":
		// Create the Jaeger exporter
		exp, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown exporter %q", exporter)
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.TraceIDRatioBased(sampleRate)),
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
			// semconv.ServiceVersionKey.String(version),
			// attribute.String("environment", env),
		)),
	)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return tp, nil
}

// Trace creates a new span with the given name and returns the span and a context
// that has the span set as the current span in the context.
// The span is created with the caller function name as the operation name.
// If spanName is provided, it will be appended to the operation name.
// The caller function name is determined using the runtime.Caller function.
// The caller function name is the name of the function that calls the Trace function.
func Trace(ctx context.Context, spanName ...string) (context.Context, trace.Span) {

	c, _, _, _ := runtime.Caller(1)
	f := runtime.FuncForPC(c).Name()
	fs := strings.SplitN(f, ".", 2)
	replacer := strings.NewReplacer("(", "", ")", "", "*", "")
	operation := replacer.Replace(fs[1])

	if len(spanName) > 0 {
		operation = fmt.Sprintf("%s.%s", operation, strings.Join(spanName, "."))
	}
	return otel.Tracer(fs[0]).Start(ctx, operation)

}

// InjectTracer injects the tracer into the HTTP headers.
func InjectTracer(ctx context.Context, header http.Header) {
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(header))
}
