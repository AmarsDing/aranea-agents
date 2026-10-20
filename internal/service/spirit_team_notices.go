// Code owned by package service; extracted from spirit_team.go for AS-COG-01 file-size fitness.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

const deliverablesNoticeType = "deliverables"

// deliverablesNoticeMaxArtifacts 限制单条通知携带的产物数，防止病态编排
// （大量 bridged 产物）撑爆 WS 事件与 steps_v2 content 列。
const deliverablesNoticeMaxArtifacts = 50

// publishDeliverablesNotice 发布交付物清单 orphan notice step（TurnID 空、
// 挂 parentTaskID，任务卡 footer 位）。仅在存在已桥接产物（artifact_id
// 非空）时发布；parentTaskID 为空时前端无法附着渲染（与兜底完成通知同一
// 约束），跳过并记 Warn。失败仅记日志 —— 产物仍可由产物抽屉/管理页访问，
// 通知缺失不阻断 synthesis 主路径。
func (s *TeamStarter) publishDeliverablesNotice(ctx context.Context, spiritSessionID, parentTaskID string, digests []biz.TeamDeliverableDigest) {
	if !s.v2EventReady() {
		return
	}
	refs := make([]biz.DeliverableArtifactRef, 0, 8)
	seen := make(map[string]struct{})
loop:
	for _, d := range digests {
		for _, r := range d.ArtifactRefs {
			if _, dup := seen[r.ArtifactID]; dup {
				continue
			}
			seen[r.ArtifactID] = struct{}{}
			refs = append(refs, r)
			if len(refs) >= deliverablesNoticeMaxArtifacts {
				break loop
			}
		}
	}
	if len(refs) == 0 {
		return
	}
	if strings.TrimSpace(parentTaskID) == "" {
		s.lg.Warn("deliverables 通知 TaskID 为空，跳过发布（前端无法附着渲染）",
			loggateway.StepID("spirit.deliverables_notice.empty_task"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
		return
	}
	payload, err := json.Marshal(map[string]any{"artifacts": refs})
	if err != nil {
		s.lg.Warn("deliverables 通知载荷序列化失败，跳过发布",
			loggateway.StepID("spirit.deliverables_notice.marshal_err"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return
	}
	now := time.Now()
	step := biz.Step{
		ID:              uuid.NewString(),
		SessionID:       spiritSessionID,
		SpiritSessionID: spiritSessionID,
		TaskID:          parentTaskID,
		Kind:            biz.StepKindNotice,
		NoticeType:      deliverablesNoticeType,
		Content:         string(payload),
		Status:          biz.StepStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &now,
		Version:         1,
		AuthorAgentKey:  "team-starter",
	}
	s.publishV2Event(ctx, biz.NewStepCreatedEvent(step))
	s.lg.Info("all teams completed: 已发布 deliverables 交付物清单通知",
		loggateway.StepID("spirit.deliverables_notice_published"),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Str("parent_task_id", parentTaskID),
		loggateway.Int("artifact_count", len(refs)),
	)
}

// The synthesis trigger text is built by biz.BuildSynthesisSummaryTrigger
// (2026-07-25 Fix 3): all-completed sessions get the standard structure;
// sessions with failed teams get an honest variant carrying the real counts
// and each failed team's reason + last reply (its unresolved questions).
// The LLM reply (a normal reply step, persisted and refresh-safe) IS the
// summary report — there is no dedicated report UI. The trigger text is not
// rendered as a user bubble (the Turn attaches to the parent Task as a
// continuation Turn), so it can be verbose.

// publishAllTeamsCompletedFallbackNotice publishes the terminal notice when
// the summary turn could not be triggered (the Spirit LLM reply is the
// normal completion signal; without it the user would see nothing). The
// notice must be honest (2026-07-25 Fix 3): failures downgrade it to a
// warning with the real counts — a "所有团队已完成" success notice while
// teams failed repeats the 19:29 lie. The notice must attach to the latest
// user Task — notice events that cannot attach to a task are not rendered
// by the frontend (hard constraint).
func (s *TeamStarter) publishAllTeamsCompletedFallbackNotice(ctx context.Context, spiritSessionID, parentTaskID string, completed, failed int) {
	if !s.v2EventReady() {
		return
	}
	if parentTaskID == "" {
		// Task 查询失败且 ctx 无 RootTaskActivityID 时兜底通知静默失效，
		// 至少留 Warn 便于诊断双重失败（turn 触发失败 + Task 查询失败）。
		s.lg.Warn("all teams completed: 兜底通知 TaskID 为空，前端可能不渲染完成信号",
			loggateway.StepID("spirit.fallback_notice.empty_task"),
			loggateway.Str("spirit_session_id", spiritSessionID),
		)
	}
	content := "所有团队已完成"
	noticeType := "success"
	if failed > 0 {
		content = fmt.Sprintf("团队执行结束：%d 完成 / %d 失败", completed, failed)
		noticeType = "warning"
	}
	now := time.Now()
	step := biz.Step{
		ID:              uuid.NewString(),
		SessionID:       spiritSessionID,
		SpiritSessionID: spiritSessionID,
		TaskID:          parentTaskID,
		Kind:            biz.StepKindNotice,
		NoticeType:      noticeType,
		Content:         content,
		Status:          biz.StepStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &now,
		Version:         1,
		AuthorAgentKey:  "team-starter",
	}
	s.publishV2Event(ctx, biz.NewStepCreatedEvent(step))
}

// resolveLatestUserTaskID returns the most recent Task ID for the given spirit
// session, so the system-push synthesis Turn can attach to it as a continuation
// Turn (ParentTaskID). Returns empty string if no Task exists or the reader is
// not wired (v1-only deployments) — in that case the synthesis Turn creates a
// new Task as before (legacy behaviour).
//
// 2026-07-04 问题 5 修复：v2 Sequencer 异步持久化 Task，当所有团队快速完成时
// （例如单团队秒级返回），ListTasksBySession 可能查不到记录，导致 parentTaskID
// 为空，synthesis Turn 在 projector.OnTurnStart 中走"创建新 Task"分支，把
// synthesis 触发文本"所有团队已完成…"渲染成新的用户输入气泡（重复执行）。
// 解决方案：DB 查不到时，从 ctx 中取 RootTaskActivityID（由 chat_orchestrator
// 在 chat_orchestrator_turn.go:401 通过 ContextWithRootTaskActivityID 注入）
// 作为 fallback。RootTaskActivityID 是根 Task 的 ID，与 team 阶段的 ctx 链
// 一脉相承，是可靠的内存态真相源。
//
// 2026-07-22 B.10.17：提取为包级函数；2026-07-24 报告 UI 移除后仅供
// TeamStarter（总结 turn ParentTaskID）使用。
func resolveLatestUserTaskID(ctx context.Context, reader biz.TaskV2Reader, lg loggateway.Logger, spiritSessionID string) string {
	if reader != nil {
		tasks, err := reader.ListTasksBySession(ctx, spiritSessionID)
		if err != nil {
			lg.Warn("resolveLatestUserTaskID: ListTasksBySession failed",
				loggateway.StepID("spirit.synthesis.parent_task_lookup_err"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(err),
			)
			// fall through to ctx fallback
		} else if len(tasks) > 0 {
			// tasks are returned in chronological (created_at) order; the last
			// one is the most recent user-input Task.
			return tasks[len(tasks)-1].ID
		}
	}
	// Fallback: use RootTaskActivityID from ctx (set by chat_orchestrator).
	// This covers the timing gap where the v2 Task hasn't been persisted yet
	// when checkAllTeamsCompleted runs, as well as v1-only deployments where
	// taskV2Reader is nil. RootTaskActivityID is the canonical root task ID
	// for the current spirit session turn chain.
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID != "" {
		lg.Info("resolveLatestUserTaskID: fell back to ctx RootTaskActivityID",
			loggateway.StepID("spirit.synthesis.parent_task_ctx_fallback"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Str("root_task_id", rtID),
		)
		return rtID
	}
	return ""
}

func (s *TeamStarter) resolveLatestUserTaskID(ctx context.Context, spiritSessionID string) string {
	return resolveLatestUserTaskID(ctx, s.taskV2Reader, s.lg, spiritSessionID)
}

// synthesisAlreadyProduced 是 checkAllTeamsCompleted 的重启幂等守卫
// （2026-08-07）：内存 CAS（synthesisTriggered）进程重启即丢失，而 spirit
// session 常驻 Running，30s 轮询会持续命中 AllDone。持久化真相：
// synthesis continuation turn 结束时 projector 关闭父 Task（task.completed，
// projector.go OnTurnEnd ParentTaskID 分支），故「最新 Task 已 Completed」
// ⟺「synthesis 已产出」。
//
// fail-open 边界：reader 未接线 / 查询失败 / 无任务行 时返回 false，维持
// 既有行为（内存 CAS 为唯一守卫）—— 读抖动不得丢失本轮总结。
// 时序间隙：新一轮编排的 Task 可能尚未异步落库（v2 sequencer），若 ctx
// 携带的根 Task ID 比 DB 最新行更新，说明列表滞后，不得据旧行拦截本轮
// synthesis。
func (s *TeamStarter) synthesisAlreadyProduced(ctx context.Context, spiritSessionID string) bool {
	if s.taskV2Reader == nil {
		return false
	}
	tasks, err := s.taskV2Reader.ListTasksBySession(ctx, spiritSessionID)
	if err != nil || len(tasks) == 0 {
		return false
	}
	latest := tasks[len(tasks)-1]
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID != "" && rtID != latest.ID {
		return false
	}
	return latest.Status == biz.TaskStatusCompleted
}

// HandleTeamTimeout implements biz.TimeoutHandler. Called when a team times out
// to trigger dependency scheduling, event publishing, and AllDone checks — the
// same lifecycle as HandleTeamTurnResult for a failed team.
func (s *TeamStarter) HandleTeamTimeout(ctx context.Context, spiritSessionID, teamID string) {
	s.HandleTeamTurnResult(ctx, spiritSessionID, teamID, biz.TeamStatusFailed, "team execution timed out", "")
}

// NotifyAllTeamsCompleted implements biz.AllTeamsCompletedNotifier. Called by the
// background poller when all teams for a spirit session have reached terminal
// state. This is the "active notification" path from the backend polling
// mechanism, supplementing the event-driven path in HandleTeamTurnResult.
func (s *TeamStarter) NotifyAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
	s.checkAllTeamsCompleted(ctx, spiritSessionID)
}

// SpiritTeamAssembler assembles spirit teams.
//
// `eventBus`/`seq` 发布 team_stage。Chat Graph 由 v2 GraphStage/GraphNode 维护，
// 不再注入 v1 ActivityEventBus。
