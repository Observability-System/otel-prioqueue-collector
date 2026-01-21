package weightupdateextension

// import "sync"
import (
	"errors"
	"strings"
	"sync"
)

var GlobalWeights = &SharedWeights{
	Weights:    make(map[string]float64),
	NumSources: 0,
}

type SharedWeights struct {
	sync.RWMutex
	Weights    map[string]float64
	NumSources int
}

// DefaultSLOThreshold is used when a tenant is not explicitly configured
var DefaultSLOThreshold int64 = 5_000_000_000 // 5 seconds in nanoseconds

type SharedSLOs struct {
	sync.RWMutex
	Thresholds map[string]int64 // tenant → threshold in nanoseconds
}

var GlobalSLOs = &SharedSLOs{
	Thresholds: make(map[string]int64),
}

// SetSLOThresholdForTenant sets threshold for a specific tenant with value + unit
func SetSLOThresholdForTenant(tenant string, value int64, unit string) error {
	if tenant == "" {
		return errors.New("tenant is required")
	}
	if value <= 0 {
		return errors.New("slo_threshold must be positive")
	}

	unit = strings.ToLower(strings.TrimSpace(unit))
	if unit == "" {
		unit = "s" // default unit = seconds
	}

	var multiplier int64
	switch unit {
	case "ns", "nanoseconds":
		multiplier = 1
	case "ms", "milliseconds":
		multiplier = 1_000_000
	case "s", "sec", "seconds":
		multiplier = 1_000_000_000
	default:
		return errors.New("invalid unit: must be ns, ms, or s")
	}

	ns := value * multiplier

	GlobalSLOs.Lock()
	GlobalSLOs.Thresholds[tenant] = ns
	GlobalSLOs.Unlock()

	return nil
}

// GetSLOThresholdForTenant returns the threshold in nanoseconds for a tenant
// Returns default if the tenant has no specific value
func GetSLOThresholdForTenant(tenant string) int64 {
	if tenant == "" {
		return DefaultSLOThreshold
	}

	GlobalSLOs.RLock()
	ns, exists := GlobalSLOs.Thresholds[tenant]
	GlobalSLOs.RUnlock()

	if !exists {
		return DefaultSLOThreshold
	}

	return ns
}

// RegisterNewTenant adds the tenant with default SLO if it doesn't exist yet
func RegisterNewTenant(tenant string) {
	if tenant == "" {
		return
	}
	GlobalSLOs.Lock()
	if _, exists := GlobalSLOs.Thresholds[tenant]; !exists {
		GlobalSLOs.Thresholds[tenant] = DefaultSLOThreshold
	}
	GlobalSLOs.Unlock()
}
