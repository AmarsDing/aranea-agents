package sandbox

import (
	"sync"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Prometheus metrics (design §8.1).
var (
	poolReadyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_sandbox_pool_ready",
		Help: "Warm-pool ready instances.",
	}, []string{"profile"})

	acquireDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_sandbox_acquire_seconds",
		Help:    "Sandbox acquisition latency (warm hit, cold create, or failure).",
		Buckets: []float64{0.05, 0.1, 0.15, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"result"})

	activeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "aranea_sandbox_active",
		Help: "Currently leased sandboxes.",
	}, []string{"profile"})

	execDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aranea_sandbox_exec_seconds",
		Help:    "Sandbox exec duration.",
		Buckets: prometheus.DefBuckets,
	}, []string{"profile", "status"})

	destroyTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_sandbox_destroy_total",
		Help: "Sandbox destructions by reason.",
	}, []string{"reason"})

	quotaRejectTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_sandbox_quota_reject_total",
		Help: "Acquire attempts rejected by quota.",
	}, []string{"scope"})
)

// labeledCounter is a tiny mutex-guarded int64 map for reason/scope labels.
type labeledCounter struct {
	mu sync.Mutex
	m  map[string]int64
}

func (c *labeledCounter) inc(label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]int64{}
	}
	c.m[label]++
}

func (c *labeledCounter) snapshot() map[string]int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int64, len(c.m))
	for k, v := range c.m {
		out[k] = v
	}
	return out
}

// stats mirrors the counters/gauges above into plain atomics so the admin
// API (GetSandboxMetrics) can serve a snapshot without scraping Prometheus.
type stats struct {
	acquireWarm  atomic.Int64
	acquireCold  atomic.Int64
	acquireFail  atomic.Int64
	execOK       atomic.Int64
	execError    atomic.Int64
	execTimeout  atomic.Int64
	destroy      labeledCounter
	quotaReject  labeledCounter
}

func newStats() *stats { return &stats{} }

// snapshot is the point-in-time copy consumed by Manager.Metrics.
type snapshot struct {
	AcquireWarm  int64
	AcquireCold  int64
	AcquireFail  int64
	ExecOK       int64
	ExecError    int64
	ExecTimeout  int64
	Destroy      map[string]int64
	QuotaReject  map[string]int64
}

func (s *stats) take() snapshot {
	return snapshot{
		AcquireWarm:  s.acquireWarm.Load(),
		AcquireCold:  s.acquireCold.Load(),
		AcquireFail:  s.acquireFail.Load(),
		ExecOK:       s.execOK.Load(),
		ExecError:    s.execError.Load(),
		ExecTimeout:  s.execTimeout.Load(),
		Destroy:      s.destroy.snapshot(),
		QuotaReject:  s.quotaReject.snapshot(),
	}
}
