package session

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"testing"
	"time"
)

// runCompress 级：CAS 冲突后既有的失败抑制记录必须保留（假成功会把它清掉）。
func TestRunCompress_CASConflictKeepsSuppression(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 200000, LastContextWindowTokens: 256000, CompressVersion: 0},
		maxSummarized: 0,

		msgs: func() []biz.ChatMessage {
			var out []biz.ChatMessage
			for t := 1; t <= 6; t++ {
				out = append(out,
					biz.ChatMessage{ID: "u" + string(rune('0'+t)), Role: "user", TurnNumber: t, ContentMarkdown: strings.Repeat("问", 300), CreatedAt: "2026-07-20T10:00:0" + string(rune('0'+t)) + "Z"},
					biz.ChatMessage{ID: "a" + string(rune('0'+t)), Role: "assistant", TurnNumber: t, ContentMarkdown: strings.Repeat("答", 300), CreatedAt: "2026-07-20T10:00:0" + string(rune('0'+t)) + "Z"},
				)
			}
			return out
		}(),
	}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 9}
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: tx,
		},
		Compress: &stubLLMCompressor{},
		lg:       loggateway.NewNoop(),
		flight:   newCompressFlightManager(),
		buf:      newCompressBufferManager(),
		suppress: newCompressSuppressManager(),
	}

	c.suppress.record("sess-1", compressFailureTransient, "/", time.Now())

	if err := c.runCompress(context.Background(), "sess-1", "", cacheEnabledAgent(), true); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.suppress.m.Load("sess-1"); !ok {
		t.Fatal("CAS 冲突不得清除既有抑制记录（假成功会误导后续退避决策）")
	}
}
