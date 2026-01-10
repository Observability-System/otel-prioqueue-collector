package weightedqueueprocessor

import "go.opentelemetry.io/collector/component"

type Config struct {
	SourceAttribute  string             `mapstructure:"source_attribute"`
	InitialWeights   map[string]float64 `mapstructure:"initial_weights"`
	PollIntervalMs   int                `mapstructure:"poll_interval_ms"`
	MaxTotalCapacity int                `mapstructure:"max_total_capacity"`
}

var _ component.Config = (*Config)(nil)
