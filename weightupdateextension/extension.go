package weightupdateextension

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.uber.org/zap"
)

type extensionImpl struct {
	config *Config
	logger *zap.Logger
	server *http.Server
}

func newExtension(_ context.Context, set extension.Settings, cfg *Config) (*extensionImpl, error) {
	return &extensionImpl{
		config: cfg,
		logger: set.Logger,
	}, nil
}

func (e *extensionImpl) Start(ctx context.Context, _ component.Host) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/update_weights", e.handleUpdateWeights)
	mux.HandleFunc("/weights", e.handleGetWeights)
	mux.HandleFunc("/delete_source", e.handleDeleteSource)

	e.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", e.config.Port),
		Handler: mux,
	}

	go func() {
		if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			e.logger.Error("Server failed", zap.Error(err))
		}
	}()
	e.logger.Info("Weight update server started", zap.Int("port", e.config.Port))
	return nil
}

func (e *extensionImpl) Shutdown(ctx context.Context) error {
	if e.server != nil {
		return e.server.Shutdown(ctx)
	}
	return nil
}

func (e *extensionImpl) handleUpdateWeights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Weights map[string]float64 `json:"weights"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate sum ≈1
	var sum float64
	for _, wt := range req.Weights {
		sum += wt
	}
	if math.Abs(sum-1.0) > 0.01 {
		http.Error(w, "Weights must sum to approximately 1", http.StatusBadRequest)
		return
	}

	// Update shared
	GlobalWeights.Lock()
	GlobalWeights.Weights = req.Weights
	GlobalWeights.NumSources = len(req.Weights)
	GlobalWeights.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Weights updated")
}

func (e *extensionImpl) handleGetWeights(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	GlobalWeights.RLock()
	resp := struct {
		Weights    map[string]float64 `json:"weights"`
		NumSources int                `json:"num_sources"`
	}{
		Weights:    GlobalWeights.Weights,
		NumSources: GlobalWeights.NumSources,
	}
	GlobalWeights.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (e *extensionImpl) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Source == "" {
		http.Error(w, "Missing source", http.StatusBadRequest)
		return
	}

	GlobalWeights.Lock()
	if _, exists := GlobalWeights.Weights[req.Source]; !exists {
		GlobalWeights.Unlock()
		http.Error(w, "Source not found", http.StatusNotFound)
		return
	}
	delete(GlobalWeights.Weights, req.Source)
	GlobalWeights.NumSources = len(GlobalWeights.Weights)
	if GlobalWeights.NumSources > 0 {
		equal := 1.0 / float64(GlobalWeights.NumSources)
		for k := range GlobalWeights.Weights {
			GlobalWeights.Weights[k] = equal
		}
	}
	GlobalWeights.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Source deleted and weights rebalanced")
}
