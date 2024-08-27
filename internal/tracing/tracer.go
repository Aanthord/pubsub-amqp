package tracing

import (
    "context"
    "github.com/opentracing/opentracing-go"
    "github.com/uber/jaeger-client-go"
    "github.com/uber/jaeger-client-go/config"
)

func NewTracer(serviceName string) (opentracing.Tracer, error) {
    cfg := config.Configuration{
        Sampler: &config.SamplerConfig{
            Type:  jaeger.SamplerTypeConst,
            Param: 1,
        },
        Reporter: &config.ReporterConfig{
            LogSpans: true,
        },
    }
    tracer, _, err := cfg.NewTracer(config.Logger(jaeger.StdLogger))
    return tracer, err
}

func StartSpanFromContext(ctx context.Context, operationName string) (opentracing.Span, context.Context) {
    span, ctx := opentracing.StartSpanFromContext(ctx, operationName)
    return span, ctx
}
