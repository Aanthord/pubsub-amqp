package tracing

import (
	"fmt"
	"io"
	"net/http"

	"github.com/aanthord/pubsub-amqp/internal/config"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-lib/metrics"
)

// InitJaeger returns an instance of Jaeger Tracer
func InitJaeger(service string) (opentracing.Tracer, io.Closer, error) {
	cfg := jaegercfg.Configuration{
		ServiceName: service,
		Sampler: &jaegercfg.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
			LocalAgentHostPort: fmt.Sprintf("%s:%s",
				config.GetEnv("JAEGER_AGENT_HOST", "localhost"),
				config.GetEnv("JAEGER_AGENT_PORT", "6831"),
			),
		},
	}

	jLogger := jaegerlog.StdLogger
	jMetricsFactory := metrics.NullFactory

	return cfg.NewTracer(
		jaegercfg.Logger(jLogger),
		jaegercfg.Metrics(jMetricsFactory),
	)
}

// Middleware creates a new span for each incoming request
func Middleware(tracer opentracing.Tracer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			spanCtx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
			span := tracer.StartSpan("http_server", ext.RPCServerOption(spanCtx))
			defer span.Finish()

			ctx := opentracing.ContextWithSpan(r.Context(), span)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
