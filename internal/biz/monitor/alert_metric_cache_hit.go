package monitor

import (
	"aranea-agents/internal/biz/monitor/alert"
)

// DEV-05: the LLM cache-hit-ratio alert metric lives in the alert subpackage;
// aliases keep the historical monitor.* API surface intact.

const DefaultCacheHitRatioLowThreshold = alert.DefaultCacheHitRatioLowThreshold

type (
	CacheHitRatioBreach    = alert.CacheHitRatioBreach
	CacheHitRatioLowMetric = alert.CacheHitRatioLowMetric
)

var NewCacheHitRatioLowMetric = alert.NewCacheHitRatioLowMetric
