package watch

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestSlugFromEvent_Basic(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/my-skill/SKILL.md")
	if slug != "my-skill" {
		t.Fatalf("expected my-skill, got %q", slug)
	}
}

func TestSlugFromEvent_RootPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills")
	if slug != "" {
		t.Fatalf("expected empty for root path, got %q", slug)
	}
}

func TestSlugFromEvent_DotDir(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/.hidden/file.md")
	if slug != "" {
		t.Fatalf("expected empty for dot dir, got %q", slug)
	}
}

func TestSlugFromEvent_DeepPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/data/skills/my-skill/sub/file.md")
	if slug != "my-skill" {
		t.Fatalf("expected my-skill, got %q", slug)
	}
}

func TestSlugFromEvent_UnrelatedPath(t *testing.T) {
	slug := slugFromEvent("/data/skills", "/other/path/file.md")
	if slug != "" {
		t.Fatalf("expected empty for unrelated path, got %q", slug)
	}
}

func TestNewMonitorSyncReporter_BothNil(t *testing.T) {
	if NewMonitorSyncReporter(nil, nil, nil) != nil {
		t.Fatal("should return nil when both args are nil")
	}
}

func TestSetSyncReporter_NilRunner(t *testing.T) {
	SetSyncReporter(nil, nil)
}

func TestSetAlertEvaluator_NilRunner(t *testing.T) {
	SetAlertEvaluator(nil, nil)
}

func TestNewRunner(t *testing.T) {
	r := NewRunner(nil, nil, nil, loggateway.NewNoop())
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

func TestNewRunnerWithBus(t *testing.T) {
	r := NewRunnerWithBus(nil, nil, nil, nil, loggateway.NewNoop())
	if r == nil {
		t.Fatal("expected non-nil runner")
	}
}

func TestRunner_Start_NilRunner(t *testing.T) {
	var r *Runner
	r.Start(context.Background())
}

func TestRunner_reportSync_NilRunner(t *testing.T) {
	var r *Runner
	r.reportSync(context.Background(), "test", "slug", "msg", "info")
}

func TestRunner_reportSync_NilReporter(t *testing.T) {
	r := NewRunner(nil, nil, nil, loggateway.NewNoop())
	r.reportSync(context.Background(), "test", "slug", "msg", "info")
}
