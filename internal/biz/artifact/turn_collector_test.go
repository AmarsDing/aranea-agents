package artifact_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/artifact"
)

func TestTurnCollector_AddAndRefs(t *testing.T) {
	ctx, c := artifact.WithTurnCollector(context.Background())
	if artifact.CollectorFromContext(ctx) != c {
		t.Fatal("collector not in context")
	}
	c.Add(artifact.Artifact{ID: "a1", Name: "out.csv", MimeType: "text/csv", Size: 12})
	refs := c.Refs()
	if len(refs) != 1 || refs[0].ID != "a1" {
		t.Fatalf("refs=%v", refs)
	}
}
