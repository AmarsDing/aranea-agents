package acp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 评审 2026-11-09 D 项：stderrHint 输出必须是合法 UTF-8——返回串拼进
// Start 的 error，最终可能写入 PG TEXT 列（非法字节 = SQLSTATE 22021）。

func TestStderrHintGBKDecoded(t *testing.T) {
	// Windows 子进程 stderr 常见 GBK：0xC4 0xE3 = "你"，0xBA 0xC3 = "好"。
	gbk := string([]byte{'e', 'r', 'r', ' ', 0xc4, 0xe3, 0xba, 0xc3})
	got := stderrHint(gbk)
	if !utf8.ValidString(got) {
		t.Fatalf("输出必须是合法 UTF-8, got %q", got)
	}
	if !strings.Contains(got, "你好") {
		t.Fatalf("GBK 内容应被解码为中文, got %q", got)
	}
}

func TestStderrHintGarbageBytesSanitized(t *testing.T) {
	// 乱码字节：无论 GBK 解码还是 U+FFFD 兜底，输出必须是合法 UTF-8 且形状不变。
	for _, raw := range [][]byte{
		{0xff, 0xfe, 'x', 0x80, 0x81, 'y'},
		{0x81},             // 截断的 GBK 前导字节
		{'o', 'k', 0xc4},   // 尾部截断在 GBK 双字节中间
		{0xed, 0xa0, 0x80}, // UTF-8 非法代理区编码
	} {
		got := stderrHint(string(raw))
		if !utf8.ValidString(got) {
			t.Fatalf("输入 %x 的输出必须是合法 UTF-8, got %q", raw, got)
		}
		if !strings.HasPrefix(got, "（stderr 尾部: ") || !strings.HasSuffix(got, "）") {
			t.Fatalf("输入 %x 的形状不应变化, got %q", raw, got)
		}
	}
}

func TestStderrHintTruncatedGBKTailKeepsText(t *testing.T) {
	// GBK 文本尾部截断在双字节中间（stderr 缓冲区切断的常见形态）：
	// 解码带截断错误，仍应选解码结果保住主体文本而非整体退回替换符。
	raw := []byte{'e', 'r', 'r', ' ', 0xc4, 0xe3, 0xba, 0xc3, 0xc4} // "err 你好" + 半个字
	got := stderrHint(string(raw))
	if !utf8.ValidString(got) {
		t.Fatalf("输出必须是合法 UTF-8, got %q", got)
	}
	if !strings.Contains(got, "err 你好") {
		t.Fatalf("截断的 GBK 尾部应保住已解码文本, got %q", got)
	}
}

func TestStderrHintLongTailTruncatedAtRuneBoundary(t *testing.T) {
	// 2100 个三字节汉字（6300 字节）超上限：截断必须落在 rune 边界。
	got := stderrHint(strings.Repeat("界", 2100))
	if !utf8.ValidString(got) {
		t.Fatalf("截断后必须是合法 UTF-8")
	}
	if !strings.HasPrefix(got, "（stderr 尾部: …") {
		t.Fatalf("超长应带省略号前缀, got prefix %q", got[:24])
	}
	if !strings.HasSuffix(got, "）") {
		t.Fatal("形状必须保持以全角括号收尾")
	}
	if n := utf8.RuneCountInString(got); n > 2048+utf8.RuneCountInString("（stderr 尾部: …）") {
		t.Fatalf("截断后超长: %d rune", n)
	}
}

func TestStderrHintLongGBKTruncatedValid(t *testing.T) {
	// GBK 长串：2100 个 "你"（4200 字节）。解码→截断后仍合法、内容健在。
	var b []byte
	for i := 0; i < 2100; i++ {
		b = append(b, 0xc4, 0xe3)
	}
	got := stderrHint(string(b))
	if !utf8.ValidString(got) {
		t.Fatal("GBK 长串截断后必须是合法 UTF-8")
	}
	if !strings.Contains(got, "你") {
		t.Fatalf("GBK 内容应保留, got prefix %q", got[:24])
	}
	if !strings.HasPrefix(got, "（stderr 尾部: …") {
		t.Fatalf("超长应带省略号前缀, got prefix %q", got[:24])
	}
}

func TestStderrHintValidShapeUnchanged(t *testing.T) {
	// 已有安全串：形状逐字节不变。
	if got := stderrHint("boom"); got != "（stderr 尾部: boom）" {
		t.Fatalf("安全串形状不应变化, got %q", got)
	}
	if got := stderrHint(" line1 \n line2 "); got != "（stderr 尾部: line1 \n line2）" {
		t.Fatalf("安全串形状不应变化, got %q", got)
	}
	if got := stderrHint(""); got != "" {
		t.Fatalf("空串应返回空, got %q", got)
	}
	if got := stderrHint("  \n\t "); got != "" {
		t.Fatalf("纯空白应返回空, got %q", got)
	}
}
