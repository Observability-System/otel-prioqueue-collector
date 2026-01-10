package weightupdateextension

import "go.opentelemetry.io/collector/component"

type Config struct {
    Port int `mapstructure:"port"` // e.g., 4500
}

var _ component.Config = (*Config)(nil)
