package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

func TestTriggerKnowledgeGraph_ForwardsIngestedDocument(t *testing.T) {
	var gotCollection string
	var gotDocs []bizknowledge.PromoteTouchedDoc
	s := &KnowledgeService{
		lg: loggateway.NewNoop(),
		writeBackGraph: func(_ context.Context, col bizknowledge.Collection, docs []bizknowledge.PromoteTouchedDoc) error {
			gotCollection = col.ID
			gotDocs = append([]bizknowledge.PromoteTouchedDoc(nil), docs...)
			return nil
		},
	}
	s.triggerKnowledgeGraph(context.Background(), biz.KnowledgeCollection{ID: "team-col"}, []bizknowledge.PromoteTouchedDoc{
		{DocID: "uploaded-doc", Created: true},
	})
	if gotCollection != "team-col" || len(gotDocs) != 1 || gotDocs[0].DocID != "uploaded-doc" {
		t.Fatalf("graph hook got collection=%q docs=%+v", gotCollection, gotDocs)
	}
}
