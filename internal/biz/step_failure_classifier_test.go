package biz

import "testing"

// TestClassifyStepFailure P2-3：L4 失败分类器规则表。capability/semantic 不
// 消耗重试预算；transient 与未知消息 fail-open 为 transient（保持 F2 语义）。
func TestClassifyStepFailure(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		msg  string
		want StepFailureClass
	}{
		// 能力缺失
		{"agent 密钥缺失", "agent keys not found", StepFailureCapability},
		{"agent 不存在", "agent not found: xxx", StepFailureCapability},
		{"未授权", "error, status code: 401, message: Unauthorized", StepFailureCapability},
		{"权限不足", "403 Forbidden", StepFailureCapability},
		{"模型不存在", "model not found: gpt-x", StepFailureCapability},
		{"配额耗尽（计费）", "insufficient_quota: exceeded your current quota", StepFailureCapability},
		{"未配置", "skill repo not configured", StepFailureCapability},
		// 语义错误
		{"输入校验", "invalid input: empty roster", StepFailureSemantic},
		{"内容安全拦截", "content filter triggered", StepFailureSemantic},
		{"护栏", "guardrail violation", StepFailureSemantic},
		// 瞬时故障
		{"首字节超时", "transient: first byte timeout", StepFailureTransient},
		{"deadline", "context deadline exceeded", StepFailureTransient},
		{"限流", "rate limit exceeded, status code: 429", StepFailureTransient},
		{"上游 503", "bad response: 503", StepFailureTransient},
		{"连接重置", "read: connection reset by peer", StepFailureTransient},
		{"EOF", "unexpected EOF", StepFailureTransient},
		// 未知消息 fail-open
		{"未知错误", "team execution failed", StepFailureTransient},
		{"空消息", "", StepFailureTransient},
		// 大小写不敏感
		{"大写 TIMEOUT", "LLM TIMEOUT waiting first token", StepFailureTransient},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClassifyStepFailure(tc.msg); got != tc.want {
				t.Errorf("ClassifyStepFailure(%q) = %s, want %s", tc.msg, got, tc.want)
			}
		})
	}
}
