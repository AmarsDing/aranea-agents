package data

import (
	"context"
	"strings"
	"testing"

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
