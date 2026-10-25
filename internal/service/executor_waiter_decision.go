package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
)

// executor_waiter_decision.go —— P0-6 阶段 1b（D6，
// 97-multi-replica-executor-waiters）：
//   - capability flag（ARANEA_EXECUTOR_WAITER）档位纯函数解析
//     （ResolveExecutorWaiterMode，对齐 ResolveBoardRunLeaseMode 先例：
//     非法值一律按缺省 off，fail-safe=不认得档=旧行为）；
//   - biz.ExecutorWaiterDecisionPort 的进程内实现（mem 档——端口语义
//     等价性灰度，无跨进程语义）。
//
// flag 档位：""（缺省）= 端口不接线（waiter 决议纯进程内 playbookConfirmDecided
// 内存位 + confirmSteps 兜底，无 300ms 轮询分支，dispatchStep 单 chan
// select 零 DB 读——字节级旧行为）；mem=InMem 试跑档；pg=executor_waiter_decisions
// 表（迁移 20261302）。
const (
	EnvExecutorWaiterMode = "ARANEA_EXECUTOR_WAITER"

	ExecutorWaiterModeOff = ""
	ExecutorWaiterModeMem = "mem"
	ExecutorWaiterModePG  = "pg"

	// executorWaiterPollInterval 是跨副本决议轮询拍（playbook confirm ticker
	// 与 team 完成 poller 共用）：300ms——HITL 验收断言「决策副本 B→waiter
	// 副本 A ≤1s 推进」留 3 拍余量；DB 读扩增≈3.3 次/s/挂起 waiter，行级
	// 主键单读，代价与 board lease 心跳（ttl/2）同量级。
	executorWaiterPollInterval = 300 * time.Millisecond
)

// ResolveExecutorWaiterMode 纯函数解析 flag（无副作用，nil getenv 回落
// os.Getenv）；非法值一律按缺省 off。
func ResolveExecutorWaiterMode(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(EnvExecutorWaiterMode))) {
	case ExecutorWaiterModeMem:
		return ExecutorWaiterModeMem
	case ExecutorWaiterModePG:
		return ExecutorWaiterModePG
	default:
		return ExecutorWaiterModeOff
	}
}

// InMemExecutorWaiterDecisions 是 biz.ExecutorWaiterDecisionPort 的进程内
// 实现（mem 档）：LoadOrStore CAS 先到者赢；写入期沿用单源校验。
// 边界声明：不提供任何跨进程语义（对照 BoardRunLeasePort 的 InMem 先例）。
type InMemExecutorWaiterDecisions struct {
	m sync.Map // executorWaiterDecisionKeyString → biz.ExecutorWaiterDecision
}

// NewInMemExecutorWaiterDecisions 构造进程内实现。
func NewInMemExecutorWaiterDecisions() *InMemExecutorWaiterDecisions {
	return &InMemExecutorWaiterDecisions{}
}

var _ biz.ExecutorWaiterDecisionPort = (*InMemExecutorWaiterDecisions)(nil)

func inMemWaiterDecisionKey(k biz.ExecutorWaiterDecisionKey) string {
	return fmt.Sprintf("%s\x00%s\x00%s",
		strings.TrimSpace(k.Kind), strings.TrimSpace(k.SessionID), strings.TrimSpace(k.ScopeID))
}

func (s *InMemExecutorWaiterDecisions) TryRecordDecision(_ context.Context, dec biz.ExecutorWaiterDecision) (bool, error) {
	if s == nil {
		return false, nil
	}
	if err := biz.ValidateExecutorWaiterDecision(dec); err != nil {
		return false, err
	}
	_, loaded := s.m.LoadOrStore(inMemWaiterDecisionKey(dec.ExecutorWaiterDecisionKey), dec)
	return !loaded, nil
}

func (s *InMemExecutorWaiterDecisions) LoadDecision(_ context.Context, key biz.ExecutorWaiterDecisionKey) (biz.ExecutorWaiterDecision, bool, error) {
	if s == nil {
		return biz.ExecutorWaiterDecision{}, false, nil
	}
	v, ok := s.m.Load(inMemWaiterDecisionKey(key))
	if !ok {
		return biz.ExecutorWaiterDecision{}, false, nil
	}
	dec, _ := v.(biz.ExecutorWaiterDecision)
	return dec, true, nil
}
