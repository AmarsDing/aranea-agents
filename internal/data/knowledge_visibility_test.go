package data

import (
	"context"
	"strings"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/auth"
)

func TestDocVisibilityClauses(t *testing.T) {
	t.Parallel()

	sys := workspace.WithSystemWorkspace(context.Background())
	if c, a := docRowVisibilityClause(sys, 2); c != "" || a != nil {
		t.Fatalf("system row clause = %q %v", c, a)
	}
	if c, a := docChunkVisibilityClause(sys, "$2", 3); c != "" || a != nil {
		t.Fatalf("system chunk clause = %q %v", c, a)
	}

	anonRow, anonArgs := docRowVisibilityClause(context.Background(), 2)
	if !strings.Contains(anonRow, "'collection'") || anonArgs != nil {
		t.Fatalf("anon row = %q %v", anonRow, anonArgs)
	}
	anonChunk, anonChunkArgs := docChunkVisibilityClause(context.Background(), "$2", 3)
	if !strings.Contains(anonChunk, "collection_id = $2") || !strings.Contains(anonChunk, "'collection'") || anonChunkArgs != nil {
		t.Fatalf("anon chunk = %q %v", anonChunk, anonChunkArgs)
	}

	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	row, args := docRowVisibilityClause(ctx, 4)
	if !strings.Contains(row, "owner_user_id = $4") || len(args) != 1 || args[0] != "7" {
		t.Fatalf("user row = %q %v", row, args)
	}
	chunk, chunkArgs := docChunkVisibilityClause(ctx, "$2", 5)
	if !strings.Contains(chunk, "owner_user_id = $5") || len(chunkArgs) != 1 || chunkArgs[0] != "7" {
		t.Fatalf("user chunk = %q %v", chunk, chunkArgs)
	}

	both, bothArgs := docBothEndpointsVisibleClause(ctx, "$1", 3)
	if !strings.Contains(both, "doc_id IN") || !strings.Contains(both, "target_doc_id IN") ||
		!strings.Contains(both, "owner_user_id = $3") || len(bothArgs) != 1 || bothArgs[0] != "7" {
		t.Fatalf("both endpoints = %q %v", both, bothArgs)
	}
	if c, a := docBothEndpointsVisibleClause(sys, "$1", 3); c != "" || a != nil {
		t.Fatalf("system both endpoints = %q %v", c, a)
	}
}

// G-08 shared 腿：org scope（专项 Agent，无 auth uid）/ uid+scope（两腿并存）/
// 空 scope（不生成 shared 腿，现状语义不变）。
func TestDocVisibilityClauses_SharedScope(t *testing.T) {
	t.Parallel()

	// 仅 scope（Agent org ctx，无 auth uid）：collection OR shared-overlap。
	scoped := bizknowledge.WithOrgScope(context.Background(), []string{"dept-a", "pos-1"})
	row, args := docRowVisibilityClause(scoped, 4)
	if !strings.Contains(row, "'shared'") || !strings.Contains(row, "shared_with && $4::text[]") || strings.Contains(row, "owner_user_id") {
		t.Fatalf("scope-only row = %q", row)
	}
	if len(args) != 1 {
		t.Fatalf("scope-only args must be the single scope array, got %v", args)
	}

	// uid + scope：owner 腿在 $4、shared 腿在 $5，参数顺序 [uid, scope]。
	bothCtx := bizknowledge.WithOrgScope(auth.NewContext(context.Background(), &auth.Auth{UserID: 7}), []string{"dept-a"})
	row, args = docRowVisibilityClause(bothCtx, 4)
	if !strings.Contains(row, "owner_user_id = $4") || !strings.Contains(row, "shared_with && $5::text[]") {
		t.Fatalf("uid+scope row = %q", row)
	}
	if len(args) != 2 || args[0] != "7" {
		t.Fatalf("uid+scope args = %v", args)
	}

	// 空 scope 回落现状（不生成 shared 腿、参数仅 uid）。
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	row, args = docRowVisibilityClause(ctx, 4)
	if strings.Contains(row, "shared") || len(args) != 1 || args[0] != "7" {
		t.Fatalf("no-scope row must keep old shape: %q %v", row, args)
	}

	// chunk 子查询同样继承 shared 腿。
	chunk, chunkArgs := docChunkVisibilityClause(scoped, "$2", 5)
	if !strings.Contains(chunk, "collection_id = $2") || !strings.Contains(chunk, "shared_with && $5::text[]") || len(chunkArgs) != 1 {
		t.Fatalf("scope chunk = %q %v", chunk, chunkArgs)
	}
	// 双端点谓词同一 $n 复用（两端共用一份 scope 参数）。
	both, bothArgs := docBothEndpointsVisibleClause(scoped, "$1", 3)
	if !strings.Contains(both, "doc_id IN") || !strings.Contains(both, "target_doc_id IN") ||
		!strings.Contains(both, "shared_with && $3::text[]") || len(bothArgs) != 1 {
		t.Fatalf("scope both endpoints = %q %v", both, bothArgs)
	}
	// system 仍整体旁路。
	sys := workspace.WithSystemWorkspace(context.Background())
	if c, a := docRowVisibilityClause(bizknowledge.WithOrgScope(sys, []string{"dept-a"}), 2); c != "" || a != nil {
		t.Fatalf("system with scope must bypass: %q %v", c, a)
	}
}
