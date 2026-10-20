package knowledge

import (
	"strings"
	"testing"
)

// 编码器失败注入：encodeTokens 可交换，验证两条降级路径与恢复。
// 注：该用例串行运行（包级变量交换）；其余 t.Parallel 用例与串行用例天然不重叠。

func TestEncodeTokensRoundTrip(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"hello world, this is plain English text.",
		"值夜班必须有两个人同时在场，禁止单人值守核心机房。",
		"中英 mixed 文本 with标点，《引号》与 emoji 🚀 混排 tail",
		"   leading and trailing   spaces  \n\t plus newlines",
	} {
		toks := encodeTokens(text)
		if toks == nil {
			t.Fatalf("encodeTokens returned nil for %q", text)
		}
		if strings.Join(toks, "") != text {
			t.Fatalf("token concat must equal original text\nwant %q\n got %q", text, strings.Join(toks, ""))
		}
		if tokenOffsets(text, toks) == nil {
			t.Fatalf("tokenOffsets must accept lib output for %q", text)
		}
	}
}

func TestTokenOffsetsRejectsDrift(t *testing.T) {
	t.Parallel()
	if tokenOffsets("abc", []string{"ab", "cd"}) != nil {
		t.Fatal("drifted token strings must be rejected")
	}
	if tokenOffsets("abc", nil) != nil {
		t.Fatal("nil tokens must be rejected")
	}
}

func TestCountTokens(t *testing.T) {
	t.Parallel()
	if n := CountTokens("hello world"); n != 2 {
		t.Fatalf("cl100k 'hello world' must be 2 tokens, got %d", n)
	}
	if n := CountTokens(""); n != 0 {
		t.Fatalf("empty must be 0 tokens, got %d", n)
	}
	cjk := strings.Repeat("值", 100)
	if n := CountTokens(cjk); n == 0 || n > 100 {
		t.Fatalf("CJK token count must be within (0, runeLen], got %d", n)
	}
	// token 策略的核心回归点：CJK 长无空格段绝不能塌缩成 1 个「词」。
	long := strings.Repeat("中文段落连续书写没有任何空格间隔", 20)
	if n := CountTokens(long); n < 50 {
		t.Fatalf("long CJK run must yield many tokens, got %d", n)
	}
}

func TestEncodeFailureFallsBack(t *testing.T) {
	orig := encodeTokens
	encodeTokens = func(string) []string { return nil }
	defer func() { encodeTokens = orig }()

	cjk := strings.Repeat("中文", 300)
	if n := CountTokens(cjk); n != 600 {
		t.Fatalf("fallback count must be rune count 600, got %d", n)
	}
	// splitByToken 降级 rune 窗口：仍产出多块、ParentIndex 归一 -1。
	chunks, err := SplitWithStrategy(ChunkByToken, cjk, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatalf("rune fallback must still split long text, got %d chunks", len(chunks))
	}
	for _, ch := range chunks {
		if ch.ParentIndex != -1 || ch.IsParent {
			t.Fatalf("flat fallback chunk must have ParentIndex=-1, got %+v", ch)
		}
	}
	// SplitParentChild 降级普通 token 分块：无父块行。
	pc := SplitParentChild(cjk, 100, 10, 0)
	for _, ch := range pc {
		if ch.IsParent {
			t.Fatal("encoder failure must degrade parent_child to flat token split")
		}
	}
}
