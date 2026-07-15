package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

func TestResolveAgentFilesystemDir_IncludesWorkspaceID(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	t.Setenv("WORKSPACE_ROOT", "")

	ctx := workspace.WithContext(context.Background(), "ws-tenant-a")
	ag := biz.Agent{ID: "a1", AgentKey: "coder"}
	deps := TRPCBuilderDeps{}
	deps.LG = loggateway.NewNoop()

	got, err := resolveAgentFilesystemDir(ctx, ag, deps, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := filepath.Join(base, "workspace", "ws-tenant-a", "coder")
	wantAbs, _ := filepath.Abs(want)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Fatalf("got %q, want %q", gotAbs, wantAbs)
	}
	if st, err := os.Stat(got); err != nil || !st.IsDir() {
		t.Fatalf("safe root not created: %v", err)
	}
}

func TestResolveAgentFilesystemDir_DefaultWorkspaceWhenMissing(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	t.Setenv("WORKSPACE_ROOT", "")

	ag := biz.Agent{ID: "a1", AgentKey: "bot"}
	deps := TRPCBuilderDeps{}
	deps.LG = loggateway.NewNoop()

	got, err := resolveAgentFilesystemDir(context.Background(), ag, deps, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := filepath.Join(base, "workspace", workspace.DefaultWorkspaceID, "bot")
	wantAbs, _ := filepath.Abs(want)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Fatalf("got %q, want %q (default workspace)", gotAbs, wantAbs)
	}
}

func TestResolveAgentFilesystemDir_RejectsHostAbsOutsideRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	t.Setenv("WORKSPACE_ROOT", "")

	outside := t.TempDir() // host abs path outside tenant root
	ctx := workspace.WithContext(context.Background(), "ws-a")
	ag := biz.Agent{ID: "a1", AgentKey: "agent-x"}
	deps := TRPCBuilderDeps{}
	deps.LG = loggateway.NewNoop()

	got, err := resolveAgentFilesystemDir(ctx, ag, deps, outside)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := filepath.Join(base, "workspace", "ws-a", "agent-x")
	wantAbs, _ := filepath.Abs(want)
	gotAbs, _ := filepath.Abs(got)
	if gotAbs != wantAbs {
		t.Fatalf("expected fallback to safe root %q, got %q", wantAbs, gotAbs)
	}
	if strings.HasPrefix(gotAbs, filepath.Clean(outside)) {
		t.Fatalf("must not accept host path outside root: %q", gotAbs)
	}
}

func TestResolveAgentFilesystemDir_AcceptsPathUnderTenantRoot(t *testing.T) {
	base := t.TempDir()
	t.Setenv("ARANEA_WORKSPACE_ROOT", base)
	t.Setenv("WORKSPACE_ROOT", "")

	ctx := workspace.WithContext(context.Background(), "ws-b")
	ag := biz.Agent{ID: "a1", AgentKey: "writer"}
	deps := TRPCBuilderDeps{}
	deps.LG = loggateway.NewNoop()

	subdir := filepath.Join(base, "workspace", "ws-b", "custom-subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got, err := resolveAgentFilesystemDir(ctx, ag, deps, subdir)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	gotAbs, _ := filepath.Abs(got)
	wantAbs, _ := filepath.Abs(subdir)
	if gotAbs != wantAbs {
		t.Fatalf("got %q, want contained path %q", gotAbs, wantAbs)
	}
}

func TestContainPathUnderRoot_RejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := containPathUnderRoot(outside, root); err == nil {
		t.Fatal("expected rejection for path outside root")
	}
}

func TestEnsureFilesystemWorkspaceDir_EmptyRejected(t *testing.T) {
	if err := ensureFilesystemWorkspaceDir(""); err == nil {
		t.Fatal("expected error for empty dir")
	}
	if err := ensureFilesystemWorkspaceDir("   "); err == nil {
		t.Fatal("expected error for whitespace dir")
	}
}
