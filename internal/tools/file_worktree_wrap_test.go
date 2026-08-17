package tools

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
)

func TestWrapFileToolSetWithWorktree_NotGitPassthrough(t *testing.T) {
	dir := t.TempDir()
	ts, err := trpcfile.NewToolSet(trpcfile.WithBaseDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()
	if got := wrapFileToolSetWithWorktree(ts, dir, loggateway.NewNoop()); got != ts {
		t.Fatal("non-git workspace must pass through the live file ToolSet")
	}
}

func TestWrapFileToolSetWithWorktree_WritesAndMerges(t *testing.T) {
	repoRoot, cleanup := initTempGitRepo(t)
	defer cleanup()
	ts, err := trpcfile.NewToolSet(trpcfile.WithBaseDir(repoRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()
	wrapped := wrapFileToolSetWithWorktree(ts, repoRoot, loggateway.NewNoop())
	if wrapped == ts {
		t.Fatal("git workspace should wrap file writes with worktree isolator")
	}
	var save trpctool.CallableTool
	for _, tool := range wrapped.Tools(context.Background()) {
		ct, ok := tool.(trpctool.CallableTool)
		if !ok || ct.Declaration() == nil || ct.Declaration().Name != "save_file" {
			continue
		}
		save = ct
		break
	}
	if save == nil {
		t.Fatal("save_file not found")
	}
	if _, err := save.Call(context.Background(), []byte(`{"file_name":"from-wrap.txt","contents":"hi","overwrite":true}`)); err != nil {
		t.Fatalf("save_file: %v", err)
	}
	if !fileExistsInRepo(t, repoRoot, "from-wrap.txt") {
		t.Fatal("expected from-wrap.txt in main repo after worktree merge")
	}
}

func TestLookupGitRoot_Empty(t *testing.T) {
	if got := LookupGitRoot(""); got != "" {
		t.Fatalf("empty dir: %q", got)
	}
	if got := LookupGitRoot(t.TempDir()); got != "" {
		t.Fatalf("non-git dir: %q", got)
	}
}
