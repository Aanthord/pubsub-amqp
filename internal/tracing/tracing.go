package tracing

import (
    "context"
    "fmt"
    "io"
    "os"

    "github.com/opentracing/opentracing-go"
    "github.com/opentracing/opentracing-go/ext"
    "github.com/opentracing/opentracing-go/log"
    "github.com/uber/jaeger-client-go"
    jaegercfg "github.com/uber/jaeger-client-go/config"
    jaegerlog "github.com/uber/jaeger-client-go/log"
    "github.com/uber/jaeger-lib/metrics"
)

// InitJaeger initializes the Jaeger Tracer.
func InitJaeger(service string) (opentracing.Tracer, io.Closer, error) {
    cfg := jaegercfg.Configuration{
        ServiceName: service,
        Sampler: &jaegercfg.SamplerConfig{
            Type:  jaeger.SamplerTypeConst,
            Param: 1,
        },
        Reporter: &jaegercfg.ReporterConfig{
            LogSpans:           true,
            LocalAgentHostPort: os.Getenv("JAEGER_AGENT_HOST") + ":" + os.Getenv("JAEGER_AGENT_PORT"),
        },
    }

    jLogger := jaegerlog.StdLogger
    jMetricsFactory := metrics.NullFactory

    tracer, closer, err := cfg.NewTracer(
        jaegercfg.Logger(jLogger),
        jaegercfg.Metrics(jMetricsFactory),
    )
    if err != nil {
        return nil, nil, fmt.Errorf("failed to create tracer: %w", err)
    }

    opentracing.SetGlobalTracer(tracer)
    return tracer, closer, nil
}

// StartSpanFromContext starts a new span from the provided context.
func StartSpanFromContext(ctx context.Context, operationName string, traceID string) (opentracing.Span, context.Context) {
    span, ctx := opentracing.StartSpanFromContext(ctx, operationName)
    ext.SpanKindRPCClient.Set(span)
    span.SetTag("traceID", traceID)
    span.LogFields(log.String("event", "StartSpanFromContext"))
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
