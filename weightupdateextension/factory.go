package weightupdateextension

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

const (
	typeStr   = "weightupdate"
	stability = component.StabilityLevelBeta
)

func NewFactory() extension.Factory {
	return extension.NewFactory(
		component.MustNewType(typeStr),
		createDefaultConfig,
		createExtension,
		stability,
	)
}

func createDefaultConfig() component.Config {
	return &Config{Port: 4500}
}

func createExtension(
	ctx context.Context,
	set extension.Settings,
	cfg component.Config,
) (extension.Extension, error) {
	oCfg := cfg.(*Config)
	return newExtension(ctx, set, oCfg)
}
