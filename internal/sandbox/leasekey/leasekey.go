// Package leasekey 会话沙箱租约键派生（code_exec 与 sandbox_fs 共享，防两处漂移）。
//
// 83-长时运行韧性 FR-4：team run 上下文在基础会话键上追加成员维度
// "#run:<runID>#agent:<agentName>"，使同 run 不同成员各持沙箱（成员级并行），
// 不再共享 session 级单沙箱互相覆盖工作区。非 team 上下文键与旧规则一致。
package leasekey

import (
	"context"
	"strings"

	"aranea-agents/internal/sandbox"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// FromContext 派生沙箱租约键：基础键 app/user/sessionID；
// team run 上下文（ctx 携带 RunID 且 AgentName 非空）追加成员维度
// "#run:<runID>#agent:<agentName>"。
// 无会话上下文返回 TrimSpace(executionID)（codeexecutor ephemeral 兜底语义；
// sandboxfs 调用方对 "" 报错，维持现状语义）。
func FromContext(ctx context.Context, executionID string) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil || inv.Session.ID == "" {
		return strings.TrimSpace(executionID)
	}
	s := inv.Session
	key := s.AppName + "/" + s.UserID + "/" + s.ID
	if runID := sandbox.RunIDFromContext(ctx); runID != "" && inv.AgentName != "" {
		key += "#run:" + runID + "#agent:" + inv.AgentName
	}
	return key
}
