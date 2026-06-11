package archlint

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// TestBizNotDependOnTrpcAgentGo verifies AS-FIT-01 P0 invariant:
// biz layer must NOT depend on pkg/trpc-agent-go.
// The trpc-agent-go module path is trpc.group/trpc-go/trpc-agent-go
// (vendored at ./pkg/trpc-agent-go via go.mod replace directive).
func TestBizNotDependOnTrpcAgentGo(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, "aranea-agents/internal/biz/...")
	if err != nil {
		t.Fatalf("failed to load biz packages: %v", err)
	}

	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			if strings.Contains(importPath, "trpc.group/trpc-go/trpc-agent-go") {
				t.Errorf("biz layer must not depend on pkg/trpc-agent-go: %s imports %s", pkg.PkgPath, importPath)
			}
		}
	}
}

// TestServiceNotDirectlyAccessData verifies AS-FIT-01 P0 invariant:
// service layer must NOT directly import the data layer.
// Service should go through biz (use cases) instead.
func TestServiceNotDirectlyAccessData(t *testing.T) {
	cfg := &packages.Config{
		Mode: packages.NeedImports | packages.NeedDeps,
	}
	pkgs, err := packages.Load(cfg, "aranea-agents/internal/service/...")
	if err != nil {
		t.Fatalf("failed to load service packages: %v", err)
	}

	for _, pkg := range pkgs {
		for importPath := range pkg.Imports {
			if strings.Contains(importPath, "aranea-agents/internal/data") {
				t.Errorf("service layer must not directly import data layer: %s imports %s", pkg.PkgPath, importPath)
			}
		}
	}
}
