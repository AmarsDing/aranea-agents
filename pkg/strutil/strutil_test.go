package strutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunes_utf8Safe(t *testing.T) {
	s := strings.Repeat("中", 100)
	got := TruncateRunes(s, 50)
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8 after truncate")
	}
	if len([]rune(got)) != 50 {
		t.Fatalf("want 50 runes, got %d", len([]rune(got)))
	}
	// Byte slice at 200 would break a 3-byte rune (legacy bug).
	broken := s[:200]
	if utf8.ValidString(broken) {
		t.Fatal("expected broken legacy slice for this sample")
	}
}

func TestValidUTF8(t *testing.T) {
	s := strings.Repeat("中", 66) + string([]byte{0xff})
	got := ValidUTF8(s)
	if !utf8.ValidString(got) {
		t.Fatal("expected valid utf-8")
	}
}

func TestProtoPreview(t *testing.T) {
	got := ProtoPreview(strings.Repeat("文", 300), 200)
	if len([]rune(got)) != 200 {
		t.Fatalf("want 200 runes, got %d", len([]rune(got)))
	}
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8")
	}
}

func TestTruncateRunesEllipsis(t *testing.T) {
	// CJK 超上限：截断 + 省略号，rune 安全。
	got := TruncateRunesEllipsis(strings.Repeat("档", 1300), 1200)
	if r := len([]rune(got)); r != 1201 || !strings.HasSuffix(got, "…") {
		t.Fatalf("want 1200 runes + ellipsis, got %d runes", r)
	}
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8")
	}
	// 未超限原样返回（同一字符串，无标记）。
	short := "短串"
	if got := TruncateRunesEllipsis(short, 10); got != short {
		t.Fatalf("short string must pass through, got %q", got)
	}
	// 边界：恰好等于上限不加标记。
	exact := strings.Repeat("a", 10)
	if got := TruncateRunesEllipsis(exact, 10); got != exact {
		t.Fatalf("exact-fit must pass through, got %q", got)
	}
	// 非正上限返回空（与 TruncateRunes 一致）。
	if got := TruncateRunesEllipsis("abc", 0); got != "" {
		t.Fatalf("maxRunes<=0 must return empty, got %q", got)
	}
}

func TestTruncateBytesRuneSafe(t *testing.T) {
	// 3 字节 CJK：字节上限落在 rune 中间时回退到边界。
	s := strings.Repeat("中", 100) // 300 bytes
	got := TruncateBytesRuneSafe(s, 200)
	if !utf8.ValidString(got) {
		t.Fatal("invalid utf-8 after byte-cap truncation")
	}
	if len(got) != 198 { // 66 runes × 3B
		t.Fatalf("want 198 bytes (rune boundary backoff), got %d", len(got))
	}
	// 未超限原样返回。
	if got := TruncateBytesRuneSafe("abc", 10); got != "abc" {
		t.Fatalf("short string must pass through, got %q", got)
	}
	// 非正上限返回空。
	if got := TruncateBytesRuneSafe("abc", 0); got != "" {
		t.Fatalf("maxBytes<=0 must return empty, got %q", got)
	}
}

func TestSliceBytesRuneSafe(t *testing.T) {
	cjk := []byte(strings.Repeat("中", 100)) // 300 bytes

	// tail（保留头部）：200 落在 rune 中间，回退到 198。
	got := SliceBytesRuneSafe(cjk, 200, "tail")
	if !utf8.Valid(got) || len(got) != 198 {
		t.Fatalf("tail: want 198 valid bytes, got %d (valid=%v)", len(got), utf8.Valid(got))
	}
	// head（保留尾部）：起点回退方向相反——向前到下一个 rune 边界。
	got = SliceBytesRuneSafe(cjk, 200, "head")
	if !utf8.Valid(got) || len(got) != 198 {
		t.Fatalf("head: want 198 valid bytes, got %d (valid=%v)", len(got), utf8.Valid(got))
	}
	if string(got) != strings.Repeat("中", 66) {
		t.Fatal("head: content mismatch")
	}
	// middle：两端各自回退，结果必须合法。
	got = SliceBytesRuneSafe(cjk, 201, "middle")
	if !utf8.Valid(got) {
		t.Fatalf("middle: invalid utf-8 (len=%d)", len(got))
	}
	// target 覆盖全长：原样返回。
	if got := SliceBytesRuneSafe(cjk, 400, "tail"); len(got) != 300 {
		t.Fatalf("oversized target must return input, got %d bytes", len(got))
	}
}
