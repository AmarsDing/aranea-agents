# Session 压缩加固（Grok 借鉴）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Session 压缩的 tail 丢失缺陷，并从 Grok xai-grok-compaction 移植递归滚动摘要、摘要质量门、失败抑制、双锚点校准四项能力。

**Architecture:** 全部改动集中在 `internal/session`（压缩主链路）+ `internal/compress`（LLM 调用）+ `internal/biz/session`（接口与门面）+ `internal/data`（Repo 实现）。纯逻辑（错误分类/退化检测/减量守卫/抑制判定）提取为零副作用纯函数，可穷举单测。

**Tech Stack:** Go + Ent + testify-less 标准 testing + loggateway

**背景与发现：**
- 🔴 缺陷（已验证）：`compressor.go:594-598` 从 `body`（全部 `TurnNumber <= cutoffTurn`）中过滤 `TurnNumber > cutoffTurn`，条件恒假 → 压缩后 runner 快照只剩摘要，最近 N 轮对话被静默丢弃。
- 🟡 `RecordAuthoritativeUsage`（`llmcontext/token_estimator.go:82`）零调用点，双锚点校准未接线。
- 🟡 摘要无限拼接：L3 不传 `PriorSummary`，事后字符串拼接所有历史摘要，无再压缩机制。
- 🟡 摘要质量零防御：无退化检测/减量守卫/错误分类。
- 🟡 确定性失败（如 401/上下文溢出）后无任何抑制，下个 turn 立即重试。
- 📋 发现但不修（超出本计划范围，见文末「后续建议」）：L1 MicroCompact 实际永不触发（body 只含 user/assistant，`tryMicroCompact` 找 `role=="tool"` 恒为 0）；系统 A/B 双压缩系统统一属架构决策。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试，再写最小实现。
- **验证命令**（每个 Phase 收尾必跑）：
  - `go build ./...`
  - `go test ./internal/session/... ./internal/compress/... ./internal/biz/session/... -count=1`
- **提交前全量**：`make build && make test && make lint`
- **日志规范**：用 `c.lg` + `loggateway.StepID("session.compress")` + 结构化字段，禁拼接。
- **接口纪律**：`SummaryWriter` 标注 `Stability:stable`，按 AS-STA-01 只允许新增方法（本计划新增 1 个 `DeleteSessionSummaries`）。
- **review 纪律**：每个 Phase 完成后做规格合规 + 代码质量两阶段自审，发现问题立即修复再继续。

---

## Phase 1（P0）：修复 tail 恒为空缺陷

**目标**：`loadCompressBody` 把保留区（`timeline[split:]`）作为 tail 显式返回，穿透 `runCompress → executeCompression → compressInTransaction`，删除恒假的死过滤。

**Files:**
- Modify: `internal/session/compressor.go:466-477`（runCompress）、`:502-533`（loadCompressBody）、`:537-570`（executeCompression）、`:573-629`（compressInTransaction）
- Create: `internal/session/compressor_test.go`

- [x] **Step 1.1: 写失败测试 `TestLoadCompressBody_returnsBodyAndTail`**

在新建的 `internal/session/compressor_test.go` 中：

```go
package session

import (
	"context"
	"strconv"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- narrow mocks for compressor deps ---

type stubCompressReadDeps struct {
	maxSummarized int
	msgs          []biz.ChatMessage
}

func (s *stubCompressReadDeps) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	return biz.Session{ID: id}, nil
}
func (s *stubCompressReadDeps) ListMessagesAfterTurn(_ context.Context, _ string, afterTurn int) ([]biz.ChatMessage, error) {
	var out []biz.ChatMessage
	for _, m := range s.msgs {
		if m.TurnNumber > afterTurn {
			out = append(out, m)
		}
	}
	return out, nil
}
func (s *stubCompressReadDeps) MaxSessionSummaryToTurn(_ context.Context, _ string) (int, error) {
	return s.maxSummarized, nil
}
func (s *stubCompressReadDeps) ListSessionSummaries(_ context.Context, _ string) ([]biz.SessionSummary, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) LatestSessionSummaryTime(_ context.Context, _ string) (string, error) {
	return "", nil
}

// makeTimeline builds alternating user/assistant messages, 2 per turn, turns 1..n.
func makeTimeline(turns int) []biz.ChatMessage {
	var out []biz.ChatMessage
	for t := 1; t <= turns; t++ {
		out = append(out,
			biz.ChatMessage{ID: "u" + strconv.Itoa(t), Role: "user", ContentMarkdown: "q" + strconv.Itoa(t), TurnNumber: t, CreatedAt: "2026-07-20T10:00:0" + strconv.Itoa(t) + "Z"},
			biz.ChatMessage{ID: "a" + strconv.Itoa(t), Role: "assistant", ContentMarkdown: "ans" + strconv.Itoa(t), TurnNumber: t, CreatedAt: "2026-07-20T10:01:0" + strconv.Itoa(t) + "Z"},
		)
	}
	return out
}

func newTestCompressor(read *stubCompressReadDeps) *Compressor {
	return &Compressor{
		deps: compressDeps{
			summaryReader: read,
			messageReader: read,
			sessionReader: read,
		},
		lg: loggateway.NewNoop(),
	}
}

func TestLoadCompressBody_returnsBodyAndTail(t *testing.T) {
	// 5 turns × 2 msgs = 10 rows; default keepTurns=4 → keepRows=8 → split=2.
	// body = turn 1 (2 rows), tail = turns 2..5 (8 rows).
	read := &stubCompressReadDeps{maxSummarized: 0, msgs: makeTimeline(5)}
	c := newTestCompressor(read)

	body, tail, cutoffTurn, err := c.loadCompressBody(context.Background(), biz.Session{}, biz.Agent{}, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if cutoffTurn != 1 {
		t.Fatalf("cutoffTurn = %d, want 1", cutoffTurn)
	}
	if len(body) != 2 {
		t.Fatalf("body len = %d, want 2", len(body))
	}
	if len(tail) != 8 {
		t.Fatalf("tail len = %d, want 8（近期轮次必须保留）", len(tail))
	}
	for _, m := range tail {
		if m.TurnNumber <= cutoffTurn {
			t.Fatalf("tail 中混入已压缩消息: %+v", m)
		}
	}
}
```

- [x] **Step 1.2: 跑测试确认编译失败**

Run: `go test ./internal/session/ -run TestLoadCompressBody_returnsBodyAndTail -count=1`
Expected: 编译失败 —— `loadCompressBody` 当前只返回 3 个值。

- [x] **Step 1.3: 修改 `loadCompressBody` 返回 tail**

```go
// loadCompressBody loads and splits messages for compression.
// Returns the body messages to compress, the tail messages to keep verbatim,
// and the cutoff turn number.
func (c *Compressor) loadCompressBody(ctx context.Context, sess biz.Session, ag biz.Agent, sessionID string) (body, tail []biz.ChatMessage, cutoffTurn int, err error) {
	maxSummarized, err := c.deps.summaryReader.MaxSessionSummaryToTurn(ctx, sessionID)
	if err != nil {
		return nil, nil, 0, err
	}
	msgs, err := c.deps.messageReader.ListMessagesAfterTurn(ctx, sessionID, maxSummarized)
	if err != nil {
		return nil, nil, 0, err
	}
	timeline := timelineUserAssistant(msgs)
	if len(timeline) == 0 {
		return nil, nil, 0, nil
	}

	_, keepTurns := compressThresholdAndKeep(ag)
	keepRows := messagesPerTurn * max(1, keepTurns)
	if len(timeline) <= keepRows {
		return nil, nil, 0, nil
	}
	split := len(timeline) - keepRows
	cutoffTurn = timeline[split-1].TurnNumber

	for _, m := range timeline {
		if m.TurnNumber > maxSummarized && m.TurnNumber <= cutoffTurn {
			body = append(body, m)
		}
	}
	tail = timeline[split:]
	return body, tail, cutoffTurn, nil
}
```

注意：`sess` 参数当前未被使用（现状如此），保持签名稳定，用 `_ biz.Session`？——不，保持现有参数名 `sess` 不动（本 Phase 不做无关清理）。如果编译器未使用参数告警不存在（Go 不检查未使用参数）。

- [x] **Step 1.4: 穿透 `runCompress` / `executeCompression` / `compressInTransaction`**

`runCompress`（:466-477）：

```go
	body, tail, cutoffTurn, err := c.loadCompressBody(ctx, sess, ag, sessionID)
	if err != nil || len(body) == 0 {
		return err
	}

	// Three-level compression cascade: MicroCompact → MemoryCompact → LLM.
	level, md := c.compressCascade(ctx, sess, ag, body, sessionID, cutoffTurn, usedTokens, hardTok)
	if level == compressLevelNone || md == "" {
		return nil
	}

	return c.executeCompression(ctx, sess, ag, body, tail, md, sessionID, trpcUserID, cutoffTurn)
```

`executeCompression` 签名加 `tail`：

```go
func (c *Compressor) executeCompression(ctx context.Context, sess biz.Session, ag biz.Agent, body, tail []biz.ChatMessage, md string, sessionID, trpcUserID string, cutoffTurn int) error {
	fromTurn := body[0].TurnNumber
	toTurn := body[len(body)-1].TurnNumber
	// ... CAS 与幂等检查不变 ...
	txMerged, txTail, txErr := c.compressInTransaction(ctx, sessionID, ag, sess, body, tail, md, fromTurn, toTurn, cutoffTurn)
```

`compressInTransaction` 删除死过滤，改用传入的 tail：

```go
func (c *Compressor) compressInTransaction(ctx context.Context, sessionID string, ag biz.Agent, sess biz.Session, body, tail []biz.ChatMessage, md string, fromTurn, toTurn, cutoffTurn int) (mergedSummary string, tailMsgs []biz.ChatMessage, err error) {
	err = c.deps.compressRepo.CompressSessionInTx(ctx, sessionID, func(txCtx context.Context) error {
		// ... 插入摘要行 + ListSessionSummaries + merge 不变 ...

		// tail 直接来自 loadCompressBody 的保留区，不再从 body 死过滤。
		tailMsgs = tail

		// ... 后续 RewriteSnapshotWithCompression 等不变 ...
	})
	return mergedSummary, tailMsgs, err
}
```

注意：`compressInTransaction` 的 `cutoffTurn` 参数在删除死过滤后不再使用 → 从签名中删除（同时改 `executeCompression` 调用处）。`body` 参数在事务闭包内只用了 `body[0]/body[len-1]`（已在 executeCompression 算好 fromTurn/toTurn 传入）——检查闭包内是否还用 body；若不用则一并删除 `body` 参数。以实现时编译器与阅读为准，保持最小 diff。

- [x] **Step 1.5: 补端到端链路测试 `TestExecuteCompression_tailWrittenIntoSnapshot`**

```go
// --- write deps + tx mocks（追加在 compressor_test.go）---

type stubCompressWriteDeps struct {
	inserted      []biz.SessionSummary
	existsResult  bool
	snapshotJSON  string
	estTokens     int
	window        int
	listSummaries []biz.SessionSummary
}

func (s *stubCompressWriteDeps) AppendChatMessage(_ context.Context, _ string, _ biz.ChatMessage, _ bool) error {
	return nil
}
func (s *stubCompressWriteDeps) InsertSessionSummary(_ context.Context, row biz.SessionSummary) error {
	s.inserted = append(s.inserted, row)
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionListSummary(_ context.Context, _, _ string) error {
	return nil
}
func (s *stubCompressWriteDeps) SessionSummaryExists(_ context.Context, _, _ string, _, _ int) (bool, error) {
	return s.existsResult, nil
}
func (s *stubCompressWriteDeps) UpdateRunnerSnapshotJSON(_ context.Context, _, raw string) error {
	s.snapshotJSON = raw
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionContextFromLLMUsage(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionContextAfterCompression(_ context.Context, _ string, est, win int) error {
	s.estTokens, s.window = est, win
	return nil
}
func (s *stubCompressWriteDeps) IncrementInvocationCounts(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (s *stubCompressWriteDeps) ApplyMetricsDelta(_ context.Context, _ *biz.SessionMetricsDelta) error {
	return nil
}

type stubCompressTxDeps struct{ version int64 }

func (s *stubCompressTxDeps) TryIncrementCompressVersion(_ context.Context, _ string) (int64, error) {
	return s.version, nil
}
func (s *stubCompressTxDeps) CompressSessionInTx(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	return fn(ctx)
}
```

测试本体（验证 tail 进入快照 JSON）：

```go
func TestExecuteCompression_tailWrittenIntoSnapshot(t *testing.T) {
	read := &stubCompressReadDeps{}
	write := &stubCompressWriteDeps{}
	tx := &stubCompressTxDeps{version: 0}
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: tx,
		},
		lg: loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-1", CompressVersion: 0, RunnerSnapshotJSON: `{"state":{}}`}
	ag := biz.Agent{AgentKey: "agent-x"}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "old q", TurnNumber: 1, CreatedAt: "2026-07-20T09:00:00Z"},
		{Role: "assistant", ContentMarkdown: "old a", TurnNumber: 1, CreatedAt: "2026-07-20T09:00:01Z"},
	}
	tail := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "recent question 保留", TurnNumber: 2, CreatedAt: "2026-07-20T10:00:00Z"},
		{Role: "assistant", ContentMarkdown: "recent answer 保留", TurnNumber: 2, CreatedAt: "2026-07-20T10:00:01Z"},
	}
	if err := c.executeCompression(context.Background(), sess, ag, body, tail, "摘要内容", "sess-1", "user-1", 1); err != nil {
		t.Fatal(err)
	}
	if write.snapshotJSON == "" {
		t.Fatal("snapshot 未写入")
	}
	if !strings.Contains(write.snapshotJSON, "recent question 保留") || !strings.Contains(write.snapshotJSON, "recent answer 保留") {
		t.Fatalf("tail 未写入快照: %s", write.snapshotJSON)
	}
	if !strings.Contains(write.snapshotJSON, "摘要内容") {
		t.Fatalf("摘要未写入快照: %s", write.snapshotJSON)
	}
}
```

注意：mock 方法签名以实现时真实接口为准（编译器会指出差异，特别是 `SessionSummaryExists` 的参数个数与 `biz.Session` 字段名）。`executeCompression` 还会调 `syncRuntimeSnapshot`（Runtime 为 nil 时直接返回）与 `postCompressionSync`（monitorBus 为 nil 时跳过事件；`AppendChatMessage` 走 mock）。

- [x] **Step 1.6: 跑测试确认通过**

Run: `go test ./internal/session/ -run "TestLoadCompressBody|TestExecuteCompression" -count=1 -v`
Expected: PASS

- [x] **Step 1.7: 全量验证 + review**

Run: `go build ./... ; go test ./internal/session/... -count=1`
Review 检查点：tail 全链无二次过滤；`syncRuntimeSnapshot`/`postCompressionSync`/`estimateCompactedPromptTokens` 拿到的都是真实 tail；现有 `TestRewriteSnapshotWithCompression_tailEvents` 仍通过。

---

## Phase 2（P1）：递归滚动摘要（PriorSummary 吸收 + 事务内替换旧摘要）

**目标**：L3 LLM 压缩时把全部历史摘要作为 `PriorSummary` 传给 LLM 吸收合并；成功后事务内删除旧摘要行、写入单条合并行，根治摘要无限拼接增长。

**Files:**
- Modify: `internal/biz/session/usecase.go:444-448`（SummaryWriter 接口 +1 方法）
- Modify: `internal/biz/session/compression.go`（Usecase 委托 +1）
- Modify: `internal/biz/session/summary.go`（Facade 委托 +1）
- Modify: `internal/data/session_repo_summaries.go`（Ent 删除实现）
- Modify: `internal/session/compressor.go`（compressOutcome 结构、cascade/llmCompress/executeCompression/compressInTransaction）
- Test: `internal/session/compressor_test.go`（追加）
- 编译驱动的 mock 补齐：`internal/biz/session/*_test.go`、`internal/biz/spirit_team_usecase_test.go` 中实现 SummaryWriter 的 mock 各加 `DeleteSessionSummaries` no-op。

- [x] **Step 2.1: 写失败测试 `TestLLMCompress_passesPriorSummaryAndAbsorbs`**

```go
type fakeLLMCompressor struct {
	lastReq compress.Request
	result  compress.Result
	err     error
}

func (f *fakeLLMCompressor) Compress(_ context.Context, req compress.Request) (compress.Result, error) {
	f.lastReq = req
	return f.result, f.err
}

func TestLLMCompress_passesPriorSummaryAndAbsorbs(t *testing.T) {
	read := &stubCompressReadDeps{}
	read.listSummaries = []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要A"},
		{FromTurn: 4, ToTurn: 5, SummaryMarkdown: "旧摘要B"},
	}
	fake := &fakeLLMCompressor{result: compress.Result{Markdown: "合并后的新摘要", Provider: "p", Model: "m"}}
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
	}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "新问题", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "新回答", TurnNumber: 6},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, "sess-1")
	if out.level != compressLevelLLM || out.markdown == "" {
		t.Fatalf("outcome = %+v", out)
	}
	if !strings.Contains(fake.lastReq.PriorSummary, "旧摘要A") || !strings.Contains(fake.lastReq.PriorSummary, "旧摘要B") {
		t.Fatalf("PriorSummary 未包含历史摘要: %q", fake.lastReq.PriorSummary)
	}
	if !out.absorbedPriors {
		t.Fatal("absorbedPriors 应为 true")
	}
}
```

注：`stubCompressReadDeps.ListSessionSummaries` 当前返回 nil —— 改为返回 `s.listSummaries` 字段（Phase 1 的 mock 上补字段）。`biz.Agent{}` 的 TruncateStrategy 默认 "summary"（走真实 LLM 路径）。

- [x] **Step 2.2: 跑测试确认编译失败**

Run: `go test ./internal/session/ -run TestLLMCompress_passesPriorSummaryAndAbsorbs -count=1`
Expected: 编译失败 —— `llmCompress` 当前返回 `(compressLevel, string)`，无 `compressOutcome`。

- [x] **Step 2.3: 引入 `compressOutcome`，改造 cascade 与 llmCompress**

`compressor.go` 新增类型（放在 compressLevel 常量块附近）：

```go
// compressOutcome carries the cascade result: which level produced markdown,
// whether prior summaries were absorbed (LLM recursive merge), and the failure
// kind when nothing was produced (Phase 3 wires classification).
type compressOutcome struct {
	level          compressLevel
	markdown       string
	absorbedPriors bool
	fail           compressFailureKind
}
```

`compressFailureKind` 本 Phase 先定义占位（Phase 3 填充分类逻辑）：

```go
type compressFailureKind int

const (
	compressFailureNone compressFailureKind = iota
	compressFailureTransient
	compressFailureDeterministic
)
```

`compressCascade` 签名改为返回 `compressOutcome`，L1/L2 分支包装返回值；L3 直接 `return c.llmCompress(...)`。

`llmCompress` 改造（核心：加载历史摘要 → 传 PriorSummary → 标记 absorbed）：

```go
func (c *Compressor) llmCompress(ctx context.Context, sess biz.Session, ag biz.Agent, body []biz.ChatMessage, sessionID string) compressOutcome {
	strategy := truncateStrategy(ag)
	filteredBody := filterMessagesForTruncateStrategy(body, strategy)

	switch strategy {
	case "drop_oldest":
		// ... 日志不变 ...
		return compressOutcome{level: compressLevelLLM, markdown: "[Earlier turns removed per drop_oldest policy]"}
	default:
		transcript := buildCompressTranscript(filteredBody)
		cProv, cMod := compressProviderModel(sess, ag)

		// 递归滚动摘要：历史摘要交给 LLM 吸收合并，防止事后拼接无限增长。
		var priorMerged string
		if c.deps.summaryReader != nil {
			if rows, err := c.deps.summaryReader.ListSessionSummaries(ctx, sessionID); err == nil {
				priorMerged = mergeSessionSummariesMarkdown(rows)
			} else {
				c.lg.Warn("读取历史摘要失败，按无历史摘要压缩",
					loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
			}
		}

		t0 := time.Now()
		res, err := c.Compress.Compress(ctx, compress.Request{
			Transcript:   transcript,
			PriorSummary: priorMerged,
			Provider:     cProv,
			Model:        cMod,
		})
		if err != nil {
			c.lg.Warn("LLM 压缩失败", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
			return compressOutcome{level: compressLevelNone, fail: compressFailureTransient} // Phase 3 替换为真实分类
		}
		md := strings.TrimSpace(res.Markdown)
		if md == "" && strategy == "hybrid" {
			md = "[Earlier turns trimmed per hybrid policy]"
		}
		if md == "" {
			return compressOutcome{level: compressLevelNone, fail: compressFailureTransient}
		}
		// ... 日志不变 ...
		return compressOutcome{
			level:          compressLevelLLM,
			markdown:       md,
			absorbedPriors: priorMerged != "" && strategy != "hybrid",
		}
	}
}
```

注意：`hybrid` 的兜底 md 是标记文本而非 LLM 合并产物 → `absorbedPriors=false`（追加行为不变）。

`runCompress` 适配：

```go
	outcome := c.compressCascade(ctx, sess, ag, body, sessionID, cutoffTurn, usedTokens, hardTok)
	if outcome.level == compressLevelNone || outcome.markdown == "" {
		return nil
	}
	return c.executeCompression(ctx, sess, ag, body, tail, outcome, sessionID, trpcUserID, cutoffTurn)
```

- [x] **Step 2.4: 事务内吸收替换（`executeCompression` / `compressInTransaction`）**

`executeCompression` 的 `md string` 参数改为 `outcome compressOutcome`；`compressInTransaction` 同步改。事务闭包核心逻辑：

```go
	err = c.deps.compressRepo.CompressSessionInTx(ctx, sessionID, func(txCtx context.Context) error {
		priorRows, err := c.deps.summaryReader.ListSessionSummaries(txCtx, sessionID)
		if err != nil {
			return err
		}

		absorb := outcome.absorbedPriors && len(priorRows) > 0
		if absorb {
			if err := c.deps.summaryWriter.DeleteSessionSummaries(txCtx, sessionID); err != nil {
				return err
			}
			if priorRows[0].FromTurn < fromTurn {
				fromTurn = priorRows[0].FromTurn
			}
		}

		row := biz.SessionSummary{
			ID:              uuid.NewString(),
			SessionID:       sessionID,
			SummaryMarkdown: outcome.markdown,
			FromTurn:        fromTurn,
			ToTurn:          toTurn,
			TokenEstimate:   roughTokenEstimate(outcome.markdown),
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		if err := c.deps.summaryWriter.InsertSessionSummary(txCtx, row); err != nil {
			return err
		}

		if absorb {
			mergedSummary = outcome.markdown
		} else {
			allRows, err := c.deps.summaryReader.ListSessionSummaries(txCtx, sessionID)
			if err != nil {
				return err
			}
			mergedSummary = mergeSessionSummariesMarkdown(allRows)
		}

		tailMsgs = tail
		// ... RewriteSnapshotWithCompression / UpdateRunnerSnapshotJSON /
		//     UpdateSessionContextAfterCompression / UpdateSessionListSummary 不变 ...
		return nil
	})
```

- [x] **Step 2.5: `SummaryWriter` 接口 + 实现 + 全量 mock 补齐**

`internal/biz/session/usecase.go`：

```go
// Stability:stable
type SummaryWriter interface {
	InsertSessionSummary(ctx context.Context, row SessionSummary) error
	DeleteSessionSummaries(ctx context.Context, sessionID string) error // 新增：递归合并时清除被吸收的旧摘要
	UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error
	SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error)
}
```

`internal/biz/session/compression.go`（委托）：

```go
// DeleteSessionSummaries removes all rolling summary rows for the session.
func (uc *SessionCompressionUsecase) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	return uc.summaryWriter.DeleteSessionSummaries(ctx, sessionID)
}
```

`internal/biz/session/summary.go`（Facade）：同模式委托到 `uc.compressionUsecase.DeleteSessionSummaries`。

`internal/data/session_repo_summaries.go`（实现 —— 先读该文件确认 Ent client 访问模式与表字段，按既有方法风格实现）：

```go
// DeleteSessionSummaries removes all summary rows for a session (recursive merge).
func (r *sessionRepo) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	_, err := r.data.RW().Write(ctx).SessionSummary.Delete().
		Where(sessionsummary.SessionID(sessionID)).
		Exec(ctx)
	return entErrToBizErr(err, apierror.DomainSession)
}
```

注意：Ent 实体名/包名以实现时读的代码为准；删除在 `CompressSessionInTx` 事务内调用，须确认走事务感知路径（`EntClientFromCtx` 或 `RW().Write(ctx)` 的 ctx 事务注入——按该文件既有写方法的同款写法）。

mock 补齐：`go build ./...` 列出所有未实现 `DeleteSessionSummaries` 的类型，逐个加：

```go
func (m *xxxMock) DeleteSessionSummaries(_ context.Context, _ string) error { return nil }
```

`stubCompressWriteDeps`（Phase 1 新建）加：

```go
func (s *stubCompressWriteDeps) DeleteSessionSummaries(_ context.Context, _ string) error {
	s.deleted = true
	return nil
}
```

（`stubCompressWriteDeps` 新增 `deleted bool` 字段。）

- [x] **Step 2.6: 补事务吸收测试**

```go
func TestExecuteCompression_absorbDeletesPriors(t *testing.T) {
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要"},
	}}
	write := &stubCompressWriteDeps{}
	tx := &stubCompressTxDeps{version: 0}
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: tx,
		},
		lg: loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-1", CompressVersion: 0, RunnerSnapshotJSON: `{"state":{}}`}
	outcome := compressOutcome{level: compressLevelLLM, markdown: "合并摘要", absorbedPriors: true}
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q", TurnNumber: 6}}
	if err := c.executeCompression(context.Background(), sess, biz.Agent{}, body, nil, outcome, "sess-1", "u", 5); err != nil {
		t.Fatal(err)
	}
	if !write.deleted {
		t.Fatal("旧摘要未被删除")
	}
	if len(write.inserted) != 1 || write.inserted[0].FromTurn != 1 {
		t.Fatalf("合并行 FromTurn 应为 1（吸收最早历史）: %+v", write.inserted)
	}
	if !strings.Contains(write.snapshotJSON, "合并摘要") {
		t.Fatalf("快照未含合并摘要: %s", write.snapshotJSON)
	}
	if strings.Contains(write.snapshotJSON, "旧摘要---") || strings.Count(write.snapshotJSON, "旧摘要") > 1 {
		t.Fatalf("快照不应再拼接旧摘要: %s", write.snapshotJSON)
	}
}

func TestExecuteCompression_noAbsorbKeepsAppend(t *testing.T) {
	// absorbedPriors=false（如 hybrid 兜底）→ 不删除旧摘要，保持追加合并。
	// 结构同上：outcome.absorbedPriors=false，断言 write.deleted == false 且
	// snapshotJSON 同时含 "旧摘要" 与新 markdown（merge 拼接）。
}
```

注意：`read.listSummaries` 需被 `ListSessionSummaries` 返回（事务内会先 list priors）。`MaxSessionSummaryToTurn`/`loadCompressBody` 不经此路径。mock 方法签名以编译器为准修正。

- [x] **Step 2.7: 全量验证 + review**

Run: `go build ./... ; go test ./internal/session/... ./internal/biz/... ./internal/data/... -count=1`
Review 检查点：
- 只有真实调用 LLM 且传了 PriorSummary 的路径才 `absorbedPriors=true`（drop_oldest/hybrid 兜底/L1/L2 均为 false）。
- 删除与插入在同一事务；`MaxSessionSummaryToTurn` 语义不变（合并行 ToTurn=最新 toTurn）。
- 幂等与 CAS 防线不受影响（`SessionSummaryExists` 在删除前检查）。
- 信息丢失防护：absorb 只发生在 LLM 成功产出非空 md 之后（Phase 3 再加退化守卫加固）。

---

## Phase 3（P1）：摘要质量门（退化检测 + 减量守卫 + 错误分类纯函数）

**目标**：移植 Grok 的三道质量防线，全部实现为零副作用纯函数。

**Files:**
- Create: `internal/session/compress_quality.go`（纯函数 + 常量）
- Create: `internal/session/compress_quality_test.go`
- Modify: `internal/session/compressor.go`（llmCompress 重试循环 + 质量门接入）

- [x] **Step 3.1: 写失败测试**

`compress_quality_test.go`：

```go
package session

import (
	"errors"
	"fmt"
	"testing"

	"aranea-agents/pkg/apierror"
	trpcmodel "aranea-agents/pkg/trpc-agent-go/model"
)

func TestIsDegenerateSummary(t *testing.T) {
	longTranscript := 5000 // runes
	short := ""
	for i := 0; i < 100; i++ {
		short += "短"
	}
	if !isDegenerateSummary(short, longTranscript) {
		t.Fatal("100 字摘要 vs 5000 字原文应判退化")
	}
	long := short + short + short // 300 字
	if isDegenerateSummary(long, longTranscript) {
		t.Fatal("300 字摘要不应判退化")
	}
	if isDegenerateSummary(short, 500) {
		t.Fatal("原文不足 1000 字时不启用退化判定")
	}
}

func TestPassesReductionGuard(t *testing.T) {
	if !passesReductionGuard(700, 1000) {
		t.Fatal("降到 70% 应通过")
	}
	if passesReductionGuard(850, 1000) {
		t.Fatal("只降到 85% 应被守卫拦截")
	}
	if !passesReductionGuard(0, 0) {
		t.Fatal("零输入不应拦截（无意义比较放过）")
	}
}

func TestClassifyCompressError(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	cases := []struct {
		name string
		err  error
		want compressFailureKind
	}{
		{"nil", nil, compressFailureNone},
		{"context_overflow_message", apierror.Wrap(&trpcmodel.ResponseError{Message: "This model's maximum context length is 131072 tokens"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"context_code", apierror.Wrap(&trpcmodel.ResponseError{Message: "bad request", Code: strPtr("context_length_exceeded")}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"invalid_request_type", apierror.Wrap(&trpcmodel.ResponseError{Message: "bad param", Type: "invalid_request_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"auth_type", apierror.Wrap(&trpcmodel.ResponseError{Message: "invalid api key", Type: "authentication_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"rate_limit", apierror.Wrap(&trpcmodel.ResponseError{Message: "slow down", Type: "rate_limit_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureTransient},
		{"server_5xx", apierror.Wrap(&trpcmodel.ResponseError{Message: "internal error", Type: "server_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureTransient},
		{"plain_unknown", fmt.Errorf("network blip"), compressFailureTransient},
		{"wrapped_unknown", fmt.Errorf("wrap: %w", errors.New("io timeout")), compressFailureTransient},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCompressError(tt.err); got != tt.want {
				t.Fatalf("classifyCompressError = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [x] **Step 3.2: 跑测试确认编译失败**

Run: `go test ./internal/session/ -run "TestIsDegenerateSummary|TestPassesReductionGuard|TestClassifyCompressError" -count=1`
Expected: 编译失败（函数不存在）。

- [x] **Step 3.3: 实现 `compress_quality.go`**

```go
package session

import (
	"errors"
	"strings"
	"unicode/utf8"

	trpcmodel "aranea-agents/pkg/trpc-agent-go/model"
)

// 质量门常量（参考 Grok xai-grok-compaction code_compaction/config.rs）：
const (
	// minSummarySeedChars: 清洗后摘要低于此 rune 数视为退化（Grok=500；本系统
	// 摘要面向中文短会话，取 200）。
	minSummarySeedChars = 200
	// minTranscriptCharsForGuard: 原文不足此 rune 数时不启用退化判定
	// （短对话本就只能产出短摘要）。
	minTranscriptCharsForGuard = 1000
	// maxSummaryReductionRatio: 摘要 token 估算 ≥ 原文 80% 视为无效压缩，丢弃结果。
	maxSummaryReductionRatio = 0.8
	// llmCompressMaxAttempts: 空响应/退化摘要的重试上限（首次 + 1 次重试）。
	llmCompressMaxAttempts = 2
)

// isDegenerateSummary reports whether the LLM summary is too short to be a
// faithful compression of a substantial transcript (pure function).
func isDegenerateSummary(md string, transcriptRunes int) bool {
	if transcriptRunes < minTranscriptCharsForGuard {
		return false
	}
	return utf8.RuneCountInString(strings.TrimSpace(md)) < minSummarySeedChars
}

// passesReductionGuard reports whether the compression materially reduced
// tokens (pure function). Zero inputs pass (nothing meaningful to compare).
func passesReductionGuard(summaryTokens, bodyTokens int) bool {
	if summaryTokens <= 0 || bodyTokens <= 0 {
		return true
	}
	return float64(summaryTokens) < maxSummaryReductionRatio*float64(bodyTokens)
}

// classifyCompressError maps a compressor error to a failure kind.
// Pure logic only — no I/O, no logging（与 Grok classify_error 同纪律）.
func classifyCompressError(err error) compressFailureKind {
	if err == nil {
		return compressFailureNone
	}
	var respErr *trpcmodel.ResponseError
	if errors.As(err, &respErr) && respErr != nil {
		msg := strings.ToLower(respErr.Message)
		// 上下文溢出：确定性失败，重发必然再败（Grok: 永远 Fatal）。
		if strings.Contains(msg, "context length") ||
			strings.Contains(msg, "maximum context") ||
			strings.Contains(msg, "context_window") ||
			strings.Contains(msg, "too many tokens") ||
			strings.Contains(msg, "reduce the length") {
			return compressFailureDeterministic
		}
		if respErr.Code != nil {
			switch strings.ToLower(*respErr.Code) {
			case "context_length_exceeded", "invalid_api_key", "model_not_found":
				return compressFailureDeterministic
			}
		}
		switch strings.ToLower(respErr.Type) {
		case "invalid_request_error", "authentication_error", "permission_error":
			return compressFailureDeterministic
		case "rate_limit_error", "server_error", "timeout", "overloaded_error":
			return compressFailureTransient
		}
	}
	// 未知错误按瞬态处理（允许重试，由失败抑制控制频率）。
	return compressFailureTransient
}
```

- [x] **Step 3.4: llmCompress 接入重试循环 + 质量门**

将 Phase 2 的 `llmCompress` default 分支替换为：

```go
	default:
		transcript := buildCompressTranscript(filteredBody)
		transcriptRunes := utf8.RuneCountInString(transcript)
		cProv, cMod := compressProviderModel(sess, ag)

		var priorMerged string
		// ...（同 Phase 2，不变）...

		t0 := time.Now()
		var md string
		var res compress.Result
		for attempt := 1; attempt <= llmCompressMaxAttempts; attempt++ {
			var err error
			res, err = c.Compress.Compress(ctx, compress.Request{
				Transcript:   transcript,
				PriorSummary: priorMerged,
				Provider:     cProv,
				Model:        cMod,
			})
			if err != nil {
				kind := classifyCompressError(err)
				c.lg.Warn("LLM 压缩调用失败",
					loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
					loggateway.Int("attempt", attempt), loggateway.Err(err))
				if kind == compressFailureDeterministic {
					return compressOutcome{level: compressLevelNone, fail: compressFailureDeterministic}
				}
				continue
			}
			md = strings.TrimSpace(res.Markdown)
			if md == "" {
				c.lg.Warn("LLM 压缩返回空摘要，重试",
					loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
					loggateway.Int("attempt", attempt))
				continue
			}
			if isDegenerateSummary(md, transcriptRunes) {
				c.lg.Warn("LLM 压缩摘要退化，重试",
					loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
					loggateway.Int("attempt", attempt),
					loggateway.Int("summary_runes", utf8.RuneCountInString(md)),
					loggateway.Int("transcript_runes", transcriptRunes))
				md = ""
				continue
			}
			break
		}
		if md == "" && strategy == "hybrid" {
			md = "[Earlier turns trimmed per hybrid policy]"
		}
		if md == "" {
			return compressOutcome{level: compressLevelNone, fail: compressFailureTransient}
		}
		// 减量守卫：压缩无实质收益则丢弃（hybrid 兜底标记除外）。
		if strategy != "hybrid" {
			bodyTokens := llmcontext.EstimateTokensFromChars(transcriptRunes)
			mdTokens := llmcontext.EstimateTokensFromChars(utf8.RuneCountInString(md))
			if !passesReductionGuard(mdTokens, bodyTokens) {
				c.lg.Warn("压缩减量不足，丢弃结果",
					loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
					loggateway.Int("summary_tokens", mdTokens), loggateway.Int("body_tokens", bodyTokens))
				return compressOutcome{level: compressLevelNone, fail: compressFailureTransient}
			}
		}
		// ... 成功日志（原有字段） ...
		return compressOutcome{
			level:          compressLevelLLM,
			markdown:       md,
			absorbedPriors: priorMerged != "" && strategy != "hybrid",
		}
	}
```

注意：`internal/session/compressor.go` 需新增 import `"unicode/utf8"`；`llmcontext` 已在 import 中。实现时先确认 `llmcontext.EstimateTokensFromChars` 的入参语义（rune 数）与既有调用点一致（`prompt_snapshot.go:131`、`token_estimate.go:15`），不一致则以既有调用点的单位为准并同步修正本处。

- [x] **Step 3.5: 补 llmCompress 质量门行为测试**

追加到 `compressor_test.go`：

```go
// fakeLLMCompressor 扩展为脚本化：按调用序返回结果。
type scriptedLLMCompressor struct {
	calls   int
	results []compress.Result
	errs    []error
}

func (s *scriptedLLMCompressor) Compress(_ context.Context, _ compress.Request) (compress.Result, error) {
	i := s.calls
	s.calls++
	if i >= len(s.results) && i >= len(s.errs) {
		return compress.Result{}, nil
	}
	if i < len(s.errs) && s.errs[i] != nil {
		return compress.Result{}, s.errs[i]
	}
	if i < len(s.results) {
		return s.results[i], nil
	}
	return compress.Result{}, nil
}
```

用例：
1. **瞬态错误后成功**：errs=[fmt.Errorf("boom")] + results=[{Markdown: 长摘要}] → level=LLM，calls=2。
2. **确定性错误不重试**：errs=[apierror.Wrap(&trpcmodel.ResponseError{Type: "authentication_error"}, ...)] → level=none，fail=deterministic，calls=1。
3. **退化摘要重试**：第一次返回短 md、第二次返回长 md（body 构造 ≥1000 runes transcript）→ 成功，calls=2。
4. **减量守卫拦截**：md 长度接近 transcript（均中文长文）→ level=none，fail=transient。

长摘要/长 transcript 用 `strings.Repeat("压缩内容", N)` 生成（每 4 rune ≈ 4 字，估 1.6 token，比例可控）。阈值设计：transcript 2000 runes → estTokens=800；守卫要求 md est < 640 → md < 1600 runes。「守卫拦截」用例：md 1800 runes → est 720 ≥ 640 → 拦截。算清楚再写断言。

- [x] **Step 3.6: 全量验证 + review**

Run: `go build ./... ; go test ./internal/session/... ./internal/compress/... -count=1`
Review 检查点：纯函数零副作用（无日志/IO）；重试不打断 ctx 取消（`ctx.Err()` 时 Compress 返回 err → 分类为 transient 继续重试一次也无害，但可在循环开头 `if ctx.Err() != nil { return none+transient }` 提前退出 —— 实现时加上）；deterministic 路径不产生任何写库。

---

## Phase 4（P1）：压缩失败抑制（deterministic sticky + transient 退避）

**目标**：移植 Grok `auto_compact_suppressed` 状态机：确定性失败按「压缩模型」sticky 抑制（模型切换自动解除）；瞬态失败按 minGap 退避；手动/durable 触发绕过抑制。

**Files:**
- Create: `internal/session/compress_suppress.go`
- Create: `internal/session/compress_suppress_test.go`
- Modify: `internal/session/compressor.go`（Compressor 字段、runCompress 接入、RemoveSessionState）

- [x] **Step 4.1: 写失败测试 `compress_suppress_test.go`**

```go
package session

import (
	"testing"
	"time"
)

func TestCompressSuppressManager(t *testing.T) {
	now := time.Now()
	m := newCompressSuppressManager()

	// 无记录 → 不抑制
	if suppressed, _ := m.check("s1", "openai/gpt-4o", 10*time.Minute, now) {
		t.Fatal("无记录不应抑制")
	}

	// 确定性失败 → 同模型抑制，不同模型放行
	m.record("s1", compressFailureDeterministic, "openai/gpt-4o", now)
	if suppressed, _ := m.check("s1", "openai/gpt-4o", 10*time.Minute, now.Add(time.Hour)) {
	} else {
		t.Fatal("确定性失败应 sticky 抑制（不受 minGap 影响）")
	}
	if suppressed, _ := m.check("s1", "anthropic/claude", 10*time.Minute, now) {
		t.Fatal("模型切换应解除抑制")
	}

	// 瞬态失败 → minGap 内抑制，过后放行
	m.record("s2", compressFailureTransient, "openai/gpt-4o", now)
	if suppressed, _ := m.check("s2", "openai/gpt-4o", 10*time.Minute, now.Add(5*time.Minute)) {
	} else {
		t.Fatal("瞬态失败 minGap 内应抑制")
	}
	if suppressed, _ := m.check("s2", "openai/gpt-4o", 10*time.Minute, now.Add(11*time.Minute)) {
		t.Fatal("瞬态失败超过 minGap 应放行")
	}

	// clear 解除
	m.record("s3", compressFailureDeterministic, "openai/gpt-4o", now)
	m.clear("s3")
	if suppressed, _ := m.check("s3", "openai/gpt-4o", 10*time.Minute, now) {
		t.Fatal("clear 后不应抑制")
	}
}
```

- [x] **Step 4.2: 跑测试确认编译失败**

Run: `go test ./internal/session/ -run TestCompressSuppressManager -count=1`
Expected: 编译失败。

- [x] **Step 4.3: 实现 `compress_suppress.go`**

```go
package session

import (
	"sync"
	"time"
)

// compressSuppression records the latest compression failure for a session.
// 移植自 Grok auto_compact_suppressed：确定性失败 sticky 到模型切换，
// 瞬态失败按 minGap 退避（避免注定/无意义的重试每 turn 重复打 LLM）。
type compressSuppression struct {
	kind          compressFailureKind
	providerModel string // 压缩模型 "provider/model"
	at            time.Time
}

// compressSuppressManager tracks per-session compression failure suppression.
// 进程内内存态：重启后重新尝试一次再抑制，代价可接受（与 Grok shell 同语义）。
type compressSuppressManager struct {
	m sync.Map // map[sessionID]compressSuppression
}

func newCompressSuppressManager() *compressSuppressManager {
	return &compressSuppressManager{}
}

// check reports whether compression should be suppressed for the session.
// deterministic: suppressed while the compress model is unchanged.
// transient: suppressed within minGap of the last failure.
func (s *compressSuppressManager) check(sessionID, providerModel string, minGap time.Duration, now time.Time) (bool, string) {
	v, ok := s.m.Load(sessionID)
	if !ok {
		return false, ""
	}
	sup := v.(compressSuppression)
	switch sup.kind {
	case compressFailureDeterministic:
		if sup.providerModel == providerModel {
			return true, "deterministic_failure"
		}
		return false, "" // 模型已切换，抑制解除
	case compressFailureTransient:
		if minGap > 0 && now.Sub(sup.at) < minGap {
			return true, "transient_backoff"
		}
		return false, ""
	default:
		return false, ""
	}
}

func (s *compressSuppressManager) record(sessionID string, kind compressFailureKind, providerModel string, now time.Time) {
	if kind == compressFailureNone || sessionID == "" {
		return
	}
	s.m.Store(sessionID, compressSuppression{kind: kind, providerModel: providerModel, at: now})
}

func (s *compressSuppressManager) clear(sessionID string) {
	s.m.Delete(sessionID)
}
```

- [x] **Step 4.4: Compressor 接入**

`compressor.go`：

1. Compressor struct 加字段 `suppress *compressSuppressManager`（第 12 个字段，AS-COG-01 ≤15 内）；`NewCompressor` 初始化 `suppress: newCompressSuppressManager()`。
2. `runCompress` 签名 `skipMinGap bool` → `forced bool`（语义：手动/durable 触发绕过防抖与抑制；三个调用点 `AfterNativeTurn(false)` / `BeforeDurableTurn(true)` / `CompactSession(true)` 不动）。
3. 在 soft/hard 判定之后、`tryStartCompress` 之前插入抑制检查：

```go
	if !forced {
		provMod := compressProviderModelKey(sess, ag)
		if suppressed, reason := c.suppress.check(sessionID, provMod, compressMinGapFromAgent(ag), time.Now()); suppressed {
			c.lg.Info("压缩被失败抑制跳过",
				loggateway.StepID("session.compress"), loggateway.SessionID(sessionID),
				loggateway.Str("suppress_reason", reason))
			return nil
		}
	}
```

`compress_policy.go` 加：

```go
// compressProviderModelKey returns the "provider/model" identity of the
// compression model, used as the suppression key (model switch clears suppression).
func compressProviderModelKey(sess biz.Session, ag biz.Agent) string {
	p, m := compressProviderModel(sess, ag)
	return p + "/" + m
}
```

4. 失败记录与成功清除：

```go
	outcome := c.compressCascade(...)
	if outcome.level == compressLevelNone || outcome.markdown == "" {
		if outcome.fail != compressFailureNone {
			c.suppress.record(sessionID, outcome.fail, compressProviderModelKey(sess, ag), time.Now())
		}
		return nil
	}
	err = c.executeCompression(...)
	if err != nil {
		c.suppress.record(sessionID, compressFailureTransient, compressProviderModelKey(sess, ag), time.Now())
		return err
	}
	c.suppress.clear(sessionID)
	return nil
```

5. `RemoveSessionState` 加 `c.suppress.clear(sessionID)`。

- [x] **Step 4.5: 补 runCompress 抑制行为测试**

```go
func TestRunCompress_suppressedAfterDeterministicFailure(t *testing.T) {
	read := &stubCompressReadDeps{maxSummarized: 0, msgs: makeTimeline(6)}
	read.sess = biz.Session{ID: "sess-1", ContextUsedTokens: 100000, LastContextWindowTokens: 1000} // 必触发
	// fake compressor 返回确定性错误
	fakeErr := apierror.Wrap(&trpcmodel.ResponseError{Type: "authentication_error", Message: "invalid key"}, apierror.CodeInternal, apierror.DomainProvider)
	fake := &scriptedLLMCompressor{errs: []error{fakeErr, fakeErr, fakeErr}}
	c := &Compressor{
		deps:     compressDeps{sessionReader: read, messageReader: read, summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
		suppress: newCompressSuppressManager(),
		flight:   newCompressFlightManager(),
		buf:      newCompressBufferManager(),
	}
	ag := biz.Agent{}
	// 第一次：调用 LLM 并失败
	_ = c.runCompress(context.Background(), "sess-1", "u", ag, false)
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	// 第二次：同模型被抑制，不再调用
	_ = c.runCompress(context.Background(), "sess-1", "u", ag, false)
	if fake.calls != 1 {
		t.Fatalf("抑制后仍调用 LLM: calls = %d", fake.calls)
	}
}
```

注意：`stubCompressReadDeps` 需加 `sess biz.Session` 字段让 `GetSessionByID` 返回可控 session（ContextUsedTokens 超 hardTok 以绕过 minGap；window 取小值确保触发）。`runCompress` 里 `flight`/`buf`/`suppress` 必须初始化。若 `runCompress` 还触达其他 nil 依赖（monitorBus 检查为 nil-safe），以测试运行暴露为准逐个补。

- [x] **Step 4.6: 全量验证 + review**

Run: `go build ./... ; go test ./internal/session/... -count=1`
Review 检查点：抑制不持久化（重启恢复尝试一次，符合预期并写入注释）；forced=true（手动 `/compact` 与 durable turn）绕过抑制；成功压缩后抑制清除（下次失败重新计时）；抑制 key 用压缩模型而非聊天模型。

---

## Phase 5（P2）：双锚点 token 估算校准接线

**目标**：在压缩 LLM 成功路径回填权威 usage，让共享估算器从 2.5 chars/token 默认值校准到真实比率。

**Files:**
- Modify: `internal/compress/service.go:103-111`
- Modify: `internal/llmcontext/token_estimator.go`（+test-only reset）
- Test: `internal/llmcontext/token_estimator_test.go`（若无则创建）、`internal/compress/service_test.go`（追加接线测试，若其 HTTP fake 基建可用）

- [x] **Step 5.1: 确认单位一致性**

读 `internal/agent/prompt_snapshot.go:125-140` 与 `internal/session/token_estimate.go`，确认 `EstimateTokensFromChars` 调用点传入的是 rune 数还是 byte 数。**接线单位必须与调用点一致**（校准比率 = chars/tokens，混用会让估算偏 3 倍）。若既有调用点传 byte 数，则校准也用 byte 数。

- [x] **Step 5.2: 写失败测试（estimator 行为）**

`internal/llmcontext/token_estimator_test.go`（若已存在则追加）：

```go
func TestSharedEstimatorCalibratedByAuthoritativeUsage(t *testing.T) {
	resetSharedEstimatorForTest()
	defer resetSharedEstimatorForTest()
	// 默认 2.5 chars/token
	if got := EstimateTokensFromChars(1000); got != 400 {
		t.Fatalf("default estimate = %d, want 400", got)
	}
	// 权威锚点：2000 chars 实测 1000 tokens → 2.0 chars/token
	RecordAuthoritativeUsage(1000, 2000)
	if got := EstimateTokensFromChars(1000); got != 500 {
		t.Fatalf("calibrated estimate = %d, want 500", got)
	}
}
```

- [x] **Step 5.3: 实现 reset + 接线**

`token_estimator.go` 追加：

```go
// resetSharedEstimatorForTest restores the shared estimator to its default
// state. For tests only.
func resetSharedEstimatorForTest() {
	shared = NewTokenEstimator()
}
```

`compress/service.go` 成功路径（`CallOpenAICompatChat` 返回后）：

```go
	out.Markdown = strings.TrimSpace(text)
	out.PromptTokens = ptok
	out.CompletionTokens = ctok
	if ptok > 0 {
		chars := inputChars(sys, req.PriorSummary, transcript)
		llmcontext.RecordAuthoritativeUsage(ptok, chars)
	}
	return out, nil
```

`inputChars` 小函数（与 Step 5.1 确定的单位一致）：

```go
func inputChars(parts ...string) int {
	n := 0
	for _, p := range parts {
		n += len([]rune(p)) // 或 len(p)，以调用点单位为准
	}
	return n
}
```

确认 `internal/compress` import `internal/llmcontext` 无循环（llmcontext 当前仅 import `sync`）。

- [x] **Step 5.4: 全量验证 + review**

Run: `go build ./... ; go test ./internal/llmcontext/... ./internal/compress/... -count=1`
Review 检查点：校准只在 `ptok > 0` 时发生（避免 0 除）；`RecordAuthoritativeUsage` 是全局共享态——多模型混用时比率会漂移（接受：Grok 同样单一比率；注释说明）；无 import cycle。

---

## Phase 6：全量验证 + 文档同步

- [x] **Step 6.1: 全量验证**

Run（提交前纪律）：`make build && make test && make lint`
若 make 不可用则：`go build ./... && go test ./... -count=1 && golangci-lint run`

- [x] **Step 6.2: 文档同步（DOC-SYNC 红线）**

- 读 `docs/development/10-session.development.md` 与 `docs/development/memory/L0-development.md`，找到压缩相关章节，更新：
  - tail 保留修复（行为契约变化：压缩后近期轮次保留在快照中）
  - 递归滚动摘要（摘要不再无限拼接；LLM 吸收合并）
  - 质量门与失败抑制（新行为：退化重试/减量丢弃/确定性抑制）
- 按 `aranea-docs-guide` 规范更新状态标记与代码锚点。

- [x] **Step 6.3: 终态 review**

对照本计划逐项核对 checkbox；对照 `aranea-review` 维度 1/2/3/8/11/14 自审；输出 review 结论。

**终态 review 结果（2026-07-20）**：

| 轮次 | 发现 | 级别 | 处置 |
|------|------|------|------|
| R1 | 设计文档 6.6 未同步质量门/抑制/校准三小节 | P1 | ✅ 已补 6.6.1~6.6.3 |
| R1 | 4 处错误日志被 `monitorBus != nil` 门控吞没 | P1 | ✅ 已解耦（日志始终输出） |
| R1 | `DeleteSessionSummaries` 未经 `entErrToBizErr` 翻译 | P2 | ✅ 已修复（DB-R5） |
| R1 | `compressor.go` 980+ 行超 AS-COG-01 上限 | P2 | ✅ 已加 `TECH-DEBT(COG)` 标记 |
| R1 | `CompactSession` 压缩失败误报 `Compacted:true` | P2 | ✅ runCompress 传播 error |
| R1 | ctx 取消被记入瞬态抑制 | P2 | ✅ 分类为 `compressFailureNone` |
| R2（终态） | hybrid 策略 LLM 成功时 `absorbedPriors` 被 `strategy != "hybrid"` 强制 false → 已吸收的历史摘要不删旧行，重复拼接无限增长 | P2 | ✅ 改为按 `llmSucceeded` 判定（兜底标记不吸收），补 2 个 TDD 测试 |

验证证据：`go build ./...` ✅；`go test ./internal/session/... ./internal/compress/... ./internal/llmcontext/... ./internal/data/... -count=1` ✅；`go test -race ./internal/session/` ✅；`go vet` ✅。

---

## 后续建议（不在本计划范围）

| # | 事项 | 说明 |
|---|------|------|
| 1 | L1 MicroCompact 永不触发 | `loadCompressBody` 只取 user/assistant，`tryMicroCompact` 找 `role=="tool"` 恒为 0。需单独决策：让 body 携带 tool 消息 or 移除 L1。 |
| 2 | 双压缩系统统一 | 系统 A（BeforeModel hook，内存态）与系统 B（Session Compressor）阈值/存储不通，建议架构决策后统一。 |
| 3 | 死配置清理 | `compress_llm_cache_*`、`ShouldCompress`、`sessionCompressThreshold`、`shouldUseStructuredCompact` 门控未接线。 |
| 4 | L1/L2 摘要累积上限 | L3 吸收替换已解决 LLM 路径增长；L1/L2 标记行在长会话仍会累积（每行 ≤2KB，风险低），需要时可加行数上限触发强制 L3。 |
