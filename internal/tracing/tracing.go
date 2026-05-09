package tracing

import (
	"context"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func Init(endpoint string) (func(), error) {
	ctx := context.Background()

	// WithEndpoint wants host:port only; parse it out of the full URL.
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	host := u.Host

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(host),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName("llm-observatory"),
		semconv.ServiceVersion("0.1.0"),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func() { _ = tp.Shutdown(ctx) }, nil
}
