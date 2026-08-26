package conf

import "testing"

// CacheHitAlertDriftThreshold 零值语义（79-runtime-governance 1.5）：
// nil/unset/zero/negative 一律回落默认 0.10；显式正值生效。
func TestCacheHitAlertDriftThreshold_NilAndZeroDefault(t *testing.T) {
	var nilRuntime *Runtime
	cases := map[string]*Runtime{
		"nil":      nilRuntime,
		"zero":     {},
		"negative": {CacheHitAlertDrift: -0.5},
	}
	for name, r := range cases {
		if got := r.CacheHitAlertDriftThreshold(); got != DefaultCacheHitAlertDrift {
			t.Errorf("%s: drift = %v, want default %v", name, got, DefaultCacheHitAlertDrift)
		}
	}
}

func TestCacheHitAlertDriftThreshold_ExplicitValue(t *testing.T) {
	r := &Runtime{CacheHitAlertDrift: 0.25}
	if got := r.CacheHitAlertDriftThreshold(); got != 0.25 {
		t.Errorf("drift = %v, want 0.25", got)
	}
}
