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
// Turn (ParentTaskID). Returns empty string if no Task exists and the ctx does
// not carry a root task ID.
//
// 2026-10-21 D3-FIX-3（87-orchestration-chain-hardening）：ctx 权威优先 ——
// chat/dispatch/dagRun 注入的 RootTaskActivityID 是本轮回合的权威挂靠点。
// 旧顺序「DB 最新行优先」在重启/重排后会错挂（最新行属于上一轮），且 v2
// sequencer 异步落库的时序间隙会让 DB 返回陈旧结果。
//
// 2026-07-04 问题 5 修复（原始动机，保留）：DB 查不到时不能轻易新建 Task，
// synthesis 触发文本会被渲染成新的用户输入气泡（重复执行）；ctx 根 Task ID
// 是可靠的内存态真相源。
func resolveLatestUserTaskID(ctx context.Context, reader biz.TaskV2Reader, lg loggateway.Logger, spiritSessionID string) string {
	// ctx authoritative-first：有根 Task ID 即返回，不再让 DB 滞后行误导挂靠。
	if rtID := string(agent.RootTaskActivityIDFromCtx(ctx)); rtID != "" {
		return rtID
	}
	if reader != nil {
		tasks, err := reader.ListTasksBySession(ctx, spiritSessionID)
		if err != nil {
			lg.Warn("resolveLatestUserTaskID: ListTasksBySession failed",
				loggateway.StepID("spirit.synthesis.parent_task_lookup_err"),
				loggateway.Str("spirit_session_id", spiritSessionID),
				loggateway.Err(err),
			)
			return ""
		}
		if len(tasks) > 0 {
			// tasks are returned in chronological (created_at) order; the last
			// one is the most recent user-input Task.
			return tasks[len(tasks)-1].ID
		}
	}
	return ""
}

func (s *TeamStarter) resolveLatestUserTaskID(ctx context.Context, spiritSessionID string) string {
	return resolveLatestUserTaskID(ctx, s.taskV2Reader, s.lg, spiritSessionID)
}

// synthesisGuardState 是 D3-FIX-3（87-orchestration-chain-hardening）的三态
// 持久化守卫：聚合「synthesis 是否已产出」的结论。
//   - produced：父 Task 已 Completed。synthesis 已产出 → CAS 保持占用吸收
//     后续 tick（重启后永久事实，不重复触发）。
//   - clear：确认本轮尚未产出（读路径未接线 / 列表滞后于 ctx 根 / 列表空
//     但 ctx 有权威根）→ 照常触发。
//   - uncertain：无法判定（查询失败 / 列表空且 ctx 无根）→ 释放 CAS 推迟
//     到 30s 轮询重试，不烧复活预算也不触发 —— 读抖动不得错挂也不得吞掉
//     总结（修复旧 fail-open 语义：重启后真错挂时用户看到重复总结）。
type synthesisGuardState int

const (
	synthesisGuardClear synthesisGuardState = iota
	synthesisGuardProduced
	synthesisGuardUncertain
)

// synthesisGuardState 判定本轮 synthesis 的持久化真相。ctx 携带的
// RootTaskActivityID 是「当前轮次」的权威锚点：
//   - rtID != "" 且 != 最新行 ID → 列表滞后，clear；
//   - rtID == "" 且列表空 → 时序间隙无法判定，uncertain；
//   - 列表空但 rtID 有 → 根 Task 未落库，clear。
func (s *TeamStarter) synthesisGuardState(ctx context.Context, spiritSessionID string) synthesisGuardState {
	if s.taskV2Reader == nil {
		return synthesisGuardClear
	}
	rtID := string(agent.RootTaskActivityIDFromCtx(ctx))
	tasks, err := s.taskV2Reader.ListTasksBySession(ctx, spiritSessionID)
	if err != nil {
		return synthesisGuardUncertain
	}
	if len(tasks) == 0 {
		if rtID == "" {
			return synthesisGuardUncertain
		}
		return synthesisGuardClear
	}
	latest := tasks[len(tasks)-1]
	if rtID != "" && rtID != latest.ID {
		return synthesisGuardClear
	}
	if latest.Status == biz.TaskStatusCompleted {
		return synthesisGuardProduced
	}
	return synthesisGuardClear
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
