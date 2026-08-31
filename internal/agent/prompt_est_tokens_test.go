package agent

import (
	"strings"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestPromptEstTokens_LatinKeepsBlended(t *testing.T) {
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: strings.Repeat("a", 1000)}}
	blended := analyzePromptRequest(msgs).EstTokens
	got := promptEstTokens(msgs)
	if got != blended {
		t.Fatalf("latin promptEstTokens=%d, want blended=%d", got, blended)
	}
}

func TestPromptEstTokens_CJKFloorExceedsBlended(t *testing.T) {
	const n = 1000
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: strings.Repeat("测", n)}}
	blended := analyzePromptRequest(msgs).EstTokens
	got := promptEstTokens(msgs)
	if blended >= n {
		t.Fatalf("precondition: blended %d should under-count %d CJK runes", blended, n)
	}
	if got != n {
		t.Fatalf("CJK promptEstTokens=%d, want floor=%d (blended=%d)", got, n, blended)
	}
}

func TestMessageEstTokens_CJKFloor(t *testing.T) {
	m := trpcmodel.Message{Role: trpcmodel.RoleTool, Content: strings.Repeat("故", 250)}
	if got := messageEstTokens(m); got != 250 {
		t.Fatalf("messageEstTokens CJK=%d, want 250", got)
	}
}
