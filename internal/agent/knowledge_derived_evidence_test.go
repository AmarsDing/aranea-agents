package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestFormatKnowledgeCue_LabelsGraphCardsAsDerivedEvidence(t *testing.T) {
	chunks := []biz.KnowledgeChunk{{ID: "g33r/11/doc-1", DocID: "doc-1", Content: "平台 depends-on 数据库", Score: 0.6}}
	cue, cited := formatKnowledgeCue(nil, chunks, false, false, false, false)
	if !strings.Contains(cue, "evidence=derived graph evidence") || !strings.Contains(cue, "not verbatim source passages") || len(cited) != 1 {
		t.Fatalf("graph evidence must not masquerade as source passages: cue=%s cited=%v", cue, cited)
	}
	chunks[0].ID = "real-chunk-1"
	cue, _ = formatKnowledgeCue(nil, chunks, false, false, false, false)
	if strings.Contains(cue, "derived graph evidence") {
		t.Fatal("ordinary source chunk was relabeled as derived")
	}
}
