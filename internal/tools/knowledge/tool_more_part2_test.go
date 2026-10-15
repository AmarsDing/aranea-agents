package knowledge

import (
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"testing"
)

func TestReflectTool_WithRetriever_MultiCollection_Error(t *testing.T) {
	ret := knowledge.NewRetriever(&mockQueryEmbedder{}, &mockKnowledgeRepo{}, nil, loggateway.NewNoop())

	tool := NewReflectTool(nil)
	ctx := WithRetriever(context.Background(), ret)

	args, _ := json.Marshal(reflectInput{
		CollectionIDs: []string{"col-1", "col-2"},
		Query:         "test",
	})

	_, err := tool.Call(ctx, args)
	if err == nil {
		t.Fatal("expected error for multi-collection without federated retriever")
	}
}
