package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: MetricRingBuffer lives in the alert subpackage; aliases keep the
// historical monitor.* API surface intact.

type MetricRingBuffer = alert.MetricRingBuffer

type WindowResult = alert.WindowResult

func NewMetricRingBuffer() *MetricRingBuffer {
	return alert.NewMetricRingBuffer()
}
