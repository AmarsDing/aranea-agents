package fleet

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 2026-11-21 九家派活实测出的几处判定：截断的汉字、CodeBuddy 的权限菜单、豆包的初始化、TUI 框线。

func TestPreviewDropsCutRunes(t *testing.T) {
	full := []byte("正在写入文件\n完成")
	// 缓冲从第一个汉字的中间开始
	got := previewLines(full[1:], 5)
	if !utf8.ValidString(got) {
		t.Fatalf("preview not valid UTF-8: %q", got)
	}
	if !strings.Contains(got, "完成") {
		t.Fatalf("preview lost text: %q", got)
	}
}

func TestCodeBuddyMenuNeedsInput(t *testing.T) {
	menu := "Do you want to create aranea.txt?\n > 1. Yes\n   2. Yes, and don't ask again this session (shift + tab)\n   3. No, and tell CodeBuddy what to do differently (escape)"
	if !matchNeedsInput(strings.ToLower(menu)) {
		t.Fatal("CodeBuddy permission menu not recognised")
	}
	if matchNeedsInput("> fix the login page") {
		t.Fatal("a plain input prompt must not count as waiting")
	}
}

func TestTranscriptDropsProgressAndTips(t *testing.T) {
	// CodeBuddy 做完时屏幕的样子（2026-11-21 实测）
	screen := "✹ Waking… (2s · streaming · ↓ 2 tokens)\n● DONE\n✔ Worked for 12s\nThinking on (AltT to toggle)\nTip: Ask CodeBuddy to create a todo list\n↵ send"
	if got := transcriptTail(screen, 10); got != "● DONE" {
		t.Fatalf("transcriptTail = %q", got)
	}
}

func TestBridgeAnswerSkipsBannerAndMenu(t *testing.T) {
	b := &BridgeSession{}
	banner := "CodeBuddy Code v2.185.0\nTips for getting started\n> Create a file named a.txt"
	b.remember(banner)
	b.remember(banner + "\nDo you want to create a.txt?\n> 1. Yes\n2. Yes, and don't ask again this session")
	final := banner + "\nDo you want to create a.txt?\n> 1. Yes\n2. Yes, and don't ask again this session\n● DONE\n✔ Worked for 12s"
	if got := b.answer(final); got != "● DONE" {
		t.Fatalf("answer = %q", got)
	}
	// 什么都没记下（比如提示发出前就做完了）：退回整段尾部
	if got := (&BridgeSession{}).answer("DONE"); got != "DONE" {
		t.Fatalf("fallback answer = %q", got)
	}
}

func TestBridgeAnswerDropsCountersAndEcho(t *testing.T) {
	b := &BridgeSession{promptHead: "Reply with exactly: ARAN"}
	b.remember("Grok 4.7")
	// Grok 做完时屏幕的样子（2026-11-21 实测，节选）
	screen := "/ 2 2\n- 3 3\n♦ Thinking…\n› Reply with exactly: ARANEA-OK-grok.\nARANEA-OK-grok\n8.1\nAsk Codex to do anything\nWorked for 6s • 1:14 PM"
	if got := b.answer(screen); got != "ARANEA-OK-grok" {
		t.Fatalf("answer = %q", got)
	}
}

func TestOSCOnly(t *testing.T) {
	if !oscOnly([]byte("\x1b]0;⠹ Create text file with ok\x07")) {
		t.Fatal("a title-only update should not count as screen output")
	}
	if oscOnly([]byte("\x1b]0;title\x07 > 1. Yes")) || oscOnly(nil) {
		t.Fatal("output with visible text is screen output")
	}
}

func TestStillStarting(t *testing.T) {
	if !stillStarting([]string{"│  directory: Initializing…  │"}) || !stillStarting([]string{"⠴ Loading… · You can type; sending is paused"}) {
		t.Fatal("豆包 start-up screens should hold the first prompt")
	}
	if stillStarting([]string{"❯  Ask Doubao to do anything"}) {
		t.Fatal("ready input box held the prompt")
	}
}
