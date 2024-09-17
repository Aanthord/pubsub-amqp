package tracing

import (
    "context"
    "fmt"
    "io"
    "os"
    "strconv"
    "time"

    "github.com/opentracing/opentracing-go"
    "github.com/opentracing/opentracing-go/ext"
    "github.com/opentracing/opentracing-go/log"
    "github.com/uber/jaeger-client-go"
    jaegercfg "github.com/uber/jaeger-client-go/config"
    jaegerlog "github.com/uber/jaeger-client-go/log"
    jaegertransport "github.com/uber/jaeger-client-go/transport"
    "github.com/uber/jaeger-lib/metrics"
)

// InitJaeger initializes the Jaeger Tracer with HTTP transport.
func InitJaeger(service string) (opentracing.Tracer, io.Closer, error) {
    // Use the environment variable to get the Jaeger collector URL
    collectorURL := os.Getenv("JAEGER_COLLECTOR_URL")
    if collectorURL == "" {
        return nil, nil, fmt.Errorf("JAEGER_COLLECTOR_URL is not set")
    }

    // Use HTTP transport instead of UDP
    httpTransport := jaegertransport.NewHTTPTransport(
        collectorURL,
        jaegertransport.HTTPBatchSize(1), // Optional: Adjust the batch size if necessary
    )

    // Parse the flush interval from environment variable, fallback to 1 second if not set
    flushInterval := 1 * time.Second // default flush interval
    if flushIntervalStr := os.Getenv("JAEGER_FLUSH_INTERVAL"); flushIntervalStr != "" {
        if parsed, err := strconv.Atoi(flushIntervalStr); err == nil {
            flushInterval = time.Duration(parsed) * time.Second
        } else {
            fmt.Printf("Invalid JAEGER_FLUSH_INTERVAL value, using default 1 second: %v\n", err)
        }
    }

    cfg := jaegercfg.Configuration{
        ServiceName: service,
        Sampler: &jaegercfg.SamplerConfig{
            Type:  jaeger.SamplerTypeConst,
            Param: 1, // Sample all traces; adjust for production if needed
        },
        Reporter: &jaegercfg.ReporterConfig{
            LogSpans:           true,       // Log all spans for debugging
            BufferFlushInterval: flushInterval, // Adjust the flush interval
        },
    }

    jLogger := jaegerlog.StdLogger
    jMetricsFactory := metrics.NullFactory

    // Set the tracer to use HTTP transport for reporting spans
    tracer, closer, err := cfg.NewTracer(
        jaegercfg.Logger(jLogger),
        jaegercfg.Metrics(jMetricsFactory),
        jaegercfg.Reporter(jaeger.NewRemoteReporter(httpTransport)),
    )
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create tracer: %w", err)
    }

    opentracing.SetGlobalTracer(tracer)
    return tracer, closer, nil
}

// StartSpanFromContext starts a new span from the provided context.
func StartSpanFromContext(ctx context.Context, operationName string, traceID string) (opentracing.Span, context.Context) {
    // Start a new span from the context
    span, ctx := opentracing.StartSpanFromContext(ctx, operationName)

    // Set the span kind as RPC client to indicate an outgoing request
    ext.SpanKindRPCClient.Set(span)

    // If a trace ID is provided, add it as a tag to the span
    if traceID != "" {
        span.SetTag("traceID", traceID)
    }

    // Log the start of the span with relevant information
    span.LogFields(
        log.String("event", "StartSpanFromContext"),
        log.String("operation", operationName),
        log.String("traceID", traceID),
    )

    return span, ctx
}

// InjectTraceToAMQP injects tracing information into AMQP headers.
func InjectTraceToAMQP(span opentracing.Span) map[string]string {
    carrier := make(opentracing.TextMapCarrier)
    err := opentracing.GlobalTracer().Inject(span.Context(), opentracing.TextMap, carrier)
    if err != nil {
        // Handle error
    }
    return carrier
}

// ExtractTraceFromAMQP extracts tracing information from AMQP headers.
func ExtractTraceFromAMQP(headers map[string]string) (opentracing.SpanContext, error) {
    return opentracing.GlobalTracer().Extract(opentracing.TextMap, opentracing.TextMapCarrier(headers))
}
