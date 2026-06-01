package browser

import (
	"testing"
)

func TestBoolPtr(t *testing.T) {
	p := BoolPtr(true)
	if p == nil || *p != true {
		t.Fatal("expected *true")
	}
	p2 := BoolPtr(false)
	if p2 == nil || *p2 != false {
		t.Fatal("expected *false")
	}
}

func TestDefaultPlaywrightMCPConfig(t *testing.T) {
	cfg := DefaultPlaywrightMCPConfig()
	if cfg.Command != "npx" {
		t.Fatalf("command=%q", cfg.Command)
	}
	if cfg.Transport != "stdio" {
		t.Fatalf("transport=%q", cfg.Transport)
	}
	if cfg.Headless == nil || *cfg.Headless != true {
		t.Fatal("default headless should be true")
	}
	if cfg.Vision == nil || *cfg.Vision != false {
		t.Fatal("default vision should be false")
	}
	if cfg.Isolated == nil || *cfg.Isolated != true {
		t.Fatal("default isolated should be true")
	}
}

func TestEffectiveHeadless(t *testing.T) {
	tests := []struct {
		name string
		cfg  PlaywrightMCPConfig
		want bool
	}{
		{"nil defaults true", PlaywrightMCPConfig{}, true},
		{"explicit true", PlaywrightMCPConfig{Headless: BoolPtr(true)}, true},
		{"explicit false", PlaywrightMCPConfig{Headless: BoolPtr(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveHeadless(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveVision(t *testing.T) {
	tests := []struct {
		name string
		cfg  PlaywrightMCPConfig
		want bool
	}{
		{"nil defaults false", PlaywrightMCPConfig{}, false},
		{"explicit true", PlaywrightMCPConfig{Vision: BoolPtr(true)}, true},
		{"explicit false", PlaywrightMCPConfig{Vision: BoolPtr(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveVision(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEffectiveIsolated(t *testing.T) {
	tests := []struct {
		name string
		cfg  PlaywrightMCPConfig
		want bool
	}{
		{"nil defaults true", PlaywrightMCPConfig{}, true},
		{"explicit true", PlaywrightMCPConfig{Isolated: BoolPtr(true)}, true},
		{"explicit false", PlaywrightMCPConfig{Isolated: BoolPtr(false)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.EffectiveIsolated(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildArgs_Default(t *testing.T) {
	cfg := DefaultPlaywrightMCPConfig()
	args := cfg.BuildArgs()
	if len(args) < 3 {
		t.Fatalf("too few args: %v", args)
	}
	foundHeadless := false
	foundIsolated := false
	for _, a := range args {
		if a == "--headless" {
			foundHeadless = true
		}
		if a == "--isolated" {
			foundIsolated = true
		}
	}
	if !foundHeadless {
		t.Fatal("missing --headless in default config")
	}
	if !foundIsolated {
		t.Fatal("missing --isolated in default config")
	}
}

func TestBuildArgs_VisionEnabled(t *testing.T) {
	cfg := PlaywrightMCPConfig{
		Args:    []string{"--yes", "@playwright/mcp@latest"},
		Headless: BoolPtr(false),
		Vision:   BoolPtr(true),
		Isolated: BoolPtr(false),
	}
	args := cfg.BuildArgs()
	foundVision := false
	foundHeadless := false
	foundIsolated := false
	for _, a := range args {
		if a == "--headless" {
			foundHeadless = true
		}
		if a == "--isolated" {
			foundIsolated = true
		}
		if a == "vision" {
			foundVision = true
		}
	}
	if foundHeadless {
		t.Fatal("should not have --headless when Headless=false")
	}
	if foundIsolated {
		t.Fatal("should not have --isolated when Isolated=false")
	}
	if !foundVision {
		t.Fatal("missing --caps vision when Vision=true")
	}
}

func TestBuildArgs_EmptyBase(t *testing.T) {
	cfg := PlaywrightMCPConfig{}
	args := cfg.BuildArgs()
	if len(args) != 2 {
		t.Fatalf("expected 2 args (headless+isolated), got %v", args)
	}
}

func TestBuildArgs_AllDisabled(t *testing.T) {
	cfg := PlaywrightMCPConfig{
		Headless: BoolPtr(false),
		Vision:   BoolPtr(false),
		Isolated: BoolPtr(false),
	}
	args := cfg.BuildArgs()
	if len(args) != 0 {
		t.Fatalf("expected 0 args when all disabled, got %v", args)
	}
}
