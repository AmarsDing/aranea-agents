package codeexecutor_test

import (
	"context"
	"testing"

	"aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/pkg/loggateway"
)

func newTestFactory() *codeexecutor.Factory {
	return codeexecutor.NewFactoryWithLogger(loggateway.NewNoop())
}

func TestFactoryResolveLocalDefault(t *testing.T) {
	f := newTestFactory()
	exec := f.Resolve(context.Background(), "", t.TempDir())
	if exec == nil {
		t.Fatal("expected non-nil executor")
	}
}

func TestFactoryRegisteredTypes(t *testing.T) {
	f := newTestFactory()
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
	f := newTestFactory()
	exec := f.Resolve(context.Background(), codeexecutor.TypeDocker, t.TempDir())
	if exec == nil {
		t.Fatal("expected fallback executor")
	}
}

func TestFactoryCapabilitiesAlwaysIncludesLocal(t *testing.T) {
	f := newTestFactory()
	caps := f.Capabilities()
	if len(caps) != len(codeexecutor.ValidTypes()) {
		t.Fatalf("expected %d capabilities, got %d", len(codeexecutor.ValidTypes()), len(caps))
	}
}
