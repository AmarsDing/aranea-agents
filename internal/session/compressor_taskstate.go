package session

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// writebackL1TaskBoard 把压缩产出的结构化任务状态回写到当前 L1 任务的
// metadata_json["task_board"]（P0 闭环：压缩器是 task_state 唯一生产点，
// L1 prompt cue 与快照注入渲染同一份进度）。
//
// 挂在压缩事务成功之后（与 syncRuntimeSnapshot 同级）：L1 仓库使用独立
// DB 句柄无法加入会话压缩事务，且回写失败不应让压缩整体失败（Warn 降级）。
// taskState 为 nil/空时跳过（保留既有 board，语义同 latestTaskState 最新非空优先）。
func (c *Compressor) writebackL1TaskBoard(ctx context.Context, sess biz.Session, ag biz.Agent, taskState *biz.TaskState) {
	if c.l1BoardWriter == nil || taskState == nil || taskState.Empty() {
		return
	}
	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		agentID = strings.TrimSpace(ag.ID)
	}
	if agentID == "" {
		return
	}
	ok, err := c.l1BoardWriter.UpdateL1TaskBoard(ctx, sess.ID, agentID, marshalTaskState(taskState))
	if err != nil {
		c.lg.Warn("L1 task_board 回写失败",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sess.ID),
			loggateway.Err(err))
		return
	}
	if ok {
		c.lg.Info("L1 task_board 已回写",
			loggateway.StepID("session.compress"),
			loggateway.SessionID(sess.ID))
	}
}
