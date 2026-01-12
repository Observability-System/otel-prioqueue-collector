package weightedqueueprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	typeStr   = "weightedqueue"
	stability = component.StabilityLevelBeta
)

var processorType = component.MustNewType(typeStr)

func NewFactory() processor.Factory {
	return processor.NewFactory(
		processorType,
		createDefaultConfig,
		processor.WithMetrics(createMetricsProcessor, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		SourceAttribute:  "source.id",
		InitialWeights:   make(map[string]float64),
		PollIntervalMs:   100,
		MaxTotalCapacity: 1000, // New
	}
}

func createMetricsProcessor(
	ctx context.Context,
	set processor.Settings,
	cfg component.Config,
	nextConsumer consumer.Metrics,
) (processor.Metrics, error) {
	conf := cfg.(*Config)

	p := &weightedQueueProcessor{
		config:       conf,
		nextConsumer: nextConsumer,
		logger:       set.Logger,
		shutdownCh:   make(chan struct{}),
	}

	// Create initial gauges/counters (for metrics exposure)
	meter := set.TelemetrySettings.MeterProvider.Meter("weightedqueueprocessor")

	droppedBatches, _ := meter.Int64Counter(
		"weightedqueue_dropped_batches",
		metric.WithDescription("Total dropped batches due to capacity"),
		metric.WithUnit("1"),
	)
	p.droppedBatchesCounter = droppedBatches

	queueLength, _ := meter.Int64ObservableGauge(
		"weightedqueue_queue_length",
		metric.WithDescription("Current queue length per source"),
		metric.WithUnit("1"),
	)
	p.queueLengthGauge = queueLength

	// Register observable callback
	_, _ = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			p.queues.Range(func(key, value any) bool {
				source := key.(string)
				q := value.(*dynamicQueue)
				o.ObserveInt64(queueLength, int64(q.len()),
					metric.WithAttributes(attribute.String("source", source)),
				)
				return true
			})
			return nil
		},
		queueLength,
	)

	return p, nil
}
