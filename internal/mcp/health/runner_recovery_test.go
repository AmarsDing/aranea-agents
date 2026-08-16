package health

import (
	"testing"

	"aranea-agents/internal/mcp/lifecycle"
)

// TestIsRecoveryEdge（P0-3 守卫）：恢复边沿 = 已知坏态 → ok。
// unknown→ok 是启动后首次探测（agent 从未因该 server 掉线而装配陈旧
// toolset），不触发；ok→ok / →error / →auth_required 均非恢复。
func TestIsRecoveryEdge(t *testing.T) {
	cases := []struct {
		name string
		prev lifecycle.State
		next lifecycle.State
		want bool
	}{
		{"error→ok 恢复", lifecycle.StateError, lifecycle.StateOK, true},
		{"auth_required→ok 恢复（凭据补齐）", lifecycle.StateAuthRequired, lifecycle.StateOK, true},
		{"degraded→ok 恢复", lifecycle.StateDegraded, lifecycle.StateOK, true},
		{"unknown→ok 首探非恢复", lifecycle.StateUnknown, lifecycle.StateOK, false},
		{"ok→ok 稳态", lifecycle.StateOK, lifecycle.StateOK, false},
		{"error→error 连续失败", lifecycle.StateError, lifecycle.StateError, false},
		{"ok→error 掉线（反向边沿不属恢复）", lifecycle.StateOK, lifecycle.StateError, false},
		{"error→auth_required 坏态迁移", lifecycle.StateError, lifecycle.StateAuthRequired, false},
		{"error→degraded 坏态迁移", lifecycle.StateError, lifecycle.StateDegraded, false},
		{"degraded→error 恶化", lifecycle.StateDegraded, lifecycle.StateError, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRecoveryEdge(tc.prev, tc.next); got != tc.want {
				t.Fatalf("isRecoveryEdge(%s, %s) = %v, want %v", tc.prev, tc.next, got, tc.want)
			}
		})
	}
}
