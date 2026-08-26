package biz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// VerificationReceipt 证据回执（ADR-79-V V3，2026-08-26，M79 R4）。
//
// V3 原则：同一 run 内，任何后续写操作（文件/记忆/外发/状态变更）使先前
// targeted verification 失效；失效项重验后方可 terminal。回执把每次门裁决
// 绑定到其计算所依据的活证据（目标 + 裁决 + 证据哈希），同 scope 写操作
// （RecordScopeWrite）将既有回执标记 Invalidated；terminal 前的全量重跑
// （ExecuteVerificationGates 失效重验段）以全新执行取代失效结论——结构性
// 保证「验证门只对活证据生效，写后必重验」。
type VerificationReceipt struct {
	Scope        string               // 门运行所属团队 ID（失效追踪的 run 维度）
	GateType     VerificationGateType `json:"gate_type"`
	Target       string               // 门目标身份（类型+agent/tool+args 规范化拼接）
	Approved     bool                 `json:"approved"`
	Reason       string               `json:"reason"`
	EvidenceHash string               // sha256(target \x00 teamOutput \x00 verdict) hex
	DecidedAt    int64                // Unix 秒
	Invalidated  bool                 // 同 scope 写操作后置真，terminal 前须重验
}

// verificationReceiptLedger 执行器内内存台账。key = scope + "\x00" + target，
// 同目标新裁决覆盖旧回执（supersede）。条目数受「团队数 × 每团队门数」约束
// （当前唯一自动门来源是 skill 安装 tool_assertion，每团队 ≤1 个），与 teams
// 表同阶，无需淘汰。进程内存态：V3 关注同 run 窗口，重启后回执无意义——
// terminal 的门本就对活证据全新执行，不依赖历史回执。
type verificationReceiptLedger struct {
	mu sync.Mutex
	by map[string]VerificationReceipt
}

func newVerificationReceiptLedger() *verificationReceiptLedger {
	return &verificationReceiptLedger{by: make(map[string]VerificationReceipt)}
}

// gateReceiptTarget 规范化门目标身份：同一逻辑门多次执行产生相同 target。
func gateReceiptTarget(gate VerificationGate) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		gate.GateType, gate.AgentID, gate.Tool, gate.ArgumentsJSON, gate.AssertPath, gate.AssertEquals)
}

// gateReceiptEvidenceHash 把裁决绑定到其输入证据。teamOutput 是 LLM 质量门的
// 证据源（tool_assertion 门不读产出文本，传空即证据为空）；verdict 分量使
// 同证据的不同裁决产生不同指纹，防止「旧通过回执」被误当「新裁决」复用。
func gateReceiptEvidenceHash(target, teamOutput string, approved bool, reason string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%t\x00%s", target, teamOutput, approved, reason)))
	return hex.EncodeToString(sum[:16])
}

// ExecuteGateScoped 执行单个验证门并记录证据回执（ADR-79-V V3）。error 路径
// 不记回执（infra 错误无裁决可绑定）。同 (scope, target) 的新裁决取代旧回执。
func (e *VerificationGateExecutor) ExecuteGateScoped(ctx context.Context, scope string, gate VerificationGate, teamOutput string, truncateChars int) (bool, string, VerificationReceipt, error) {
	approved, reason, err := e.ExecuteGate(ctx, gate, teamOutput, truncateChars)
	if err != nil {
		return false, "", VerificationReceipt{}, err
	}
	target := gateReceiptTarget(gate)
	receipt := VerificationReceipt{
		Scope:        scope,
		GateType:     gate.GateType,
		Target:       target,
		Approved:     approved,
		Reason:       reason,
		EvidenceHash: gateReceiptEvidenceHash(target, teamOutput, approved, reason),
		DecidedAt:    time.Now().Unix(),
	}
	e.receipts.mu.Lock()
	e.receipts.by[scope+"\x00"+target] = receipt
	e.receipts.mu.Unlock()
	return approved, reason, receipt, nil
}

// RecordScopeWrite 上报 scope 内一次写操作（交付物落库/状态变更等），将该
// scope 全部既有回执标记为 Invalidated（宁重勿轻：不做写目标与门目标的
// 相关性猜测，同 scope 写一律失效）。返回被失效的回执数。
func (e *VerificationGateExecutor) RecordScopeWrite(scope, writeTarget string) int {
	e.receipts.mu.Lock()
	defer e.receipts.mu.Unlock()
	n := 0
	for k, r := range e.receipts.by {
		if r.Scope != scope || r.Invalidated {
			continue
		}
		r.Invalidated = true
		r.Reason += fmt.Sprintf("（写后失效：%s）", writeTarget)
		e.receipts.by[k] = r
		n++
	}
	return n
}

// InvalidatedReceipts 返回 scope 内处于失效态的回执（terminal 前重验清单）。
func (e *VerificationGateExecutor) InvalidatedReceipts(scope string) []VerificationReceipt {
	e.receipts.mu.Lock()
	defer e.receipts.mu.Unlock()
	var out []VerificationReceipt
	for _, r := range e.receipts.by {
		if r.Scope == scope && r.Invalidated {
			out = append(out, r)
		}
	}
	return out
}
