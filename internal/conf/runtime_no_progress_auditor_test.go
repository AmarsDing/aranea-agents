package conf

import "testing"

// NoProgressAuditorConfig 零值语义（79-runtime-governance R5）：默认 ON，
// 3 次纠偏 / 再 2 次终止；enabled=false 为唯一回退开关。
func TestNoProgressAuditorConfig_NilAndZeroDefaults(t *testing.T) {
	var nilRuntime *Runtime
	zero := &Runtime{}
	for name, r := range map[string]*Runtime{"nil": nilRuntime, "zero": zero, "nil-sub": {NoProgressAuditor: nil}} {
		cfg := r.NoProgressAuditorConfig()
		if !cfg.Enabled {
			t.Errorf("%s: Enabled = false, want true（默认开启）", name)
		}
		if cfg.CorrectAfter != 3 {
			t.Errorf("%s: CorrectAfter = %d, want 3", name, cfg.CorrectAfter)
		}
		if cfg.CancelAfter != 2 {
			t.Errorf("%s: CancelAfter = %d, want 2", name, cfg.CancelAfter)
		}
	}
}

// proto3 optional 三态：nil=unset（默认 ON）；显式 false=kill switch；显式 true=ON。
func TestNoProgressAuditorConfig_EnabledTriState(t *testing.T) {
	unset := &Runtime{NoProgressAuditor: &Runtime_NoProgressAuditor{}}
	if cfg := unset.NoProgressAuditorConfig(); !cfg.Enabled {
		t.Error("enabled unset (nil) must default to true")
	}
	off := false
	killed := &Runtime{NoProgressAuditor: &Runtime_NoProgressAuditor{Enabled: &off}}
	if cfg := killed.NoProgressAuditorConfig(); cfg.Enabled {
		t.Error("enabled=false must act as kill switch")
	}
	on := true
	explicit := &Runtime{NoProgressAuditor: &Runtime_NoProgressAuditor{Enabled: &on}}
	if cfg := explicit.NoProgressAuditorConfig(); !cfg.Enabled {
		t.Error("enabled=true must stay on")
	}
}

func TestNoProgressAuditorConfig_Overrides(t *testing.T) {
	r := &Runtime{NoProgressAuditor: &Runtime_NoProgressAuditor{CorrectAfter: 5, CancelAfter: 4}}
	cfg := r.NoProgressAuditorConfig()
	if cfg.CorrectAfter != 5 || cfg.CancelAfter != 4 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}
