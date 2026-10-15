package voice

import (
	"aranea-agents/internal/biz"
	"testing"

	"github.com/stretchr/testify/require"
)

// 口播文本渲染：空题/单题/多题/选项/补标点。
func TestClarificationSpeech(t *testing.T) {
	require.Empty(t, clarificationSpeech(&biz.ClarificationEnvelope{}))
	require.Empty(t, clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{{Question: "  "}}}))

	single := clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{
		{Question: "平台？", Options: []string{"Web", "iOS"}},
	}})
	require.Contains(t, single, "一个问题")
	require.Contains(t, single, "平台？")
	require.Contains(t, single, "Web")
	require.Contains(t, single, "iOS")
	require.NotContains(t, single, "第一")
	require.Contains(t, single, "回答")

	multi := clarificationSpeech(&biz.ClarificationEnvelope{Questions: []biz.ClarificationQuestion{
		{Question: "平台？"},
		{Question: "风格"},
	}})
	require.Contains(t, multi, "两个问题")
	require.Contains(t, multi, "第一")
	require.Contains(t, multi, "第二")
	require.Contains(t, multi, "风格。")
}

func TestRouteEvent_SpeaksCodingTaskApproval(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.sess.mu.Lock()
	fx.sess.pendingTurns = 1
	fx.sess.mu.Unlock()

	fx.sess.routeEvent(biz.NewSystemNoticeEvent("sess-1", noticeCodingTaskApproval, "编码助手请求确认：运行测试", map[string]any{
		"speak": true,
		"title": "运行测试",
	}))
	fx.sess.mu.Lock()
	got := append([]string(nil), fx.sess.delegationOutbox...)
	fx.sess.mu.Unlock()
	require.Equal(t, []string{"编码助手请求确认：运行测试"}, got)
}

func TestRouteEvent_IgnoresCodingApprovalWithoutSpeak(t *testing.T) {
	fx := newSessionFixture(t)
	fx.sess.Start(StartParams{Language: "zh-CN", SampleRate: 16000})
	fx.sess.mu.Lock()
	fx.sess.pendingTurns = 1
	fx.sess.mu.Unlock()

	fx.sess.routeEvent(biz.NewSystemNoticeEvent("sess-1", noticeCodingTaskApproval, "不应播报", map[string]any{
		"speak": false,
	}))
	fx.sess.mu.Lock()
	n := len(fx.sess.delegationOutbox)
	fx.sess.mu.Unlock()
	require.Zero(t, n)
}
