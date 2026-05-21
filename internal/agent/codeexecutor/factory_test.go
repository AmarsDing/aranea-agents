package codeexecutor_test

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/codeexecutor"
)

func TestFactoryResolveLocalDefault(t *testing.T) {
	f := codeexecutor.NewFactory()
	exec := f.Resolve(context.Background(), "", t.TempDir())
	if exec == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestFactoryRegisteredTypes(t *testing.T) {
	f := codeexecutor.NewFactory()
	types := f.RegisteredTypes()
	if len(types) < 1 {
		t.Fatalf("expected at least local, got %v", types)
	}
	foundLocal := false
	for _, typ := range types {
		if typ == codeexecutor.TypeLocal {
			foundLocal = true
		}
	}
	if !foundLocal {
		t.Fatalf("expected local in %v", types)
	}
}

func TestFactoryDockerFallbackWhenUnavailable(t *testing.T) {
	codeexecutor.ResetDockerProbe()
	f := codeexecutor.NewFactory()
	exec := f.Resolve(context.Background(), codeexecutor.TypeDocker, t.TempDir())
	if exec == nil {
		t.Fatal("expected fallback executor")
	}
}

func TestFactoryCapabilitiesAlwaysIncludesLocal(t *testing.T) {
	f := codeexecutor.NewFactory()
	caps := f.Capabilities()
	if len(caps) != len(codeexecutor.ValidTypes()) {
		t.Fatalf("expected %d capabilities, got %d", len(codeexecutor.ValidTypes()), len(caps))
	}
}
