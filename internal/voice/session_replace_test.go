package voice

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// closingDownlink 在 fakeDownlink 之上实现 DownlinkCloser，记录关闭调用。
type closingDownlink struct {
	fakeDownlink
	codes []int
	texts []string
	calls int
}

func (d *closingDownlink) CloseWithCode(code int, text string) {
	d.calls++
	d.codes = append(d.codes, code)
	d.texts = append(d.texts, text)
}

func newReplaceTestSession(t *testing.T, down Downlink) *Session {
	t.Helper()
	sess := NewSession(context.Background(), SessionDeps{
		NewASR: func(context.Context) (biz.StreamingASRProvider, biz.ASRSessionConfig, error) {
			return &fakeASRProvider{sess: &fakeASRSession{events: make(chan biz.ASREvent, 1)}}, biz.ASRSessionConfig{}, nil
		},
		NewTTS: func(context.Context) (biz.StreamingTTSProvider, biz.TTSSessionConfig, error) {
			return nil, biz.TTSSessionConfig{}, nil
		},
		Bus: newFakeBus(),
		LG:  loggateway.NewNoop(),
	}, "sess-1", "user-1", down)
	t.Cleanup(sess.Close)
	return sess
}

func sentTypes(d *fakeDownlink) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.jsons))
	for _, m := range d.jsons {
		if s, ok := m["type"].(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestReplaceNoticeAndCloseClosesConn 评审 2026-11-09 V1：单会话单连接替换时，
// ReplaceNoticeAndClose 发出 voice.replaced 后必须以业务关闭码 4401
// （voice_replaced）经 DownlinkCloser 优雅关闭旧连接——旧实现只拆会话不碰
// 连接，旧 conn 两端残留（服务端 readPump 阻塞，客户端误以为连接仍活）。
func TestReplaceNoticeAndCloseClosesConn(t *testing.T) {
	down := &closingDownlink{}
	sess := newReplaceTestSession(t, down)

	sess.ReplaceNoticeAndClose()

	if got := sentTypes(&down.fakeDownlink); len(got) != 1 || got[0] != "voice.replaced" {
		t.Fatalf("sent notices = %v, want [voice.replaced]", got)
	}
	if down.calls != 1 {
		t.Fatalf("CloseWithCode calls = %d, want 1", down.calls)
	}
	if down.codes[0] != CloseCodeSessionRevoked || down.texts[0] != "voice_replaced" {
		t.Fatalf("close = %d/%q, want %d/voice_replaced",
			down.codes[0], down.texts[0], CloseCodeSessionRevoked)
	}
}

// TestReplaceNoticeAndCloseWithoutCloser 下行通道未实现 DownlinkCloser 时
// 维持原 teardown-only 语义：通知照发、全量拆除、不得 panic。
func TestReplaceNoticeAndCloseWithoutCloser(t *testing.T) {
	down := &fakeDownlink{}
	sess := newReplaceTestSession(t, down)

	sess.ReplaceNoticeAndClose()

	if got := sentTypes(down); len(got) != 1 || got[0] != "voice.replaced" {
		t.Fatalf("sent notices = %v, want [voice.replaced]", got)
	}
}
