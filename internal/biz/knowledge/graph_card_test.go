package knowledge

import (
	"strings"
	"testing"
)

// G-33 卡片文构造（ADR-KN-20261020-G）：词条文一档（实体名片）、谓词文一档
// （active semantic 边陈述），为独立向量档的最小嵌入单元。

func TestEntityCardText(t *testing.T) {
	t.Run("name only", func(t *testing.T) {
		if got := EntityCardText("Claude Code", "", nil); got != "Claude Code" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("name + type", func(t *testing.T) {
		if got := EntityCardText("Claude Code", "AI Agent", nil); got != "Claude Code（AI Agent）" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("aliases sorted and deduped", func(t *testing.T) {
		// 升序钉："claude code cli"（space 0x20）< "claude-code"（0x2d）。
		got := EntityCardText("Claude Code", "AI Agent", []string{"claude-code", "claude code cli", "claude-code", "", "claude code cli"})
		want := "Claude Code（AI Agent）\n别名：claude code cli, claude-code"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("aliases without type", func(t *testing.T) {
		got := EntityCardText("pgvector", "", []string{"vector ext"})
		want := "pgvector\n别名：vector ext"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("empty aliases produce no alias line", func(t *testing.T) {
		if got := EntityCardText("pgvector", "", []string{"", ""}); got != "pgvector" {
			t.Fatalf("got %q", got)
		}
	})
}

func TestEdgeCardText(t *testing.T) {
	t.Run("spo + evidence", func(t *testing.T) {
		got := EdgeCardText("PostgreSQL", "depends-on", "pgvector", "向量索引依赖 pgvector 扩展")
		want := "PostgreSQL depends-on pgvector — 向量索引依赖 pgvector 扩展"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("empty evidence drops separator", func(t *testing.T) {
		got := EdgeCardText("PostgreSQL", "depends-on", "pgvector", "")
		want := "PostgreSQL depends-on pgvector"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("240 rune boundary preserves SPO and cuts evidence tail", func(t *testing.T) {
		// 主谓宾 20 runes + 长证据：主谓宾不得被截，证据尾部截断并带省略号
		// （沿用 relationContextMaxRunes=240 截断纪律）。
		evidence := strings.Repeat("证", 300)
		got := EdgeCardText("PostgreSQL", "depends-on", "pgvector", evidence)
		if !strings.HasPrefix(got, "PostgreSQL depends-on pgvector — ") {
			t.Fatalf("SPO head must be preserved, got %q", got[:60])
		}
		runes := []rune(got)
		if len(runes) > GraphCardTextMaxRunes+1 { // 240 + 省略号
			t.Fatalf("card too long: %d runes", len(runes))
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("truncated card must carry ellipsis, got tail %q", string(runes[len(runes)-4:]))
		}
	})

	t.Run("evidence verbatim: subject rename never rewrites evidence body", func(t *testing.T) {
		// 边 context 内旧名不漂白（重写纪律只服 entity 轨）——构造器对 evidence
		// 只做截断，不做任何实体名替换/解析。
		evidence := "旧系统 Aranea 1.0 依赖 MySQL（Aranea 1.0 为上线时快照名）"
		got := EdgeCardText("Aranea", "depends-on", "MySQL", evidence)
		if !strings.Contains(got, evidence) {
			t.Fatalf("evidence must pass through verbatim, got %q", got)
		}
	})
}

func TestGraphCardSourceHash(t *testing.T) {
	t.Run("sha1 hex pinned", func(t *testing.T) {
		// sha1("abc")——漂移对账键格式钉（40 位小写 hex）。
		if got := GraphCardSourceHash("abc"); got != "a9993e364706816aba3e25717850c26c9cd0d89d" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("text change flips hash", func(t *testing.T) {
		if GraphCardSourceHash("abc") == GraphCardSourceHash("abd") {
			t.Fatal("hash must change with text")
		}
	})
}
