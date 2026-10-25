package service

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"aranea-agents/pkg/loggateway"
)

// P0-6 阶段 0（docs/reports/2026-10-21-review-harness-p0-gap-solutions.md §7.8
// doNow ②③）：replica-safety 启动自检 + healthz 状态透出。
// 边界声明：
//   - 本轮零行为变化：未声明或声明 1 时自检静默通过，不改变任何现有路径。
//   - 声明 >1 时逐项 Warn（结构化日志，loggateway 注入）；fail-fast 通过
//     ENForce 环境变量显式开启（灰度开关，默认 OFF）。
//   - 本阶段不迁移任何 PG 租约/信封（阶段 1/2 见 docs/development/replica-safety.md）。
//   - AS-EVT-01：本轮 P0-6 零新增事件，仅新增启动结构化日志（非事件）。
const (
	// EnvDeclaredReplicas 声明副本数（0=未声明，按单副本处理）。
	// 批 C（2026-10-22）：server.declared_replicas conf 挂载已落地（批 B 加的
	// proto 字段），conf 值 >0 优先于本环境变量；env 回落保留 CLI/裸跑部署
	// 与存量 gray 发布姿势。
	EnvDeclaredReplicas = "ARANEA_DECLARED_REPLICAS"
	// EnvReplicaEnforce 多副本 fail-fast 灰度开关（1/true 开启，默认 OFF=仅 Warn）。
	EnvReplicaEnforce = "ARANEA_REPLICA_ENFORCE"
)

// CoordinationMode 值——healthz coordination_mode 字段与本包状态共用。
const (
	CoordinationModeSingle        = "single"         // 未声明或声明 1：单副本语义（现状）
	CoordinationModeMultiWarn     = "multi_warn"     // 声明 >1：9 项单副本边界功能逐条 Warn 降级
	CoordinationModeMultiEnforced = "multi_enforced" // 声明 >1 + ENFORCE：fail-fast
)

// SingleReplicaFeature 描述一项「单副本才安全」功能（阶段 0 公示清单，
// 与 docs/development/replica-safety.md §3 表一一对应）。
type SingleReplicaFeature struct {
	Key         string // 稳定机器键（Warn 日志与文档共用）
	Description string // 中文功能描述
	Anchor      string // 代码锚点 file:line（DOC-SYNC-6：必须真实存在）
}

// singleReplicaFeatures 阶段 0 公示的 9 项单副本边界清单（P0-6 §7.8 doNow ①）。
// 锚点见 docs/development/replica-safety.md §3；新增项须同步该文档。
func singleReplicaFeatures() []SingleReplicaFeature {
	return []SingleReplicaFeature{
		{"dag_run_lease", "PlanBoard DAG 执行租约（重复投递去重）", "internal/service/plan_executor_lifecycle.go:157"},
		{"has_active_run_for_session", "HasActiveRunForSession 门控（synthesis 波次中点拦截）", "internal/service/plan_executor_taskplan.go:96"},
		{"playbook_confirm_waiter", "playbook_confirm_before 等待器", "internal/service/plan_executor.go:125"},
		{"team_completion_waiter", "team 完成回传等待器", "internal/service/team_orchestrator_real.go:31"},
		{"tool_approval_waiter", "工具审批等待器（agentbridge pending）", "internal/service/agentbridge.go:55"},
		{"chat_clarify_waiter", "chat clarify 澄清等待", "internal/service/chat_clarify.go:50"},
		{"run_registry_cancel_status", "RunRegistry Cancel/status 内存表", "internal/runtime/run_registry.go:46"},
		{"synthesis_dedup", "synthesis 触发去重（synthesisTriggered/Attempts）", "internal/service/spirit_team.go:56"},
		{"seqassigner_issuance", "seqassigner 会话事件发号", "internal/agent/v2/seqassigner.go:22"},
	}
}

// ReplicaSafetyStatus 是 healthz 暴露与启动自检共用的快照。Mode 取
// CoordinationMode* 常量；Affected 仅在 multi_* 模式下非空（单副本清单全量列出
// 属文档职责，healthz 不重复枚举以避免噪音）。
type ReplicaSafetyStatus struct {
	DeclaredReplicas int
	Mode             string
	Enforce          bool
	ParseError       bool // 声明值无法解析（按未声明=单副本处理）
}

var lastReplicaSafety atomic.Value // stores ReplicaSafetyStatus

// ParseDeclaredReplicas 解析声明副本数；空/非法/负数 → (0, false) 按未声明处理。
// 坏输入必拒：拒绝静默吞错，调用方须按 parseOK=false 走 Warn 降级路径。
func ParseDeclaredReplicas(raw string) (replicas int, parseOK bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, true // 未声明
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// ParseEnforce 解析 fail-fast 开关；仅 "1"/"true"（不区分大小写、可含空白）开启。
func ParseEnforce(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	return s == "1" || s == "true"
}

// ResolveReplicaSafety 以纯函数解析当前环境变量快照（无副作用，不读全局）。
// 规则：未声明(0) 或 1 → single（零行为变化）；>1 → multi_warn，开 ENFORCE → multi_enforced。
func ResolveReplicaSafety(getenv func(string) string) ReplicaSafetyStatus {
	if getenv == nil {
		getenv = os.Getenv
	}
	replicas, parseOK := ParseDeclaredReplicas(getenv(EnvDeclaredReplicas))
	enforce := ParseEnforce(getenv(EnvReplicaEnforce))
	st := ReplicaSafetyStatus{
		DeclaredReplicas: replicas,
		Mode:             CoordinationModeSingle,
		Enforce:          enforce,
		ParseError:       !parseOK,
	}
	if replicas > 1 {
		if enforce {
			st.Mode = CoordinationModeMultiEnforced
		} else {
			st.Mode = CoordinationModeMultiWarn
		}
	}
	return st
}

// ResolveReplicaSafetyWithOverride 在 env 链路之上叠加 conf 声明（批 C Q3
// 2026-10-22）：override >0 时副本数以 conf 为准（声明式配置优先；PB int32
// 类型化无解析错误，ParseError 恒 false），<=0 等价 ResolveReplicaSafety
// 纯 env 回落。ENFORCE 无 conf 字段，仍仅经 env 读取（fail-fast 是显式
// 运维手势，不宜沉到配置文件里误随滚动发布开启）。
func ResolveReplicaSafetyWithOverride(getenv func(string) string, override int32) ReplicaSafetyStatus {
	if override <= 0 {
		return ResolveReplicaSafety(getenv)
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	st := ReplicaSafetyStatus{
		DeclaredReplicas: int(override),
		Mode:             CoordinationModeSingle,
		Enforce:          ParseEnforce(getenv(EnvReplicaEnforce)),
	}
	if override > 1 {
		if st.Enforce {
			st.Mode = CoordinationModeMultiEnforced
		} else {
			st.Mode = CoordinationModeMultiWarn
		}
	}
	return st
}

// ErrMultiReplicaEnforced 在 multi_enforced 模式下由启动自检返回，供装配层
// 决定是否中止启动（阶段 0 装配层不中断，仅 Error 级日志；硬中止接线属批 B 部署立场拍板项）。
var ErrMultiReplicaEnforced = errors.New("declared_replicas > 1 with replica enforce enabled")

// RunReplicaSafetyStartupCheck 启动自检（P0-6 doNow ②）：
//   - 未声明/声明 1：静默返回 single 状态（零行为变化，一条日志都不发）。
//   - 声明 >1：对 9 项单副本边界清单逐项 Warn 结构化日志。
//   - >1 + ENFORCE：各项 Warn 后另发 Error 并返回 ErrMultiReplicaEnforced。
//
// 返回值同时写入包内快照供 healthz 查询。
func RunReplicaSafetyStartupCheck(lg loggateway.Logger, getenv func(string) string) (ReplicaSafetyStatus, error) {
	return runReplicaSafetyStartupCheck(lg, ResolveReplicaSafety(getenv))
}

// RunReplicaSafetyStartupCheckWithOverride 批 C Q3 conf 挂载入口：override =
// c.ParsedDeclaredReplicas()（>0 conf 优先），行为与 env 入口一致（单一
// 执行体防两套语义漂移）。
func RunReplicaSafetyStartupCheckWithOverride(lg loggateway.Logger, getenv func(string) string, override int32) (ReplicaSafetyStatus, error) {
	return runReplicaSafetyStartupCheck(lg, ResolveReplicaSafetyWithOverride(getenv, override))
}

func runReplicaSafetyStartupCheck(lg loggateway.Logger, st ReplicaSafetyStatus) (ReplicaSafetyStatus, error) {
	lastReplicaSafety.Store(st)
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if st.ParseError {
		lg.Warn("declared_replicas 环境变量无法解析，按未声明（单副本）处理",
			loggateway.Str("env", EnvDeclaredReplicas),
			loggateway.Str("component", "replica_guard"))
	}
	if st.Mode == CoordinationModeSingle {
		return st, nil
	}
	if st.Mode == CoordinationModeMultiEnforced {
		lg.Error("declared_replicas > 1 且 ENFORCE 开启：fail-fast 灰度触发",
			loggateway.Str("coordination_mode", st.Mode),
			loggateway.Int("declared_replicas", st.DeclaredReplicas))
		return st, ErrMultiReplicaEnforced
	}
	// multi_warn：逐项 Warn 降级（P0-6 doNow ②）。
	lg.Warn("declared_replicas > 1：以下功能仅在单副本下保证正确，进入 Warn 降级模式",
		loggateway.Str("coordination_mode", st.Mode),
		loggateway.Int("declared_replicas", st.DeclaredReplicas),
		loggateway.Str("docs", "docs/development/replica-safety.md"))
	for _, f := range singleReplicaFeatures() {
		lg.Warn("单副本边界功能降级", loggateway.Str("feature", f.Key),
			loggateway.Str("anchor", f.Anchor), loggateway.Str("desc", f.Description))
	}
	return st, nil
}

// CurrentReplicaSafety 返回 healthz 使用的当前快照；启动自检尚未运行时
// 静默按环境解析（不产生任何日志），保证 healthz 语义与实际一致。
func CurrentReplicaSafety() ReplicaSafetyStatus {
	if v := lastReplicaSafety.Load(); v != nil {
		if st, ok := v.(ReplicaSafetyStatus); ok {
			return st
		}
	}
	return ResolveReplicaSafety(os.Getenv)
}
