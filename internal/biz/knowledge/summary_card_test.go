package knowledge

import (
	"strings"
	"testing"
)

func TestDeriveSummaryCard(t *testing.T) {
	t.Run("heading and path type", func(t *testing.T) {
		sum, typ, hash := DeriveSummaryCard("# 退款流程\n\n用户申请后三个工作日到账。", "faq/refund.md", "refund.md")
		if sum != "退款流程" {
			t.Fatalf("summary = %q", sum)
		}
		if typ != "faq" {
			t.Fatalf("type = %q", typ)
		}
		if hash == "" || hash != HashContent("# 退款流程\n\n用户申请后三个工作日到账。") {
			t.Fatalf("hash = %q", hash)
		}
	})

	t.Run("skips frontmatter and clips", func(t *testing.T) {
		body := "---\ntitle: x\n---\n\n" + strings.Repeat("字", 200)
		sum, typ, _ := DeriveSummaryCard(body, "notes/a.md", "a.md")
		if typ != "note" {
			t.Fatalf("type = %q", typ)
		}
		if []rune(sum)[len([]rune(sum))-1] != '…' {
			t.Fatalf("expected clip marker, got %q", sum)
		}
		if n := len([]rune(sum)); n != summaryCardMaxRunes+1 {
			t.Fatalf("rune count = %d", n)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		sum, typ, hash := DeriveSummaryCard("", "reports/q2.md", "q2.md")
		if sum != "" || hash != "" || typ != "report" {
			t.Fatalf("sum=%q typ=%q hash=%q", sum, typ, hash)
		}
	})
}
