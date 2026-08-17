package knowledge

import (
	"testing"
)

func TestLoadRetrievalGoldV1_RewriteRobustnessSize(t *testing.T) {
	cases, texts, err := LoadRetrievalGoldV1()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 300 {
		t.Fatalf("gold v1 has %d cases, want at least 300 rewrite-robust queries", len(cases))
	}
	ranked := 0
	abstain := 0
	for _, gold := range cases {
		query := texts[gold.ID]
		if query == "" {
			t.Fatalf("missing query text for %s", gold.ID)
		}
		if gold.Abstain {
			abstain++
			if len(gold.RelevantDocIDs) != 0 {
				t.Fatalf("abstain case %s must not have relevant docs", gold.ID)
			}
			continue
		}
		if len(gold.RelevantDocIDs) == 0 {
			t.Fatalf("ranked case %s missing relevant doc", gold.ID)
		}
		ranked++
	}
	if ranked < 300 {
		t.Fatalf("ranked cases = %d, want at least 300", ranked)
	}
	if abstain < 5 {
		t.Fatalf("abstain cases = %d, want at least 5", abstain)
	}
}
