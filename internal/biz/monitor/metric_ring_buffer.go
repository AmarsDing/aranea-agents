package monitor

import (
	"sync"
	"time"
)

type metricBucket struct {
	startUnix int64
	totals    map[string]int64
	errors    map[string]int64
	durations map[string]durationAccum
}

type durationAccum struct {
	sum   int64
	count int64
}

func newMetricBucket(startUnix int64) metricBucket {
	return metricBucket{
		startUnix: startUnix,
		totals:    make(map[string]int64),
		errors:    make(map[string]int64),
		durations: make(map[string]durationAccum),
	}
}

func (b *metricBucket) incTotal(key string) {
	b.totals[key]++
}

func (b *metricBucket) incError(key string) {
	b.errors[key]++
}

func (b *metricBucket) addDuration(key string, ms int64) {
	d := b.durations[key]
	d.sum += ms
	d.count++
	b.durations[key] = d
}

// MetricRingBuffer is a fixed-size ring of time-bucketed metric counters.
// Each bucket covers bucketSize duration (default 1 minute).
// The buffer holds `capacity` buckets, providing a sliding window of
// capacity × bucketSize (default 60 minutes).
//
// It is safe for concurrent use: Increment is called from event bus consumers
// (hot path), while SumLastN is called from the periodic alert evaluator.
type MetricRingBuffer struct {
	mu        sync.Mutex
	buckets   []metricBucket
	capacity  int
	bucketSize time.Duration
}

const (
	defaultBucketCapacity = 60
	defaultBucketSize     = time.Minute
)

func NewMetricRingBuffer() *MetricRingBuffer {
	return &MetricRingBuffer{
		buckets:    make([]metricBucket, 0, defaultBucketCapacity),
		capacity:   defaultBucketCapacity,
		bucketSize: defaultBucketSize,
	}
}

func (rb *MetricRingBuffer) currentBucketStart() int64 {
	now := time.Now().UTC()
	return now.Truncate(rb.bucketSize).Unix()
}

func (rb *MetricRingBuffer) ensureBucket() *metricBucket {
	start := rb.currentBucketStart()
	if len(rb.buckets) > 0 && rb.buckets[len(rb.buckets)-1].startUnix == start {
		return &rb.buckets[len(rb.buckets)-1]
	}
	if len(rb.buckets) >= rb.capacity {
		copy(rb.buckets, rb.buckets[1:])
		rb.buckets = rb.buckets[:rb.capacity-1]
	}
	rb.buckets = append(rb.buckets, newMetricBucket(start))
	return &rb.buckets[len(rb.buckets)-1]
}

func (rb *MetricRingBuffer) ensureBucketAt(startUnix int64) *metricBucket {
	for i := range rb.buckets {
		if rb.buckets[i].startUnix == startUnix {
			return &rb.buckets[i]
		}
	}
	if len(rb.buckets) >= rb.capacity {
		copy(rb.buckets, rb.buckets[1:])
		rb.buckets = rb.buckets[:rb.capacity-1]
	}
	rb.buckets = append(rb.buckets, newMetricBucket(startUnix))
	return &rb.buckets[len(rb.buckets)-1]
}

func (rb *MetricRingBuffer) IncTotal(key string) {
	rb.mu.Lock()
	b := rb.ensureBucket()
	b.incTotal(key)
	rb.mu.Unlock()
}

func (rb *MetricRingBuffer) IncError(key string) {
	rb.mu.Lock()
	b := rb.ensureBucket()
	b.incError(key)
	rb.mu.Unlock()
}

func (rb *MetricRingBuffer) AddDuration(key string, ms int64) {
	rb.mu.Lock()
	b := rb.ensureBucket()
	b.addDuration(key, ms)
	rb.mu.Unlock()
}

func (rb *MetricRingBuffer) RecordCompletion(status string, durationMs int64) {
	rb.IncTotal("runner.completion")
	if status == "error" {
		rb.IncError("runner.completion")
	}
	if durationMs > 0 {
		rb.AddDuration("runner.completion", durationMs)
	}
}

type WindowResult struct {
	Total   int64
	Errors  int64
	AvgMs   float64
	CountMs int64
}

func (rb *MetricRingBuffer) SumLastN(windowMinutes int) WindowResult {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if windowMinutes <= 0 {
		windowMinutes = defaultBucketCapacity
	}
	if windowMinutes > rb.capacity {
		windowMinutes = rb.capacity
	}

	cutoff := time.Now().UTC().Add(-time.Duration(windowMinutes) * time.Minute).Truncate(rb.bucketSize).Unix()
	var out WindowResult
	for i := len(rb.buckets) - 1; i >= 0; i-- {
		if rb.buckets[i].startUnix < cutoff {
			break
		}
		b := rb.buckets[i]
		out.Total += b.totals["runner.completion"]
		out.Errors += b.errors["runner.completion"]
		d := b.durations["runner.completion"]
		out.AvgMs += float64(d.sum)
		out.CountMs += d.count
	}
	if out.CountMs > 0 {
		out.AvgMs = out.AvgMs / float64(out.CountMs)
	} else {
		out.AvgMs = 0
	}
	return out
}
