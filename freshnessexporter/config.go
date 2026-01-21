package freshnessexporter

import (
	"errors"

	"go.opentelemetry.io/collector/component"
)

const TypeStr = "freshness"

var Type = component.MustNewType(TypeStr)

type Config struct {
	TenantAttribute string            `mapstructure:"tenant_attribute"`
	InitialSLOs     map[string]string `mapstructure:"initial_slos"` // e.g. "src1": "3s", "src2": "500ms"
}

var _ component.Config = (*Config)(nil)

func (cfg *Config) Validate() error {
	if cfg.TenantAttribute == "" {
		cfg.TenantAttribute = "source" // your actual attribute name
	}

	// Optional: basic sanity check for initial_slos
	for tenant, durationStr := range cfg.InitialSLOs {
		if tenant == "" {
			return errors.New("initial_slos tenant key cannot be empty")
		}
		if durationStr == "" {
			return errors.New("initial_slos duration cannot be empty for tenant: " + tenant)
		}
		// Further parsing/validation happens in SetSLOThresholdForTenant
	}

	return nil
}
