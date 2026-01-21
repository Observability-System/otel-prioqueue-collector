package freshnessexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		Type,
		createDefaultConfig,
		exporter.WithMetrics(createMetricsExporter, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

func createMetricsExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Metrics, error) {
	oCfg := cfg.(*Config)

	exp := &freshnessExporter{
		cfg:      oCfg,
		settings: set, // exporter.Settings
	}

	return exporterhelper.NewMetrics(
		ctx,
		set,
		cfg,
		exp.consume,
		exporterhelper.WithStart(exp.start),
		exporterhelper.WithShutdown(exp.shutdown),
	)
}
