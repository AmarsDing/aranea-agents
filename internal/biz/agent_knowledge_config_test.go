package biz

import (
	"strings"
	"testing"
)

func TestParseAgentKnowledgeConfig(t *testing.T) {
	t.Parallel()
	if ParseAgentKnowledgeConfig("").GroundedOnly {
		t.Fatal("empty config should be off")
	}
	if ParseAgentKnowledgeConfig(`{"knowledge":{"grounded_only":false}}`).GroundedOnly {
		t.Fatal("explicit false")
	}
	if !ParseAgentKnowledgeConfig(`{"knowledge":{"grounded_only":true}}`).GroundedOnly {
		t.Fatal("want grounded_only")
	}
	if ParseAgentKnowledgeConfig(`{"evaluation":{"auto_after_turn":true}}`).GroundedOnly {
		t.Fatal("missing knowledge key")
	}
}

func TestExtractConfigJSONKeys(t *testing.T) {
	t.Parallel()
	raw := `{"tools":{"enabled":true},"evaluation":{"auto_after_turn":true},"knowledge":{"grounded_only":true}}`
	got := extractConfigJSONKeys(raw, "evaluation", "knowledge")
	if !ParseAgentEvalAutoConfig(got).Enabled {
		t.Fatalf("evaluation lost: %s", got)
	}
	if !ParseAgentKnowledgeConfig(got).GroundedOnly {
		t.Fatalf("knowledge lost: %s", got)
	}
	if strings.Contains(got, `"tools"`) {
		t.Fatalf("tools must not persist in overlay: %s", got)
	}
}

func TestMergeConfigJSONKeys(t *testing.T) {
	t.Parallel()
	computed := `{"self_evolve":true}`
	legacy := `{"evaluation":{"auto_after_turn":true},"knowledge":{"grounded_only":true}}`
	got := mergeConfigJSONKeys(computed, legacy, nil, "evaluation", "knowledge")
	if !ParseAgentEvalAutoConfig(got).Enabled || !ParseAgentKnowledgeConfig(got).GroundedOnly {
		t.Fatalf("overlay missing: %s", got)
	}
	if !strings.Contains(got, `"self_evolve"`) {
		t.Fatalf("computed keys dropped: %s", got)
	}
}
