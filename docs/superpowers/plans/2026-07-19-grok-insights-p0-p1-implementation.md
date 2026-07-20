# Grok Build 借鉴功能 P0-P1 实施计划

> **Goal:** 将 Grok Build 对比分析中标记为 P0-P1 的 10 项改进落地到 Aranea-Agents 代码库
> **Architecture:** 逐个模块 TDD 实施，两阶段 review，验证后同步文档
> **Tech Stack:** Go + trpc-agent-go + Kratos v2

---

## 总体假设

1. **trpc-agent-go model 不变**：Aranea 依赖 trpc-agent-go 运行时，不改其内部实现，只在其暴露的接口层做适配
2. **向后兼容**：所有改动保持现有 API 兼容，新增字段/配置有默认值
3. **纯函数优先**：可提取为纯函数的逻辑优先独立成文件，方便单测
4. **错误分类不破坏现有流**：llmcompat 的错误 wrap 需保留原始错误类型，供重试分类器判断

---

## P0-1: Tool-pair 安全切分

**问题**: `partitionMessagesForCompression` 不感知 `ToolCalls`/`tool_call_id` 配对，evict 集合可能拆散 assistant tool_call 与其 tool result → API 400

**Files:**
- Modify: `internal/agent/context_compression_inject.go:146-200` (`partitionMessagesForCompression`)
- Test: `internal/agent/context_compression_inject_test.go` (新建或修改现有)

### Task P0-1

- [ ] **Step 1: Write the failing test**

```go
func TestPartitionMessagesForCompression_ToolPairSafety(t *testing.T) {
	// assistant 消息包含 tool_call，下一条是 tool result
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: "sys"},
		{Role: trpcmodel.RoleUser, Content: "u1"},
		{Role: trpcmodel.RoleAssistant, Content: "a1", ToolCalls: []trpcmodel.ToolCall{{ID: "tc1", Function: trpcmodel.ToolCallFunction{Name: "read"}}}},
		{Role: trpcmodel.RoleTool, Content: "r1", ToolCallID: "tc1"},
		{Role: trpcmodel.RoleUser, Content: "u2"},
	}
	keep, evicted := partitionMessagesForCompression(msgs, 0.30)
	// 如果按纯比例切，evicted 会包含 assistant tool_call 但不包含 tool result
	// 安全切分应把跨边界的 tool-pair 整体保留或整体驱逐
	for _, m := range evicted {
		if m.Role == trpcmodel.RoleAssistant && len(m.ToolCalls) > 0 {
			// 对应的 tool result 也必须在 evicted 中
			found := false
			for _, e := range evicted {
				if e.Role == trpcmodel.RoleTool && e.ToolCallID == m.ToolCalls[0].ID {
					found = true
					break
				}
			}
			t.Fatalf("evicted assistant tool_call %s without its tool_result", m.ToolCalls[0].ID)
		}
		if m.Role == trpcmodel.RoleTool && m.ToolCallID != "" {
			// 对应的 tool_call 也必须在 evicted 中
			found := false
			for _, e := range evicted {
				if e.Role == trpcmodel.RoleAssistant {
					for _, tc := range e.ToolCalls {
						if tc.ID == m.ToolCallID {
							found = true
							break
						}
					}
				}
			}
			t.Fatalf("evicted tool_result %s without its tool_call", m.ToolCallID)
		}
	}
	// keep 侧同理
	for _, m := range keep {
		if m.Role == trpcmodel.RoleAssistant && len(m.ToolCalls) > 0 {
			found := false
			for _, k := range keep {
				if k.Role == trpcmodel.RoleTool && k.ToolCallID == m.ToolCalls[0].ID {
					found = true
					break
				}
			}
			t.Fatalf("kept assistant tool_call %s without its tool_result", m.ToolCalls[0].ID)
		}
		if m.Role == trpcmodel.RoleTool && m.ToolCallID != "" {
			found := false
			for _, k := range keep {
				if k.Role == trpcmodel.RoleAssistant {
					for _, tc := range k.ToolCalls {
						if tc.ID == m.ToolCallID {
							found = true
							break
						}
					}
				}
			}
			t.Fatalf("kept tool_result %s without its tool_call", m.ToolCallID)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/... -run TestPartitionMessagesForCompression_ToolPairSafety -count=1 -v`
Expected: FAIL (当前实现不感知 ToolCalls)

- [ ] **Step 3: Write minimal implementation**

修改 `partitionMessagesForCompression`：在确定 evict 边界后，检查边界处的 assistant `ToolCalls` 是否有对应的 tool result 在 evicted 中，如果没有则把 tool result 也加入 evicted（或整体保留在 keep 中——策略选择：把跨边界的不完整 pair 整体保留在 keep 侧更安全，因为 keep 侧是最近消息）。

- [ ] **Step 4: Run test to verify it passes**

- [ ] **Step 5: Add edge case tests**

- 多 tool call 的情况
- tool result 在 assistant 之前（异常顺序但需处理）
- 没有 tool message 的场景（不影响现有行为）

- [ ] **Step 6: Commit**

---

## P0-2: 日志密钥清洗入 Pipeline

**问题**: `tools/preview/preview.go` 仅 4 个正则且未接入 loggateway Pipeline，API key 可能泄漏到日志/遥测

**Files:**
- Modify: `internal/tools/preview/preview.go`
- Modify: `pkg/logpipeline/pipeline.go` 或新增 `pkg/logpipeline/sanitizer.go`
- Modify: `pkg/loggateway/logger.go` (新增 Sanitize 选项或中间件)
- Test: `internal/tools/preview/preview_test.go`

### Task P0-2

- [ ] **Step 1: Write the failing test**

```go
func TestRedactAndTruncate_APIKeyPatterns(t *testing.T) {
	cases := []struct {
		input    string
		wantContain string
	}{
		{"sk-abc123def456", "[secret redacted]"},
		{"xai-abc123", "[secret redacted]"},
		{"AKIAIOSFODNN7EXAMPLE", "[secret redacted]"},
		{"ghp_xxxxxxxxxxxx", "[secret redacted]"},
		{"AIzaSyA123", "[secret redacted]"},
		{"Authorization: Bearer eyJ...", "[secret redacted]"},
	}
	for _, c := range cases {
		got := RedactAndTruncate(c.input, 10000)
		if !strings.Contains(got, c.wantContain) {
			t.Errorf("RedactAndTruncate(%q) did not contain %q, got: %q", c.input, c.wantContain, got)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: 扩充 preview.go 正则**

将 4 个正则扩充为 10 类模式（带 `\b` 锚定防过杀）：
1. `sk-`/`sk_`/`xai-` 厂商 key
2. AWS AKIA/ASIA
3. GitHub PAT
4. GitLab/Slack token
5. Google AIza
6. PEM 私钥块
7. Bearer token
8. 裸 JWT
9. `api_key|token|secret|password` 赋值（8 字符下限）

- [ ] **Step 4: 接入 loggateway Pipeline**

新增 `SanitizingSink`（wrap 任意 Sink）或 `SanitizingFormatter`：在日志写入前对所有 msg 和 fields 的值调用 `RedactAndTruncate`。

```go
type sanitizingSink struct {
	base Sink
}
func (s *sanitizingSink) Write(rec Record) error {
	rec.Msg = preview.RedactAndTruncate(rec.Msg, 0)
	for i := range rec.Fields {
		if rec.Fields[i].Type == StringField {
			rec.Fields[i].Str = preview.RedactAndTruncate(rec.Fields[i].Str, 0)
		}
	}
	return s.base.Write(rec)
}
```

- [ ] **Step 5: Run tests**

- [ ] **Step 6: Commit**

---

## P0-3: 熔断器探针遗弃回收

**问题**: `biz/tool/circuit_breaker.go:101` `halfOpenProbes++` 无回收路径，若探针 owner 的 future 被取消且 Record 未调用，槽位永久泄漏困死 HalfOpen

**Files:**
- Modify: `internal/biz/tool/circuit_breaker.go`
- Test: `internal/biz/tool/circuit_breaker_test.go`

### Task P0-3

- [ ] **Step 1: Write the failing test**

```go
func TestCircuitBreaker_ProbeAbandonmentRecovery(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{
		FailureThreshold: 1, RecoveryTimeoutSec: 1, HalfOpenMaxProbe: 1,
	})
	cb.RecordFailure() // Open
	time.Sleep(1100 * time.Millisecond)
	// First Allow transitions to HalfOpen and reserves the only probe slot
	allowed, _ := cb.Allow()
	if !allowed {
		t.Fatal("expected Allow after recovery timeout")
	}
	// Simulate probe abandonment: do NOT call RecordSuccess/RecordFailure
	// Wait for recovery timeout again
	time.Sleep(1100 * time.Millisecond)
	// Without abandonment recovery, this would be false (probe slot leaked)
	allowed2, _ := cb.Allow()
	if !allowed2 {
		t.Fatal("expected Allow after probe abandonment recovery")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write minimal implementation**

在 `CircuitBreaker` 增加：
```go
type CircuitBreaker struct {
	// ... existing fields ...
	probeClaimedAt time.Time // 最近一次探针槽位认领时间
}
```

在 `Allow()` HalfOpen 分支中：
```go
case CircuitHalfOpen:
	// 检查是否有遗弃的探针槽位可回收
	if !cb.probeClaimedAt.IsZero() && time.Since(cb.probeClaimedAt) > cb.config.recoveryTimeout() {
		cb.halfOpenProbes-- // 回收遗弃槽位
		cb.probeClaimedAt = time.Time{}
	}
	if cb.halfOpenProbes < cb.config.HalfOpenMaxProbe {
		cb.halfOpenProbes++
		cb.probeClaimedAt = time.Now()
		return true, cb.state
	}
	return false, cb.state
```

- [ ] **Step 4: Run test to verify it passes**

- [ ] **Step 5: Commit**

---

## P0-4: 双锚点 token 估算收口

**问题**: `estTokensFromChars` 统一 2.5 chars/token 混合比率，无模型权威值回填，多处独立估算

**Files:**
- Create: `internal/llmcontext/token_estimator.go` (统一估算器)
- Modify: `internal/agent/prompt_snapshot.go:130` (替换 estTokensFromChars)
- Modify: `internal/agent/context_compression_inject.go:84` (使用统一估算器)
- Modify: `internal/session/compress_policy.go` (使用统一估算器)
- Test: `internal/llmcontext/token_estimator_test.go`

### Task P0-4

- [ ] **Step 1: Write the failing test**

```go
func TestTokenEstimator_DualAnchor(t *testing.T) {
	e := NewTokenEstimator()
	// Before any authoritative value
	if e.EstimateChars(100) != 40 { // 100/2.5
		t.Error("default estimate mismatch")
	}
	// After authoritative value
	e.RecordAuthoritative(250, 1000) // 250 tokens for 1000 chars = 4.0 chars/token
	if e.EstimateChars(100) != 25 { // 100/4.0
		t.Error("authoritative estimate mismatch")
	}
	// Incremental estimate since last authoritative
	e.RecordIncremental(10) // 10 bytes since last authoritative
	got := e.EstimateTotal()
	// total = 250 + 10/4 = 252 or 253
	if got < 252 || got > 253 {
		t.Errorf("incremental estimate mismatch: got %d", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write minimal implementation**

```go
package llmcontext

type TokenEstimator struct {
	authoritativeTokens int
	authoritativeChars  int
	incrementalBytes    int
}

func NewTokenEstimator() *TokenEstimator { return &TokenEstimator{} }

func (e *TokenEstimator) RecordAuthoritative(tokens, chars int) {
	e.authoritativeTokens = tokens
	e.authoritativeChars = chars
	e.incrementalBytes = 0
}

func (e *TokenEstimator) RecordIncremental(bytes int) {
	e.incrementalBytes += bytes
}

func (e *TokenEstimator) EstimateChars(chars int) int {
	charsPerToken := 2.5
	if e.authoritativeChars > 0 && e.authoritativeTokens > 0 {
		charsPerToken = float64(e.authoritativeChars) / float64(e.authoritativeTokens)
	}
	tokens := int(float64(chars) / charsPerToken)
	if tokens == 0 && chars > 0 {
		return 1
	}
	return tokens
}

func (e *TokenEstimator) EstimateTotal() int {
	return e.authoritativeTokens + int(float64(e.incrementalBytes)/4.0)
}
```

- [ ] **Step 4: Replace existing estTokensFromChars usages**

- [ ] **Step 5: Run all tests**

- [ ] **Step 6: Commit**

---

## P1-1: LLM 重试分类纯函数

**问题**: 重试决策硬编码在 `RoundTrip` 循环里，全项目错误分类散落 4+ 处，llmcompat 把所有错误 wrap 成 `apierror.Internal`

**Files:**
- Create: `internal/provider/retry_classifier.go`
- Modify: `internal/provider/retry_transport.go`
- Modify: `internal/agent/llmcompat/llmcompat.go` (保留原始错误类型)
- Test: `internal/provider/retry_classifier_test.go`

### Task P1-1

- [ ] **Step 1: Write the failing test**

```go
func TestClassifyRetry_429(t *testing.T) {
	decision := ClassifyRetry(&http.Response{StatusCode: 429}, nil)
	if decision.Type != RetryWithBackoff {
		t.Errorf("expected RetryWithBackoff for 429, got %v", decision.Type)
	}
}

func TestClassifyRetry_ContextOverflow(t *testing.T) {
	err := errors.New("context length exceeded")
	decision := ClassifyRetry(nil, err)
	if decision.Type != RetryFatal {
		t.Errorf("expected RetryFatal for context overflow, got %v", decision.Type)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write minimal implementation**

```go
package provider

type RetryDecisionType int

const (
	Retry RetryDecisionType = iota
	RetryWithBackoff
	RetryWithImageStrip
	RetryWithClientRebuild
	EmitToSession
	RetryFatal
)

type RetryDecision struct {
	Type            RetryDecisionType
	IsRateLimited   bool
	MaxAttempts     int
	BackoffStrategy string
}

func ClassifyRetry(resp *http.Response, err error) RetryDecision {
	// 纯函数，无 I/O，无 logging
	// ... 6 态决策逻辑 ...
}
```

- [ ] **Step 4: Integrate into retry_transport**

- [ ] **Step 5: Modify llmcompat to preserve error types**

- [ ] **Step 6: Run all tests**

- [ ] **Step 7: Commit**

---

## P1-2: Doom Loop 检测

**问题**: 全仓库无 LLM 层循环检测

**Files:**
- Create: `internal/agent/doom_loop_detector.go`
- Modify: `internal/agent/stream_consumer.go` 或 trpc-agent-go 适配层
- Test: `internal/agent/doom_loop_detector_test.go`

### Task P1-2

- [ ] **Step 1: Write the failing test**

```go
func TestDoomLoopDetector_DetectsRepetition(t *testing.T) {
	d := NewDoomLoopDetector(3, 0.95) // 3 repeats, 95% similarity
	texts := []string{
		"I need to check the file.",
		"I need to check the file.",
		"I need to check the file.",
	}
	for _, text := range texts {
		if d.Observe(text) {
			t.Log("Doom loop detected")
			return
		}
	}
	t.Fatal("expected doom loop detection after 3 identical texts")
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write minimal implementation**

```go
type DoomLoopDetector struct {
	window          []string
	threshold       int
	similarityThreshold float64
}

func (d *DoomLoopDetector) Observe(text string) bool {
	// 维护最近 N 条文本窗口
	// 计算与窗口内文本的相似度（简单实现：精确匹配或 Jaccard）
	// 如果连续 threshold 条文本相似度 > similarityThreshold，返回 true
}
```

- [ ] **Step 4: Integrate into stream processing**

在 stream consumer 中：每收到一个 delta chunk，提取文本送入 detector，检测到 doom loop 时触发 abort + 重采样。

- [ ] **Step 5: Run tests**

- [ ] **Step 6: Commit**

---

## P1-3: Reminder 机制 ✅（2026-07-20 完成，含隔离性修复）

**问题**: 工具执行后无副作用反馈回 Agent

**Files:**
- Create: `internal/agent/tool_reminder.go`
- Modify: `internal/agent/callback_chain.go`（注册 BeforeAgent + AfterTool 钩子）
- Test: `internal/agent/tool_reminder_test.go`

**实施记录（与原始计划的偏差）**:
- Reminder 通过 **AfterTool 钩子追加到工具结果文本**（`[reminder] ...`），而非 turn 结束注入 system message——LLM 在工具响应中直接看到副作用反馈。
- 测试运行（命令含 "test"）清除提醒；文件修改类工具（write/edit/patch/delete/create/rename/move）武装提醒。
- **隔离性修复**：初版在闭包内共享单个 `ToolReminder` 实例，随 Agent 缓存被多会话共享 → 跨会话状态污染。修复为 `BeforeAgent` 钩子按 invocation 预创建实例存入 state（`aranea.tool_reminder`），`AfterTool` 从 invocation state 解析（缺失时惰性创建兜底）。子 invocation（Clone）共享父实例为预期行为（同一 run 作用域）。
- 测试覆盖：5 个结构单测 + 3 个钩子级测试（invocation 隔离、惰性初始化、无 invocation 安全 no-op）。

### Task P1-3

- [x] **Step 1: Write the failing test**

```go
func TestToolReminder_CollectsReminders(t *testing.T) {
	r := NewToolReminder()
	// 模拟工具执行结果：文件修改但未测试
	r.OnToolExecuted("edit_file", map[string]string{"path": "/foo.go"})
	reminders := r.Collect()
	if len(reminders) == 0 {
		t.Fatal("expected reminders for unverified file edit")
	}
}
```

- [x] **Step 2: Run test to verify it fails**

- [x] **Step 3: Write minimal implementation**

```go
type ToolReminder struct {
	editsWithoutTests []string
}

func (r *ToolReminder) OnToolExecuted(name string, params map[string]string) {
	if name == "edit_file" || name == "write_file" {
		if path, ok := params["path"]; ok {
			// 记录文件修改
			// 如果后续没有 run_test 或 run_command("go test")，生成 reminder
		}
	}
}

func (r *ToolReminder) Collect() []string {
	// 返回 collected reminders
}
```

- [x] **Step 4: Integrate into turn end**

实际实现：AfterTool 钩子追加到工具结果（见上方实施记录）。

- [x] **Step 5: Run tests**

- [x] **Step 6: Commit**

---

## P1-4: 工具行为版本化

**问题**: 工具行为升级直接影响所有会话，会话复现不一致

**Files:**
- Modify: `tools/toolset.go` (ToolRegistration 加 behavior_version)
- Modify: `internal/biz/session.go` (会话锁定工具版本)
- Modify: `internal/data/ent/schema/` (可能需加字段)
- Test: `tools/toolset_test.go`

### Task P1-4

- [ ] **Step 1: Write the failing test**

```go
func TestToolRegistry_BehaviorVersioning(t *testing.T) {
	reg := NewRegistry()
	reg.Register(ToolRegistration{Name: "read", BehaviorVersion: 1})
	reg.Register(ToolRegistration{Name: "read", BehaviorVersion: 2})
	
	// Session created at v1 should always get v1
	v1Tool := reg.Resolve("read", 1)
	if v1Tool == nil || v1Tool.BehaviorVersion != 1 {
		t.Fatal("expected behavior version 1")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

- [ ] **Step 3: Write minimal implementation**

在 `ToolRegistration` 增加 `BehaviorVersion int`。
在 `Registry` 中按 `(name, version)` 索引工具。
会话创建时锁定当前最新版本。

- [ ] **Step 4: Run tests**

- [ ] **Step 5: Commit**

---

## P1-5: 记忆搜索管线增强 ✅（2026-07-20 核查确认已实现）

**问题**: 搜索管线阶段少于 Grok Build

**Files:**
- Implement: `internal/data/memory_helpers.go`（decayFactor / factRecencyDecay / factDecayWithKind / isEvergreenFactKind / recencyBoost）
- Implement: `internal/data/memory_shim_l2.go`（L2 混合评分接入 decay）、`internal/data/memory_shim_l3.go`（L3 混合评分接入 decay + recency）
- Implement: `internal/data/memory_composite_adapter.go`（MMR 多样性重排，λ=0.7）
- Test: `internal/data/memory_mmr_test.go`

**核查结论（2026-07-20）**：全部能力已实现且测试通过（8/8），无需新代码。
- **session 半衰**：L2 episode `decayFactor(endedAt)` 半衰期 14 天（`l2DecayHalfLifeDays=14.0`），作用于 hybrid 评分 importance 项；同会话行另有 sessionBoost=0.1。
- **L3 时间衰减**：`factRecencyDecay(updatedAt)` 半衰期 30 天（`l3DecayHalfLifeDays=30.0`）。
- **evergreen 豁免**：`isEvergreenFactKind` — user_identity / user_preference / agent_instruction / domain_knowledge 四类恒返回 1.0（永不衰减）。
- **recency boost**：≤7 天 1.0 / ≤30 天 0.5 / 之后 0.1（`l3ScoreWeightRecency=0.15`）。
- **MMR 多样性重排**：composite recall 在 sort 后执行 `mmrRerankTexts`（λ=0.7）。

### Task P1-5

- [x] **Step 1: Write the failing test**

```go
func TestMemorySearch_TimeDecay(t *testing.T) {
	// 较旧的记忆应得到较低的分数
}

func TestMemorySearch_MMR(t *testing.T) {
	// 多样性重排：相似的记忆不应全部排在前面
}
```

- [x] **Step 2: Run test to verify it fails**

- [x] **Step 3: Write minimal implementation**

增加时间衰减函数（evergreen 豁免、session 半衰）和 MMR 多样性重排。

- [x] **Step 4: Run tests**

- [x] **Step 5: Commit**

---

## P1-6: 配置错误脱敏 ✅（2026-07-20 完成）

**问题**: 配置加载错误可能泄漏敏感配置值

**Files:**
- Modify: `cmd/admin/main.go`（新增 `redactConfigError`，接入 Load/Scan/wireApp 三处 panic 点）
- Test: `cmd/admin/main_test.go`（新建）

**实施记录**:
- 现状核查发现 CLI 配置路径已脱敏（`internal/cli/config/config.go` 的 `sanitizeConfigError` → `preview.RedactAndTruncate`），缺口在**服务端启动路径**。
- YAML v3 类型错误会回显标量值（`cannot unmarshal !!str 'sk-...' into int`），DSN 错误含密码（`postgres://user:pass@host`）——两类泄漏面均由 `preview.RedactAndTruncate` 覆盖。
- `redactConfigError(op, err)` 包裹三处启动 panic 点：`c.Load()`、`c.Scan(&bc)`、`wireApp(...)`；运行时 `App.Run()` 错误不属配置面，未包裹。
- 测试覆盖：YAML 值回显脱敏、非 secret 透传、DSN 密码脱敏、nil 安全。

### Task P1-6

- [x] **Step 1: Locate config loading error handling**

- [x] **Step 2: Write the failing test**

```go
func TestConfigError_Redacted(t *testing.T) {
	// 模拟包含 API key 的无效配置
	// 错误信息不应包含原始 API key
}
```

- [x] **Step 3: Run test to verify it fails**

- [x] **Step 4: Write minimal implementation**

在配置解析错误处理中，对错误消息调用 `preview.RedactAndTruncate`。

- [x] **Step 5: Run tests**

- [x] **Step 6: Commit**

---

## 文档同步计划

每个 P0/P1 任务完成后，同步更新以下文档：

1. **需求文档**: `docs/development/` 下对应模块的需求文档（`.md`）
2. **设计文档**: `docs/development/` 下对应模块的设计文档（`.design.md`）
3. **开发计划**: `docs/development/` 下对应模块的开发计划（`.development.md`）
4. **对比文档**: `docs/reports/2026-07-19-analysis-grok-build-function-by-function-comparison.md`（标记已完成项）

---

## Review 检查点

每个任务完成后：

### Phase A: Spec Compliance
- [ ] 是否解决了该 P0/P1 项描述的问题？
- [ ] 是否有回归测试覆盖？
- [ ] 是否保持了向后兼容？

### Phase B: Code Quality
- [ ] 是否遵循 aranea-coding-guide 红线？
- [ ] 错误处理是否使用 apierror？
- [ ] 日志是否使用 loggateway.Logger？
- [ ] 是否有并发安全问题？
- [ ] 单测是否独立（无 DB/network 依赖）？

### Phase C: Verification
- [ ] `go test ./internal/... -count=1` 通过
- [ ] `make lint` 通过
- [ ] `make build` 通过
- [ ] 文档已同步
