package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerConfig holds tracing configuration.
type TracerConfig struct {
	ServiceName    string // Service name for traces
	ServiceVersion string // Service version
	Environment    string // Environment (production, staging, development)
	OTLPEndpoint   string // OTLP collector endpoint (e.g., "localhost:4318")
	Enabled        bool   // Whether tracing is enabled
}

// Tracer wraps OpenTelemetry tracer with convenience methods.
type Tracer struct {
	tracer   trace.Tracer
	provider *sdktrace.TracerProvider
	enabled  bool
}

// NewTracer creates a new OpenTelemetry tracer.
// If config.Enabled is false, returns a no-op tracer.
func NewTracer(ctx context.Context, config TracerConfig) (*Tracer, error) {
	if !config.Enabled {
		return &Tracer{
			tracer:  otel.Tracer(config.ServiceName),
			enabled: false,
		}, nil
	}

	// Create OTLP exporter
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpoint(config.OTLPEndpoint),
		otlptracehttp.WithInsecure(), // Use TLS in production
	)

	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create tracer provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Sample all traces for now
	)

	// Set global provider and propagator
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		tracer:   provider.Tracer(config.ServiceName),
		provider: provider,
		enabled:  true,
	}, nil
}

// Shutdown gracefully shuts down the tracer.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span with the given name.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from context.
func (t *Tracer) SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span.
func (t *Tracer) AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetAttributes sets attributes on the current span.
func (t *Tracer) SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordError records an error on the current span.
func (t *Tracer) RecordError(ctx context.Context, err error, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.RecordError(err, trace.WithAttributes(attrs...))
	}
}

// IsEnabled returns whether tracing is enabled.
func (t *Tracer) IsEnabled() bool {
	return t.enabled
}

// Common attribute keys for blockchain events
var (
	AttrGameID          = attribute.Key("game.id")
	AttrEventType       = attribute.Key("event.type")
	AttrTransactionHash = attribute.Key("tx.hash")
	AttrTransactionLt   = attribute.Key("tx.lt")
	AttrBlockNumber     = attribute.Key("block.number")
	AttrWalletAddress   = attribute.Key("wallet.address")
	AttrErrorType       = attribute.Key("error.type")
)
