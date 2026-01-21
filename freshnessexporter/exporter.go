package freshnessexporter

import (
	"context"
	"time"

	weightupdateextension "github.com/alexandrosst/weightupdateextension"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

type freshnessExporter struct {
	cfg      *Config
	settings exporter.Settings

	goodCounter  metric.Int64Counter
	totalCounter metric.Int64Counter
	logger       *zap.Logger
}

func (e *freshnessExporter) start(ctx context.Context, host component.Host) error {
	meter := e.settings.TelemetrySettings.MeterProvider.Meter("freshness")

	var err error
	e.goodCounter, err = meter.Int64Counter(
		"freshness_good_batches_total",
		metric.WithDescription("Number of metric batches received within freshness SLO threshold"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	e.totalCounter, err = meter.Int64Counter(
		"freshness_total_batches_total",
		metric.WithDescription("Total number of metric batches processed"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return err
	}

	e.logger = e.settings.TelemetrySettings.Logger

	// Apply any initial SLOs from config
	for tenant, durationStr := range e.cfg.InitialSLOs {
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			e.logger.Warn("Failed to parse initial SLO duration from config",
				zap.String("source", tenant),
				zap.String("duration", durationStr),
				zap.Error(err))
			continue
		}
		if err := weightupdateextension.SetSLOThresholdForTenant(tenant, duration.Nanoseconds(), "ns"); err != nil {
			e.logger.Warn("Failed to apply initial SLO from config",
				zap.String("source", tenant),
				zap.String("duration", durationStr),
				zap.Error(err))
			continue
		}
		e.logger.Info("Applied initial SLO from config",
			zap.String("tenant", tenant),
			zap.String("duration", durationStr))
	}

	e.logger.Info("Freshness exporter started",
		zap.String("source_attribute", e.cfg.SourceAttribute))

	return nil
}

func (e *freshnessExporter) consume(ctx context.Context, md pmetric.Metrics) error {
	rms := md.ResourceMetrics()
	now := time.Now().UnixNano()

	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		resAttrs := rm.Resource().Attributes()

		sourceVal, sourceOk := resAttrs.Get(e.cfg.SourceAttribute)
		if !sourceOk || sourceVal.Str() == "" {
			continue
		}
		source := sourceVal.Str()

		var initialTs int64
		found := false

		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len() && !found; j++ {
			metrics := sms.At(j).Metrics()
			for k := 0; k < metrics.Len() && !found; k++ {
				m := metrics.At(k)
				dps := getNumberDataPoints(m)
				if dps.Len() == 0 {
					continue
				}
				attrVal, ok := dps.At(0).Attributes().Get("initial_timestamp")
				if ok && attrVal.Type() == pcommon.ValueTypeInt {
					initialTs = attrVal.Int()
					found = true
				}
			}
		}

		if !found {
			e.logger.Debug("Skipping batch: no initial_timestamp found",
				zap.String("source", source))
			continue
		}

		freshnessNs := now - initialTs

		// Per-tenant SLO from shared state (runtime-updatable)
		threshold := weightupdateextension.GetSLOThresholdForTenant(source)

		sourceAttr := attribute.String("source", source)

		e.totalCounter.Add(ctx, 1, metric.WithAttributes(sourceAttr))

		if freshnessNs <= threshold && freshnessNs >= 0 {
			e.goodCounter.Add(ctx, 1, metric.WithAttributes(sourceAttr))
		}
	}

	return nil
}

func getNumberDataPoints(m pmetric.Metric) pmetric.NumberDataPointSlice {
	switch m.Type() {
	case pmetric.MetricTypeSum:
		return m.Sum().DataPoints()
	case pmetric.MetricTypeGauge:
		return m.Gauge().DataPoints()
	default:
		return pmetric.NumberDataPointSlice{}
	}
}

func (e *freshnessExporter) shutdown(context.Context) error {
	return nil
}
