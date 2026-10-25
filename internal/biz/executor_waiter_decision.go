package biz

import (
	"context"
	"strings"
	"time"
)

// ============================================================================
// ExecutorWaiterDecisionPort —— 跨副本执行等待器决议窄接口（P0-6 阶段 1b，
// 97-multi-replica-executor-waiters，deferred 战役 D6）
// ============================================================================
//
// 背景与边界（docs/reports/2026-10-25-plan-deferred-batch-implementation.md §7）：
// 阶段 1a 已把「看板租约」跨副本化（BoardRunLeasePort）；1b 收口「等
// 待器决议」——playbook 阶段确认（R18 confirm_before）与看板首次团队派发对账
//（P2-②）目前驻留进程内存（playbookConfirmDecided / startup sync.Map），
// waiter 在 A 副本、决议触达 B 副本时 A 永远等不到（或 dag ctx 超时落入
// abort 取消路径，把已批准决议误终为 cancelled）。
//
// 写语义（CAS，评审确认项钉死）：TryRecordDecision=INSERT ... ON CONFLICT
// DO NOTHING——0 行命中=别副本已决议（先到者赢），调用方 LoadDecision
// 一读拿现值；严禁 UPDATE 覆盖（防两管理员双副本 approve/deny 并发翻案）。
// LoadDecision 单读，无多跳语义。
//
// 行生命周期：无 sweeper——行量≈决议次数（HITL 确认/看板启动各每看板至多
// 一行，team 槽位快照每 team 至多一行），低频有界；看板清理链带删评估
// 记档 97 三件套后续批。
//
// 消费面全部藏在 ARANEA_EXECUTOR_WAITER=off|mem|pg（缺省 off 字节级旧
// 行为）之后；off 档本端口不接线（nil 注入）。
//
// Stability: internal
type ExecutorWaiterDecisionPort interface {
	// TryRecordDecision 原子记录决议：recorded=false（nil error）=同 key 决议
	// 已存在（先到者赢），调用方须 LoadDecision 读现值并收敛本进程缓存，
	// 禁止重试覆盖。
	TryRecordDecision(ctx context.Context, d ExecutorWaiterDecision) (recorded bool, err error)

	// LoadDecision 单读决议；found=false（nil error）=尚无决议。
	LoadDecision(ctx context.Context, key ExecutorWaiterDecisionKey) (dec ExecutorWaiterDecision, found bool, err error)
}

// ExecutorWaiterDecisionKey 决议键：kind 区分等待器种类；session_id 归属
// spirit session（可空——team 槽位快照按 team 寻址）；scope_id 细粒度对象
//（plan-step id / board id / team id）。
type ExecutorWaiterDecisionKey struct {
	Kind      string
	SessionID string
	ScopeID   string
}

// ExecutorWaiterDecision 是一条等待器决议的载体。
// Approved 承载 approve/ok；Reason 承载拒绝/失败原因（team_agent_loads 复用
// 为 agent key 快照，"," 拼接）；Actor 明文归属（接口/user id/内置子系统名）。
type ExecutorWaiterDecision struct {
	ExecutorWaiterDecisionKey
	Approved  bool
	Reason    string
	Actor     string
	DecidedAt time.Time
}

// 等待器决议 kind 常量。新 kind 取值写入即冻结，命名评审前置（生命周期语义
// 不同不得复用既有 kind）。
const (
	// WaiterDecisionKindPlaybookConfirm：R18 playbook confirm_before 阶段确认
	//（scope_id=plan-step id 或 PlaybookConfirmActivityID 卡 id，双键同写）。
	WaiterDecisionKindPlaybookConfirm = "playbook_confirm"
	// WaiterDecisionKindBoardStartup：P2-② 看板首次团队派发对账
	//（scope_id=board id；Approved=ok，Reason=失败原因）。
	WaiterDecisionKindBoardStartup = "board_startup"
	// WaiterDecisionKindTeamAgentLoads：DEV-03 slot 快照（scope_id=team id，
	// Reason=授位 agent keys "," 拼接）；供孤儿收养/对账后续批消费，本批
	// 仅落表（消费读写语义随收养链批次定稿）。
	WaiterDecisionKindTeamAgentLoads = "team_agent_loads"
)

// ValidateExecutorWaiterDecision 写入期校验：kind/scope_id 必填，
// session_id 可空、DecidedAt 缺省由实现侧补戳。
func ValidateExecutorWaiterDecision(d ExecutorWaiterDecision) error {
	if strings.TrimSpace(d.Kind) == "" {
		return errWaiterDecisionField("kind is required")
	}
	if strings.TrimSpace(d.ScopeID) == "" {
		return errWaiterDecisionField("scope_id is required")
	}
	return nil
}

type errWaiterDecisionField string

func (e errWaiterDecisionField) Error() string {
	return "executor waiter decision invalid: " + string(e)
}
