package session

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// Fork-from-Turn（79-runtime-governance R6）：以某一轮为分叉点复制会话。
//
// 复制面（全部在单事务内完成）：
//   - sessions 行：parent_session_id=源 id、fork_from_turn_id=分叉 turn、agent 绑定同源；
//   - trpc_session_events：≤ 分叉 turn 末事件的运行时历史（框架 GetSession 的续聊依据）；
//   - trpc_session_states：新会话空 state 行（无此行框架 GetSession 报 session not found）；
//   - tasks_v2 / turns_v2 / steps_v2：≤ 分叉 turn 的 UI 消息记录（新 id 重挂新 session_id）。
//
// 不复制（有意决策，见 dev plan Phase 4）：
//   - L1 scratchpad / session state（fork 从干净工作记忆开始，recall 类 agent 级记忆共享不受影响）；
//   - runner_snapshot_json / session_summaries / trpc_session_summaries（派生产物，按需重建）；
//   - session_turns 准入/指标记录（fork 会话指标自零累计，不回填源会话用量）。
//
// 范围门禁：仅根会话（parent_session_id 为空）可 fork——team/member 子会话的
// 历史与 spirit 根交织（member steps 挂在根 spirit_session_id 下），复制语义
// 不闭合，明确拒绝而非产出半成品。

// SessionForkStore 是 Fork-from-Turn 复制原语的持久化端口
// （internal/data raw-SQL 双方言实现，框架表无前缀配置——trpc_ 为现网固定前缀）。
// Stability:evolving
type SessionForkStore interface {
	// ForkSessionInTx 在单事务内执行 fn（ent 与 raw-SQL 写共享同一 ent.Tx）。
	ForkSessionInTx(ctx context.Context, fn func(ctx context.Context) error) error
	// FindTurnEventBoundary 返回分叉 turn 末条框架事件的自增 id；
	// found=false 表示该 turn 在 trpc_session_events 中无任何事件。
	FindTurnEventBoundary(ctx context.Context, sessionID, turnID string) (boundary int64, found bool, err error)
	// CopyFrameworkEvents 复制 ≤ boundary 的未删事件到新会话（保序，event 原文 verbatim），
	// 返回复制条数。
	CopyFrameworkEvents(ctx context.Context, srcSessionID, dstSessionID string, boundary int64) (copied int, err error)
	// CreateFrameworkState 为新会话建空 state 行（app/user 取自源会话事件首行，
	// 与框架写入键一致）。
	CreateFrameworkState(ctx context.Context, srcSessionID, dstSessionID string) error
	// CopyV2Records 复制 ≤ forkTurnID 的 tasks/turns/steps（id 加确定性前缀重映射，
	// seq 原值保留），返回各表复制条数。
	CopyV2Records(ctx context.Context, srcSessionID, dstSessionID, forkTurnID string) (tasks, turns, steps int, err error)
}

// SessionForkUsecase 独立子用例（不塞进 SessionUsecase——AS-COG-01 字段预算已满）。
type SessionForkUsecase struct {
	sessions SessionReader
	writer   SessionWriter
	fork     SessionForkStore
	lg       loggateway.Logger
}

// NewSessionForkUsecase 构造 fork 用例；fork store 缺失（DB 未装配）时返回 nil，
// service 层据此 503。
func NewSessionForkUsecase(sessions SessionReader, writer SessionWriter, fork SessionForkStore, lg loggateway.Logger) *SessionForkUsecase {
	if sessions == nil || writer == nil || fork == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SessionForkUsecase{
		sessions: sessions, writer: writer, fork: fork,
		lg: lg.With(loggateway.Domain("session_fork")),
	}
}

// Fork 以 turnID 为分叉点从 srcID 派生新会话。turnID = v2 turn id（= invocation id，
// 即前端消息列表 ChatMessage.TurnID）。title 为空时默认「源标题（分支）」。
func (uc *SessionForkUsecase) Fork(ctx context.Context, srcID, turnID, title string) (Session, error) {
	if uc == nil {
		return Session{}, apierror.Internal("SESSION", "session fork not available")
	}
	srcID = strings.TrimSpace(srcID)
	turnID = strings.TrimSpace(turnID)
	if srcID == "" || turnID == "" {
		return Session{}, validationErr("session id and turn_id are required")
	}
	src, err := uc.sessions.GetSessionByID(ctx, srcID)
	if err != nil {
		return Session{}, err
	}
	// 范围门禁：仅根会话可 fork（team/member 子会话历史不闭合，见文件头注释）。
	// fork 出的会话 ParentSessionID 非空，同样被此门禁拦截——链式 fork 有意
	// 不支持：继承 turn 的 id 带 fk<dst8>- 前缀，而复制事件的 invocationId 是
	// 无前缀原 run id，FindTurnEventBoundary 无法命中；若未来放开需先做前缀
	// 剥离改造。
	if strings.TrimSpace(src.ParentSessionID) != "" {
		return Session{}, apierror.BadRequest("SESSION", "only root sessions can be forked (team/member child sessions and already-forked sessions are not supported)")
	}
	if strings.TrimSpace(title) == "" {
		title = src.Title + "（分支）"
	}

	dst := Session{
		ID:                         uuid.NewString(),
		WorkspaceID:                src.WorkspaceID,
		UserID:                     src.UserID,
		OwnerType:                  src.OwnerType,
		AgentID:                    src.AgentID,
		TeamID:                     src.TeamID,
		Title:                      title,
		TagsJSON:                   src.TagsJSON,
		DialogMode:                 src.DialogMode,
		DefaultProvider:            src.DefaultProvider,
		DefaultModel:               src.DefaultModel,
		DefaultContextWindowTokens: src.DefaultContextWindowTokens,
		Visibility:                 src.Visibility,
		ParentSessionID:            src.ID,
		ForkFromTurnID:             turnID,
		SessionType:                src.SessionType,
	}
	// 血缘根：源是树根则新会话挂入同一棵树；源本身是根则新会话以源为根。
	dst.RootSessionID = src.RootSessionID
	if strings.TrimSpace(dst.RootSessionID) == "" {
		dst.RootSessionID = src.ID
	}

	err = uc.fork.ForkSessionInTx(ctx, func(txCtx context.Context) error {
		if _, err := uc.writer.CreateSession(txCtx, dst); err != nil {
			return err
		}
		boundary, found, err := uc.fork.FindTurnEventBoundary(txCtx, srcID, turnID)
		if err != nil {
			return err
		}
		if !found {
			// turn 在 v2 中存在但无框架事件（事件 TTL 清理 / 产出任何事件前
			// 即失败）：前缀复制无从谈起，明确拒绝并给出可排查的消息。
			return apierror.NotFound("SESSION", "turn has no runtime events to fork from in source session")
		}
		if err := uc.fork.CreateFrameworkState(txCtx, srcID, dst.ID); err != nil {
			return err
		}
		events, err := uc.fork.CopyFrameworkEvents(txCtx, srcID, dst.ID, boundary)
		if err != nil {
			return err
		}
		tasks, turns, steps, err := uc.fork.CopyV2Records(txCtx, srcID, dst.ID, turnID)
		if err != nil {
			return err
		}
		uc.lg.Info("session forked",
			loggateway.StepID("session.fork"),
			loggateway.Str("src_session_id", srcID),
			loggateway.Str("dst_session_id", dst.ID),
			loggateway.Str("fork_turn_id", turnID),
			loggateway.Int("events_copied", events),
			loggateway.Int("tasks_copied", tasks),
			loggateway.Int("turns_copied", turns),
			loggateway.Int("steps_copied", steps),
		)
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return uc.sessions.GetSessionByID(ctx, dst.ID)
}
