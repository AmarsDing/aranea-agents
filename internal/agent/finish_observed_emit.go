package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"
)

// finish_observed_emit.go —— Q5 批 D3 业务 wrap：llm_abnormal_finish 的
// Important 级（AS-EVT-01）EmitGate 双写（94-llm-abnormal-finish-observer
// 三件套；评审 2026-10-22-review-framework-finish-reason-eof §4 验收口径。
// provider/finish_observed.go 的 wrapper 只采集纯事实，本闭包是唯一判定与
// 发射面：
//   1. kind 判定——length/content_filter 走 finish_reason 小写化字面
//      （gemini UPPER_SNAKE 不映射，避免把供应商 enum 拼写当 length 证据）；
//      EOF 断言=本包分类器 ClassifyInvalidStream **同一判序**下的
//      StreamInvalidHTTPEOF 结果——overflow 恒 None、stall 耗尽恒 None 的
//      排除语义免费获得（评审 major：禁止字符串级镜像，防 fork 例外窗口
//      的证据污染）。silent 净关 / 其余 kind 不在 Q5 事件面（chat 层
//      NoFinishReason 已覆盖静默净关）。
//   2. 会话级 5min 去重——键形 llm_abnormal_finish:<session>:<floor(ts,300)>
//      （复用 spill_failed 桶 deduper）；空 session 回落 "unknown" 占位并
//      记档——防 title/memory 等全进程非同源事件坍缩同桶互吃。
//   3. 三方证据：decision EmitGate（异步 outbox）+ flow_log system.gate.
//      llm_abnormal_finish（event.EmitGate 内建对偶；turn ctx 无 TraceEmitter
//      时静默跳过——后台路径天然裸记录，94.design.md 记档）+ 进程 Warn。
// collector 为 nil（CLI/未接管部署）时 EmitGate 内建 LogNilCollector 告警，
// Warn 仍写——绝不静默（D1/0827 skip 先例）。
//
// 多副本语义（94.design.md D3-6）：deduper 是进程内闭包态——N 副本同 session
// 每桶各发一封（线性放大被接受，origin 字段供折算）；进程内跨接线面
// （chat/team/graph）共享同一 Hooks 实例故共享同一个 deduper。
//
// 洗涤边界：只做观测发射，不参与 LLM 调用主链——发射自身异常一律在
// wrapper 侧 recover 吞掉（观测链失败不炸 turn）。

// finishObservedUnknownSession 是空 session 的桶键占位（记档：防全进程非同源
// 事件坍缩进同一 5min 桶互吃第一封名额）。
const finishObservedUnknownSession = "unknown"

// NewFinishObservedEmitHooks 构造 provider.FinishObserverHooks 的生产发射闭
// 包（wire 注入点：provideLLMFinishObserverHooks，经 TurnDeps/graph 链共享
// 同一实例）。lg 为 nil 时回落 Noop。
func NewFinishObservedEmitHooks(c decision.Collector, lg loggateway.Logger) provider.FinishObserverHooks {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	dedup := &spillFailedDeduper{seen: make(map[string]struct{})}
	return provider.FinishObserverHooks{
		OnAbnormalFinish: func(ctx context.Context, obs provider.FinishObservation) {
			kind := classifyFinishObserved(obs)
			if kind == "" {
				return // 正常收官 / 各有专属语义路径的终态：零日志零记录
			}
			// 取消中性：turn ctx 取消（用户中止/超时回收）不审计——既有失败
			// 路径已覆盖（D2 关停 neutral 先例的调用侧同义）。先判取消再计
			// 去重，防取消帧吃掉同桶第一封名额。
			if ctx != nil && ctx.Err() != nil {
				return
			}
			runID := strings.TrimSpace(decision.GateRunIDFromContext(ctx))
			// D3-f 裁决（对抗评审双 major 确认）：本事件面不用 gateRunID 的
			// invocation-id 回落。gateRunID（tool_param_rule_gate.go:57-70）
			// 是 2026-08-27 H5 为 RunGateStats 记录族设计的「无注入回落当前
			// invocation id（不 join 任何 team run，stats 自然忽略）」语义；
			// 但 llm_abnormal_finish 的 origin 三方归属与 SourceRef.RunID 审计
			// 口径都必须是真 team run id——生产 chat/graph 轮 ctx 恒带
			// invocation（runner.NewInvocationContext，InvocationID 恒非空），
			// 采纳回落会产生双重危害：origin 首支恒命中 team_turn（chat_turn/
			// background 在 runner 驱动路径下不可达，FR-8 三方折算失真）且
			// SourceRef.RunID 落 invocation uuid（永不可 join team_runs，仅
			// 污染审计）。故 origin 判定与记录 RunID 统一取 raw 注入值：team
			// runner 注入非空=team_turn；无注入=chat_turn/background（「chat
			// 轮无 run」）。会话归属 gateSessionID 的 inv.Session.ID 回落仍正用
			// ——chat invocation 的 Session 即真会话（T5 语义），无过载。
			recSession := strings.TrimSpace(obs.SessionID)
			if gs := strings.TrimSpace(gateSessionID(ctx)); gs != "" {
				recSession = gs // ctx 注入（chat T5 回填/team runner）优先
			}
			bucketSession := recSession
			if bucketSession == "" {
				// 桶键回落 "unknown"（记档：防 title/memory 等全进程非同源
				// 事件坍缩同桶互吃第一封名额）——仅桶键占位，记录面 SessionID
				// 保持原值（空就空，不污染 SourceRef 审计口径）。
				bucketSession = finishObservedUnknownSession
			}
			dedupeKey := fmt.Sprintf("llm_abnormal_finish:%s:%d", bucketSession, time.Now().Unix()/300)
			if !dedup.seenOnce(dedupeKey) {
				return // 同 5min 桶第二次起静默（观测已落，防刷屏）
			}
			origin := strings.TrimSpace(obs.Origin)
			if origin == "" {
				switch {
				case runID != "":
					origin = "team_turn"
				case recSession != "":
					origin = "chat_turn"
				default:
					origin = "background"
				}
			}
			finish := strings.TrimSpace(obs.FinishReason)
			reasoning := fmt.Sprintf("finish_reason=%q kind=%s last_error=%q", finish, kind, obs.LastError)
			lg.Warn("LLM 输出异常收官（finish_reason/EOF 结构化透传）",
				loggateway.StepID("system.gate.llm_abnormal_finish"),
				loggateway.Str("kind", kind),
				loggateway.Str("finish_reason", finish),
				loggateway.Str("provider", obs.Provider),
				loggateway.Str("model", obs.Model),
				loggateway.Str("origin", origin),
				loggateway.SessionID(recSession),
				loggateway.Str("dedupe_key", dedupeKey))
			event.EmitGate(ctx, c, decision.GateDecision{
				TriggerRule: decision.TriggerLLMAbnormalFinish,
				Outcome:     "tripped",
				Scenario:    "LLM 输出异常收官（finish_reason/EOF）",
				Reasoning:   reasoning,
				GuardName:   "llm_finish_observer",
				RunID:       runID,
				SessionID:   recSession,
				Action:      "observe",
				Extra: map[string]any{
					"finish_reason": finish,
					"kind":          kind,
					"provider":      obs.Provider,
					"model":         obs.Model,
					"origin":        origin,
					"dedupe_key":    dedupeKey,
				},
			})
		},
	}
}

// classifyFinishObserved 按 Q5 §4 验收口径判定观测快照是否属本事件面：
//   - finish_reason ∈ {length, content_filter} → kind=小写字面（gemini
//     UPPER_SNAKE 枚举拼写不映射——只认归宿一后的字面）；
//   - 否则流内错误帧经本包分类器同一判序判定为 StreamInvalidHTTPEOF →
//     kind="eof"（overflow+eof 共存、stall 耗尽等恒 None 的排除语义随判序
//     免费获得，防误报）；
//   - 其他（正常 stop/tool_calls、silent 净关、malformed、通用 stream error）
//     → ""（不在 Q5 事件面，静默）。
func classifyFinishObserved(obs provider.FinishObservation) string {
	if obs.SawFinishReason {
		switch finish := strings.ToLower(strings.TrimSpace(obs.FinishReason)); finish {
		case "length", "content_filter":
			return finish
		}
	}
	if obs.HasError {
		if ClassifyInvalidStream(StreamInvalidObs{
			SawFinishReason: obs.SawFinishReason,
			FinishReason:    obs.FinishReason,
			HasError:        true,
			LastError:       obs.LastError,
			LastErrorType:   obs.LastErrorType,
		}) == StreamInvalidHTTPEOF {
			return "eof"
		}
	}
	return ""
}
