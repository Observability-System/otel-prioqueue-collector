package weightedqueueprocessor

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	weightupdateextension "github.com/alexandrosst/weightupdateextension"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// dynamicQueue for resizable queues
type dynamicQueue struct {
	mu    sync.Mutex
	items []pmetric.Metrics
	cap   int
}

func (q *dynamicQueue) enqueue(item pmetric.Metrics) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.cap {
		return false
	}
	q.items = append(q.items, item)
	return true
}

func (q *dynamicQueue) dequeue() (pmetric.Metrics, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return pmetric.Metrics{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

func (q *dynamicQueue) len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

func (q *dynamicQueue) setCap(newCap int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cap = newCap
	if len(q.items) > q.cap {
		q.items = q.items[:q.cap] // Trim excess
	}
}

func (q *dynamicQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items = nil
}

type weightedQueueProcessor struct {
	config                  *Config
	nextConsumer            consumer.Metrics
	logger                  *zap.Logger
	shutdownCh              chan struct{}
	wg                      sync.WaitGroup
	queues                  sync.Map                    // map[string]*dynamicQueue
	totalEnqueued           atomic.Int64                // Total batches across queues
	droppedBatchesCounter   metric.Int64Counter         // total drops
	queueLengthGauge        metric.Int64ObservableGauge // per-source length
	forwardedBatchesCounter metric.Int64Counter         // total forwarded
}

func (p *weightedQueueProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (p *weightedQueueProcessor) Start(ctx context.Context, _ component.Host) error {
	p.wg.Add(1)
	go p.dequeueLoop()
	p.wg.Add(1)
	go p.monitorSharedChanges()
	return nil
}

func (p *weightedQueueProcessor) Shutdown(ctx context.Context) error {
	close(p.shutdownCh)
	p.wg.Wait()
	p.queues.Range(func(key, value any) bool {
		value.(*dynamicQueue).close()
		return true
	})
	return nil
}

func (p *weightedQueueProcessor) calculateInitialCap() int {
	weightupdateextension.GlobalWeights.RLock()
	num := weightupdateextension.GlobalWeights.NumSources
	weightupdateextension.GlobalWeights.RUnlock()
	if num == 0 {
		return 100 // Fallback if no sources yet
	}
	return p.config.MaxTotalCapacity / num
}

func (p *weightedQueueProcessor) ConsumeMetrics(_ context.Context, md pmetric.Metrics) error {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		sourceVal, ok := rm.Resource().Attributes().Get(p.config.SourceAttribute)
		if !ok {
			p.logger.Warn("Missing source attribute, skipping", zap.String("attr", p.config.SourceAttribute))
			continue
		}
		source := sourceVal.Str()

		// Get or create queue
		qIface, loaded := p.queues.LoadOrStore(source, &dynamicQueue{cap: p.calculateInitialCap()})
		queue := qIface.(*dynamicQueue)
		if !loaded {
			p.maybeAddSource(source)
		}

		// Check global total before enqueue
		if p.totalEnqueued.Load()+1 > int64(p.config.MaxTotalCapacity) {
			p.logger.Warn("Global capacity exceeded, dropping batch", zap.String("source", source))
			p.droppedBatchesCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("source", source)))
			return errors.New("global queue full: backpressure")
		}

		// Clone and enqueue, check per-queue
		cloned := pmetric.NewMetrics()
		rm.CopyTo(cloned.ResourceMetrics().AppendEmpty())
		if !queue.enqueue(cloned) {
			p.logger.Warn("Per-queue capacity exceeded, dropping batch", zap.String("source", source))
			p.droppedBatchesCounter.Add(context.Background(), 1, metric.WithAttributes(attribute.String("source", source)))
			continue // Or return error for stricter
		}

		p.totalEnqueued.Add(1)
	}
	return nil
}

func (p *weightedQueueProcessor) maybeAddSource(source string) {
	weightupdateextension.GlobalWeights.RLock()
	_, exists := weightupdateextension.GlobalWeights.Weights[source]
	weightupdateextension.GlobalWeights.RUnlock()

	if exists {
		return
	}

	weightupdateextension.GlobalWeights.Lock()
	n := len(weightupdateextension.GlobalWeights.Weights)
	if n == 0 {
		weightupdateextension.GlobalWeights.Weights[source] = 1.0
	} else {
		equal := 1.0 / float64(n+1)
		for k := range weightupdateextension.GlobalWeights.Weights {
			weightupdateextension.GlobalWeights.Weights[k] = equal
		}
		weightupdateextension.GlobalWeights.Weights[source] = equal
	}
	weightupdateextension.GlobalWeights.NumSources = n + 1
	weightupdateextension.GlobalWeights.Unlock()

	p.updateQueueCaps()

	p.logger.Info("New source added, weights rebalanced", zap.String("source", source), zap.Int("total", weightupdateextension.GlobalWeights.NumSources))
}

func (p *weightedQueueProcessor) monitorSharedChanges() {
	defer p.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdownCh:
			return
		case <-ticker.C:
			p.cleanDeletedQueues()
			p.updateQueueCaps()
		}
	}
}

func (p *weightedQueueProcessor) cleanDeletedQueues() {
	p.queues.Range(func(key, value any) bool {
		source := key.(string)
		weightupdateextension.GlobalWeights.RLock()
		_, exists := weightupdateextension.GlobalWeights.Weights[source]
		weightupdateextension.GlobalWeights.RUnlock()
		if !exists {
			value.(*dynamicQueue).close()
			p.queues.Delete(key)
			p.logger.Info("Deleted queue for removed source", zap.String("source", source))
		}
		return true
	})
}

func (p *weightedQueueProcessor) updateQueueCaps() {
	weightupdateextension.GlobalWeights.RLock()
	num := weightupdateextension.GlobalWeights.NumSources
	weightupdateextension.GlobalWeights.RUnlock()

	if num == 0 {
		return
	}
	perQueueCap := p.config.MaxTotalCapacity / num

	p.queues.Range(func(key, value any) bool {
		queue := value.(*dynamicQueue)
		queue.setCap(perQueueCap)
		return true
	})

	p.logger.Debug("Updated per-queue capacities equally", zap.Int("per_queue", perQueueCap))
}

func (p *weightedQueueProcessor) dequeueLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(time.Duration(p.config.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-p.shutdownCh:
			return
		case <-ticker.C:
			source := p.selectWeightedSource()
			if source == "" {
				continue
			}

			qIface, ok := p.queues.Load(source)
			if !ok {
				continue
			}
			queue := qIface.(*dynamicQueue)

			batch, ok := queue.dequeue()
			if !ok {
				continue
			}

			if err := p.nextConsumer.ConsumeMetrics(context.Background(), batch); err != nil {
				p.logger.Error("Failed to forward batch", zap.Error(err))
			} else {
				p.totalEnqueued.Add(-1)
				p.forwardedBatchesCounter.Add(context.Background(), 1,
					metric.WithAttributes(attribute.String("source", source)),
				)
			}
		}
	}
}

func (p *weightedQueueProcessor) selectWeightedSource() string {
	weightupdateextension.GlobalWeights.RLock()
	weights := make(map[string]float64, len(weightupdateextension.GlobalWeights.Weights))
	for k, v := range weightupdateextension.GlobalWeights.Weights {
		weights[k] = v
	}
	weightupdateextension.GlobalWeights.RUnlock()
	if len(weights) == 0 {
		return ""
	}

	var total float64
	for _, w := range weights {
		total += w
	}
	r := rand.Float64() * total

	var acc float64
	for source, weight := range weights {
		acc += weight
		if r <= acc {
			qIface, ok := p.queues.Load(source)
			if ok && qIface.(*dynamicQueue).len() > 0 {
				return source
			}
		}
	}
	return ""
}
