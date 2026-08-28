package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestSubTasksFromSubIntents_RequiresTwoGoals(t *testing.T) {
	if got := subTasksFromSubIntents(nil); got != nil {
		t.Fatalf("nil artifact: %+v", got)
	}
	if got := subTasksFromSubIntents(&biz.IntentArtifact{
		SubIntents: []biz.SubIntent{{Goal: "only one"}},
	}); got != nil {
		t.Fatalf("single intent must stay nil: %+v", got)
	}
	got := subTasksFromSubIntents(&biz.IntentArtifact{
		SubIntents: []biz.SubIntent{
			{Goal: "查数据", IntentKind: "research"},
			{Goal: "写成邮件", IntentKind: "task"},
		},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].DomainPath != "研究/调研" {
		t.Fatalf("research domain=%q", got[0].DomainPath)
	}
	if len(got[1].DependsOn) != 1 || got[1].DependsOn[0] != got[0].ID {
		t.Fatalf("depends=%v", got[1].DependsOn)
	}
}
