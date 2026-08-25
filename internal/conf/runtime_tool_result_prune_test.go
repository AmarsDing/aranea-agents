package conf

import "testing"

// ToolResultPruneConfig 零值语义（79-runtime-governance R2）：默认 ON，
// K=8 / S=4096 / 无豁免；enabled=false 为唯一回退开关。
func TestToolResultPruneConfig_NilAndZeroDefaults(t *testing.T) {
	var nilRuntime *Runtime
	zero := &Runtime{}
	for name, r := range map[string]*Runtime{"nil": nilRuntime, "zero": zero, "nil-sub": {ToolResultPrune: nil}} {
		cfg := r.ToolResultPruneConfig()
		if !cfg.Enabled {
			t.Errorf("%s: Enabled = false, want true（默认开启）", name)
		}
		if cfg.AfterTurns != 8 {
			t.Errorf("%s: AfterTurns = %d, want 8", name, cfg.AfterTurns)
		}
		if cfg.SizeBytes != 4096 {
			t.Errorf("%s: SizeBytes = %d, want 4096", name, cfg.SizeBytes)
		}
		if len(cfg.ExemptTools) != 0 {
			t.Errorf("%s: ExemptTools = %v, want empty", name, cfg.ExemptTools)
		}
	}
}

// proto3 optional 三态：nil=unset（默认 ON）；显式 false=kill switch；显式 true=ON。
func TestToolResultPruneConfig_EnabledTriState(t *testing.T) {
	unset := &Runtime{ToolResultPrune: &Runtime_ToolResultPrune{}}
	if cfg := unset.ToolResultPruneConfig(); !cfg.Enabled {
		t.Error("enabled unset (nil) must default to true")
	}
	off := false
	killed := &Runtime{ToolResultPrune: &Runtime_ToolResultPrune{Enabled: &off}}
	if cfg := killed.ToolResultPruneConfig(); cfg.Enabled {
		t.Error("enabled=false must act as kill switch")
	}
	on := true
	explicit := &Runtime{ToolResultPrune: &Runtime_ToolResultPrune{Enabled: &on}}
	if cfg := explicit.ToolResultPruneConfig(); !cfg.Enabled {
		t.Error("enabled=true must stay on")
	}
}

func TestToolResultPruneConfig_OverridesAndExemptTools(t *testing.T) {
	r := &Runtime{ToolResultPrune: &Runtime_ToolResultPrune{
		AfterTurns:  3,
		SizeBytes:   8192,
		ExemptTools: []string{" gns3_exec ", "", "read_tool_result"},
	}}
	cfg := r.ToolResultPruneConfig()
	if cfg.AfterTurns != 3 || cfg.SizeBytes != 8192 {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if !cfg.ExemptTools["gns3_exec"] || !cfg.ExemptTools["read_tool_result"] {
		t.Errorf("exempt tools must be trimmed and mapped: %v", cfg.ExemptTools)
	}
	if len(cfg.ExemptTools) != 2 {
		t.Errorf("blank entries must be dropped: %v", cfg.ExemptTools)
	}
}
