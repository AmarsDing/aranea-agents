// knowledge_sourcesync_frames.go —— G-38 W8：SourceSync 轮次 WS 聚合帧
// （FrameSink 实现：runner 事件点 → v2 SystemNoticeEvent WS-only 广播）。
//
// 帧纪律（types.go FrameSink 契约＋design ⑧）：
//   - ≤1 started + ≤1 done（runner 已在 SyncOnce/finalize 各打一点，本缝零节流
//     负担——节流条款（≥15s 或 >20% 进度）留给未来 progress 帧，本批不发）；
//   - auth_paused 状态迁移必发（done 帧 status=failed∧error_code∈auth 家族即
//     迁移信号——凭据管理员修凭据后 PUT 唤醒面在同一面板）；
//   - skipped 轮零帧（runner 不触发事件点）；
//   - nil bus 静默（与 publishKnowledgeIngest 同闸——W5 先例）。
//
// 载荷面：meta 键集唯一构造在本文件（前端 KnowledgeConnectorsPanel 消费面；
// 键集改动唯一改这里＋前端 types 镜像同步）。
package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	knowledgesync "aranea-agents/internal/knowledge/sourcesync"
)

// SourceSyncFrameNoticeType WS NoticeType（前端 isSourceSyncNotice 过滤键）。
const SourceSyncFrameNoticeType = "knowledge_source_sync"

// SourceSyncFrameMeta 帧 meta 键集中央构造（W8 决策：与 KnowledgeChunksDiffMeta
// 同款中央构造纪律——键集改动唯一改这里）。event_phase ∈ started/finished。
func SourceSyncFrameMeta(ev knowledgesync.RunEvent, phase string) map[string]any {
	meta := map[string]any{
		"event_type":   SourceSyncFrameNoticeType,
		"event_phase":  phase,
		"connector_id": ev.ConnectorID,
		"kind":         string(ev.Kind),
		"run_id":       ev.RunID,
	}
	if phase == "finished" {
		meta["status"] = string(ev.Status)
		meta["error_code"] = string(ev.ErrorCode)
		meta["seen"] = ev.Stats.Seen
		meta["created"] = ev.Stats.Created
		meta["updated"] = ev.Stats.Updated
		meta["unchanged"] = ev.Stats.Unchanged
		meta["fetch_failed"] = ev.Stats.FetchFailed
		meta["deleted"] = ev.Stats.Deleted
		meta["miss_unknown"] = ev.Stats.MissUnknown
		meta["bytes_pulled"] = ev.Stats.BytesPulled
	}
	return meta
}

// sourceSyncFrameSink FrameSink 实现（薄封：事件点 → SystemNoticeEvent 广播；
// 无节流无聚合——runner 已保证 ≤1 started + ≤1 done/轮）。
type sourceSyncFrameSink struct {
	bus biz.EventBus
}

// NewSourceSyncFrameSink 装配入口（wire：bus nil 时返回 nil=静默缝，
// runner nil-Frames 闸即 noop——双闸与 PublishKnowledgeChunksDiffFrame 同款）。
func NewSourceSyncFrameSink(bus biz.EventBus) knowledgesync.FrameSink {
	if bus == nil {
		return nil
	}
	return &sourceSyncFrameSink{bus: bus}
}

func (s *sourceSyncFrameSink) OnRunStarted(ctx context.Context, ev knowledgesync.RunEvent) {
	meta := SourceSyncFrameMeta(ev, "started")
	msg := fmt.Sprintf("Source sync started: connector=%s run=%s", ev.ConnectorID, ev.RunID)
	s.bus.Publish(ctx, biz.NewSystemNoticeEvent("", SourceSyncFrameNoticeType, msg, meta))
}

func (s *sourceSyncFrameSink) OnRunFinished(ctx context.Context, ev knowledgesync.RunEvent) {
	meta := SourceSyncFrameMeta(ev, "finished")
	msg := fmt.Sprintf("Source sync finished: connector=%s run=%s status=%s",
		ev.ConnectorID, ev.RunID, ev.Status)
	s.bus.Publish(ctx, biz.NewSystemNoticeEvent("", SourceSyncFrameNoticeType, msg, meta))
}
