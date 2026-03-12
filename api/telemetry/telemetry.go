package telemetry

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jonasbg/paste/m/v2/middleware"
	"github.com/jonasbg/paste/m/v2/utils"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const serviceName = "paste-api"

type Provider struct {
	meterProvider *sdkmetric.MeterProvider
	promHandler   http.Handler
	requests      metric.Int64Counter
	latency       metric.Float64Histogram
	transfers     metric.Int64Counter
	transferBytes metric.Int64Counter
}

func Init(ctx context.Context) (*Provider, error) {
	readers := make([]sdkmetric.Reader, 0, 2)

	if endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); endpoint != "" {
		exporter, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, err
		}
		readers = append(readers, sdkmetric.NewPeriodicReader(exporter))
	}

	var promHandler http.Handler
	if promEnabled() {
		exporter, err := prometheus.New()
		if err != nil {
			return nil, err
		}
		readers = append(readers, exporter)
		promHandler = promhttp.Handler()
	}

	if len(readers) == 0 {
		readers = append(readers, sdkmetric.NewManualReader())
	}

	options := make([]sdkmetric.Option, 0, len(readers))
	for _, reader := range readers {
		options = append(options, sdkmetric.WithReader(reader))
	}
	mp := sdkmetric.NewMeterProvider(options...)
	otel.SetMeterProvider(mp)

	meter := otel.Meter(serviceName)

	requests, err := meter.Int64Counter("http.server.requests")
	if err != nil {
		return nil, err
	}
	latency, err := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}
	transfers, err := meter.Int64Counter("paste.transfer.operations")
	if err != nil {
		return nil, err
	}
	transferBytes, err := meter.Int64Counter("paste.transfer.bytes")
	if err != nil {
		return nil, err
	}

	return &Provider{
		meterProvider: mp,
		promHandler:   promHandler,
		requests:      requests,
		latency:       latency,
		transfers:     transfers,
		transferBytes: transferBytes,
	}, nil
}

func promEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("OTEL_PROMETHEUS_ENABLED")))
	return value == "" || value == "true" || value == "1" || value == "yes"
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.meterProvider == nil {
		return nil
	}
	return p.meterProvider.Shutdown(ctx)
}

func (p *Provider) PrometheusHandler() http.Handler {
	return p.promHandler
}

func (p *Provider) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		durationMs := float64(time.Since(start).Milliseconds())
		attrs := []attribute.KeyValue{
			attribute.String("http.request.method", strings.ToLower(c.Request.Method)),
			attribute.String("http.route", routeLabel(c)),
			attribute.Int("http.response.status_code", c.Writer.Status()),
		}

		p.requests.Add(c.Request.Context(), 1, metric.WithAttributes(attrs...))
		p.latency.Record(c.Request.Context(), durationMs, metric.WithAttributes(attrs...))
	}
}

func routeLabel(c *gin.Context) string {
	if path := c.FullPath(); path != "" {
		return path
	}
	return c.Request.URL.Path
}

func (p *Provider) RecordTransfer(ctx context.Context, operation string, size int64, success bool, protocol string) {
	if p == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("paste.operation", operation),
		attribute.Bool("paste.success", success),
		attribute.String("network.protocol.name", protocol),
	}
	p.transfers.Add(ctx, 1, metric.WithAttributes(attrs...))
	if size > 0 {
		p.transferBytes.Add(ctx, size, metric.WithAttributes(attrs...))
	}
}

func MountPrometheusRoute(r *gin.Engine, handler http.Handler) error {
	if handler == nil {
		return nil
	}

	path := utils.GetEnv("OTEL_PROMETHEUS_PATH", "/metrics")
	if path == "" || path[0] != '/' {
		return errors.New("OTEL_PROMETHEUS_PATH must start with '/'")
	}

	allowedIPs := utils.GetEnv("METRICS_ALLOWED_IPS", "127.0.0.1/8,::1/128")
	r.GET(path, middleware.IPSourceRestriction(allowedIPs), gin.WrapH(handler))
	return nil
}
