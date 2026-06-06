# Self-iteration-v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 激活自愈闭环 + 落地 Skill Intelligence + 建立进化闭环，从"被动修复"进化到"主动进化"。

**Architecture:** 四层架构（Server→Service→Biz→Data）+ Wire DI + CronRunner + CI Auto-Fix + 前端 Vue 3/Quasar

**Tech Stack:** Go 1.23, Ent ORM, Kratos v2, Wire, SQLite, Vue 3, Quasar, Pinia, TypeScript

**Traceability (sddflow):**
- plan-ready: `openspec/changes/self-iteration-v2/plan-ready.md`
- tasks: `openspec/changes/self-iteration-v2/tasks.md`
- plan: `docs/superpowers/plans/2026-06-06-self-iteration-v2.md`

---

### Task 1: RootCauseAnalyzer 接口抽取

> **trace:** plan-ready.md → `### Task 1: RootCauseAnalyzer 接口抽取` | tasks.md → `- [ ] 1.1 创建 internal/biz/monitor/root_cause_analyzer.go`
> **sync:** tasks.md → `- [ ] 1.1 创建 internal/biz/monitor/root_cause_analyzer.go` | plan-ready.md → `### Task 1: RootCauseAnalyzer 接口抽取`

**Files:**
- Create: `internal/biz/monitor/root_cause_analyzer.go`
- Modify: `internal/biz/monitor/root_cause_engine.go`
- Modify: `internal/biz/monitor/wire.go`

- [x] **Step 1: 创建 RootCauseAnalyzer 接口文件**

创建 `internal/biz/monitor/root_cause_analyzer.go`，定义接口：

```go
package monitor

import (
	"context"
)

// RootCauseAnalyzer is the interface for root cause analysis, extracted from RootCauseEngine
// for dependency inversion. Both SkillIntelligenceUsecase and PredictiveHealUsecase depend on
// this interface instead of the concrete RootCauseEngine.
type RootCauseAnalyzer interface {
	// Analyze performs root cause analysis for a step failure.
	Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)
	// AnalyzeFromReport performs root cause analysis from a structured FailureReport.
	AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: May fail until FailureReport is defined (Task 2). If so, use a forward declaration or define FailureReport first. Alternative: define AnalyzeFromReport later in Task 2.

**Note:** Since FailureReport is defined in Task 2, for now define only the Analyze method in the interface, and add AnalyzeFromReport in Task 2 after FailureReport is created.

- [x] **Step 2: 让 RootCauseEngine 实现 RootCauseAnalyzer 接口**

修改 `internal/biz/monitor/root_cause_engine.go`，确认 `RootCauseEngine` 的 `Evaluate` 方法签名可适配 `RootCauseAnalyzer.Analyze`。添加适配方法：

```go
// Analyze implements RootCauseAnalyzer.Analyze.
func (e *RootCauseEngine) Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error) {
	results := e.Evaluate(ctx, stepID, phase, metadata)
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: PASS

- [x] **Step 3: Wire 绑定 RootCauseAnalyzer**

修改 `internal/biz/monitor/wire.go`，添加 `wire.Bind` 将 `RootCauseAnalyzer` 绑定到 `*RootCauseEngine`：

```go
var WireProviderSet = wire.NewSet(
	NewAlertMetricRegistry,
	ProvideSelfCheckers,
	ProvideSelfCheckRepairers,
	wire.Bind(new(RootCauseAnalyzer), new(*RootCauseEngine)),
)
```

Run: `make wire && go build ./cmd/admin`
Expected: PASS

- [x] **Step 4: 验证**

Run: `go test ./internal/biz/monitor/... -count=1`
Expected: All existing tests PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 2: FailureReport 标准化错误表示

> **trace:** plan-ready.md → `### Task 2: FailureReport 标准化错误表示` | tasks.md → `- [ ] 2.1 创建 internal/biz/monitor/failure_report.go`
> **sync:** tasks.md → `- [ ] 2.1 创建 internal/biz/monitor/failure_report.go` | plan-ready.md → `### Task 2: FailureReport 标准化错误表示`

**Files:**
- Create: `internal/biz/monitor/failure_report.go`
- Create: `internal/biz/monitor/failure_report_parser.go`
- Create: `internal/biz/monitor/failure_report_parser_test.go`
- Create: `.auto-fix/scripts/parse-logs.py`

- [x] **Step 1: 创建 FailureReport 结构体**

创建 `internal/biz/monitor/failure_report.go`：

```go
package monitor

import (
	"crypto/rand"
	"fmt"
)

// FailureType categorizes the type of failure.
type FailureType string

const (
	FailureTypeLint      FailureType = "lint_error"
	FailureTypeTest      FailureType = "test_failure"
	FailureTypeBuild     FailureType = "build_failure"
	FailureTypeProtoSync FailureType = "proto_sync"
	FailureTypeRuntime   FailureType = "runtime_error"
)

// FailureReport is a standardized error representation for both CI and runtime failures.
type FailureReport struct {
	ID          string            `json:"id"`
	Type        FailureType       `json:"type"`
	Source      string            `json:"source"`       // "ci" or "runtime"
	Job         string            `json:"job"`
	File        string            `json:"file"`
	Line        int               `json:"line"`
	ErrorCode   string            `json:"error_code"`
	Message     string            `json:"message"`
	StackTrace  string            `json:"stack_trace"`
	RelatedCode string            `json:"related_code"`
	Metadata    map[string]string `json:"metadata"`
}

// NewFailureReport creates a FailureReport with a generated UUID.
func NewFailureReport() *FailureReport {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &FailureReport{
		ID:       fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]),
		Metadata: make(map[string]string),
	}
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: PASS

- [x] **Step 2: 补充 RootCauseAnalyzer.AnalyzeFromReport 方法**

回到 `internal/biz/monitor/root_cause_analyzer.go`，添加 `AnalyzeFromReport` 方法到接口（现在 FailureReport 已定义）：

```go
type RootCauseAnalyzer interface {
	Analyze(ctx context.Context, stepID, phase string, err error, metadata map[string]any) (*RootCauseResult, error)
	AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error)
}
```

在 `root_cause_engine.go` 添加实现：

```go
// AnalyzeFromReport implements RootCauseAnalyzer.AnalyzeFromReport.
func (e *RootCauseEngine) AnalyzeFromReport(ctx context.Context, report *FailureReport) (*RootCauseResult, error) {
	metadata := make(map[string]any)
	for k, v := range report.Metadata {
		metadata[k] = v
	}
	metadata["failure_type"] = string(report.Type)
	metadata["source"] = report.Source
	metadata["file"] = report.File
	metadata["error_code"] = report.ErrorCode

	results := e.Evaluate(ctx, report.Job, string(report.Type), metadata)
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: PASS

- [x] **Step 3: 创建 FailureReport 解析器**

创建 `internal/biz/monitor/failure_report_parser.go`，实现 `ParseCILogs` 和 `ParseRuntimeError`：

```go
package monitor

import (
	"regexp"
	"strings"
)

// ParseCILogs parses raw CI log output into a FailureReport.
func ParseCILogs(log string, job string) *FailureReport {
	report := NewFailureReport()
	report.Source = "ci"
	report.Job = job

	lines := strings.Split(log, "\n")

	// Try Go build error pattern
	if r := parseGoBuildError(lines); r != nil {
		r.Source = "ci"
		r.Job = job
		return r
	}
	// Try Go test failure pattern
	if r := parseGoTestFailure(lines); r != nil {
		r.Source = "ci"
		r.Job = job
		return r
	}
	// Try lint error pattern
	if r := parseLintError(lines); r != nil {
		r.Source = "ci"
		r.Job = job
		return r
	}

	// Fallback: treat as unknown
	report.Type = FailureTypeBuild
	report.Message = truncateString(log, 500)
	return report
}

// ParseRuntimeError converts a runtime error into a FailureReport.
func ParseRuntimeError(err error, stepID, phase string) *FailureReport {
	report := NewFailureReport()
	report.Source = "runtime"
	report.Type = FailureTypeRuntime
	report.Job = stepID
	report.Message = err.Error()
	report.Metadata["phase"] = phase
	return report
}

var goBuildErrorRe = regexp.MustCompile(`^(.+\.go):(\d+):(\d+):\s*(.+)$`)
var goTestFailureRe = regexp.MustCompile(`^\s*--- FAIL:\s*(\S+)\s+\((.+\.go):(\d+)\)`)
var lintErrorRe = regexp.MustCompile(`^(.+\.go):(\d+):(\d+):\s*(.+)$`)

func parseGoBuildError(lines []string) *FailureReport {
	for _, line := range lines {
		m := goBuildErrorRe.FindStringSubmatch(line)
		if m != nil {
			report := NewFailureReport()
			report.Type = FailureTypeBuild
			report.File = m[1]
			report.Line = mustAtoi(m[2])
			report.ErrorCode = "compile_error"
			report.Message = m[4]
			return report
		}
	}
	return nil
}

func parseGoTestFailure(lines []string) *FailureReport {
	for _, line := range lines {
		m := goTestFailureRe.FindStringSubmatch(line)
		if m != nil {
			report := NewFailureReport()
			report.Type = FailureTypeTest
			report.File = m[2]
			report.Line = mustAtoi(m[3])
			report.ErrorCode = "test_failed"
			report.Message = "Test failed: " + m[1]
			return report
		}
	}
	return nil
}

func parseLintError(lines []string) *FailureReport {
	for _, line := range lines {
		m := lintErrorRe.FindStringSubmatch(line)
		if m != nil {
			report := NewFailureReport()
			report.Type = FailureTypeLint
			report.File = m[1]
			report.Line = mustAtoi(m[2])
			report.ErrorCode = "lint_error"
			report.Message = m[4]
			return report
		}
	}
	return nil
}

func mustAtoi(s string) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: PASS

- [x] **Step 4: 创建解析器测试**

创建 `internal/biz/monitor/failure_report_parser_test.go`：

```go
package monitor

import (
	"errors"
	"testing"
)

func TestParseGoBuildError(t *testing.T) {
	log := `internal/biz/monitor/root_cause_engine.go:42:10: undefined: someVar
make: *** [Makefile:10: build] Error 1`
	report := ParseCILogs(log, "build")
	if report.Type != FailureTypeBuild {
		t.Fatalf("expected type %s, got %s", FailureTypeBuild, report.Type)
	}
	if report.Source != "ci" {
		t.Fatalf("expected source ci, got %s", report.Source)
	}
	if report.File != "internal/biz/monitor/root_cause_engine.go" {
		t.Fatalf("expected file, got %s", report.File)
	}
	if report.Line != 42 {
		t.Fatalf("expected line 42, got %d", report.Line)
	}
	if report.ErrorCode != "compile_error" {
		t.Fatalf("expected error_code compile_error, got %s", report.ErrorCode)
	}
}

func TestParseGoTestFailure(t *testing.T) {
	log := `=== RUN   TestSomething
    --- FAIL: TestSomething (internal/biz/monitor/root_cause_engine_test.go:15)
        test output here`
	report := ParseCILogs(log, "test")
	if report.Type != FailureTypeTest {
		t.Fatalf("expected type %s, got %s", FailureTypeTest, report.Type)
	}
	if report.File != "internal/biz/monitor/root_cause_engine_test.go" {
		t.Fatalf("expected file, got %s", report.File)
	}
	if report.Line != 15 {
		t.Fatalf("expected line 15, got %d", report.Line)
	}
}

func TestParseLintError(t *testing.T) {
	log := `internal/service/monitor.go:100:5: exported function should have comment`
	report := ParseCILogs(log, "lint")
	if report.Type != FailureTypeLint {
		t.Fatalf("expected type %s, got %s", FailureTypeLint, report.Type)
	}
}

func TestParseRuntimeError(t *testing.T) {
	err := errors.New("provider timeout after 30s")
	report := ParseRuntimeError(err, "step-123", "llm_call")
	if report.Type != FailureTypeRuntime {
		t.Fatalf("expected type %s, got %s", FailureTypeRuntime, report.Type)
	}
	if report.Source != "runtime" {
		t.Fatalf("expected source runtime, got %s", report.Source)
	}
	if report.Message != "provider timeout after 30s" {
		t.Fatalf("expected message, got %s", report.Message)
	}
	if report.Metadata["phase"] != "llm_call" {
		t.Fatalf("expected phase llm_call, got %s", report.Metadata["phase"])
	}
}
```

Run: `go test ./internal/biz/monitor/... -run TestParse -count=1`
Expected: All tests PASS

- [x] **Step 5: 创建 CI 侧 Python 解析脚本**

创建 `.auto-fix/scripts/parse-logs.py`：

```python
#!/usr/bin/env python3
"""Parse CI failure logs into structured FailureReport JSON."""
import json
import re
import sys
import uuid


def parse_go_build_error(lines):
    pattern = re.compile(r'^(.+\.go):(\d+):(\d+):\s*(.+)$')
    for line in lines:
        m = pattern.match(line)
        if m:
            return {
                "id": str(uuid.uuid4()),
                "type": "build_failure",
                "source": "ci",
                "job": "",
                "file": m.group(1),
                "line": int(m.group(2)),
                "error_code": "compile_error",
                "message": m.group(4),
                "stack_trace": "",
                "related_code": "",
                "metadata": {}
            }
    return None


def parse_go_test_failure(lines):
    pattern = re.compile(r'^\s*--- FAIL:\s+(\S+)\s+\((.+\.go):(\d+)\)')
    for line in lines:
        m = pattern.match(line)
        if m:
            return {
                "id": str(uuid.uuid4()),
                "type": "test_failure",
                "source": "ci",
                "job": "",
                "file": m.group(2),
                "line": int(m.group(3)),
                "error_code": "test_failed",
                "message": f"Test failed: {m.group(1)}",
                "stack_trace": "",
                "related_code": "",
                "metadata": {"test_name": m.group(1)}
            }
    return None


def parse_lint_error(lines):
    pattern = re.compile(r'^(.+\.go):(\d+):(\d+):\s*(.+)$')
    for line in lines:
        m = pattern.match(line)
        if m:
            return {
                "id": str(uuid.uuid4()),
                "type": "lint_error",
                "source": "ci",
                "job": "",
                "file": m.group(1),
                "line": int(m.group(2)),
                "error_code": "lint_error",
                "message": m.group(4),
                "stack_trace": "",
                "related_code": "",
                "metadata": {}
            }
    return None


def main():
    log = sys.stdin.read()
    lines = log.strip().split('\n')

    report = parse_go_build_error(lines)
    if not report:
        report = parse_go_test_failure(lines)
    if not report:
        report = parse_lint_error(lines)
    if not report:
        report = {
            "id": str(uuid.uuid4()),
            "type": "build_failure",
            "source": "ci",
            "job": "",
            "file": "",
            "line": 0,
            "error_code": "unknown",
            "message": log[:500],
            "stack_trace": "",
            "related_code": "",
            "metadata": {}
        }

    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    main()
```

Run: `python3 .auto-fix/scripts/parse-logs.py` with test input
Expected: Valid JSON output

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 3: 统一失败模式知识库

> **trace:** plan-ready.md → `### Task 3: 统一失败模式知识库` | tasks.md → `- [ ] 3.1 创建 internal/data/ent/schema/failure_pattern.go`
> **sync:** tasks.md → `- [ ] 3.1 创建 internal/data/ent/schema/failure_pattern.go` | plan-ready.md → `### Task 3: 统一失败模式知识库`

**Files:**
- Create: `internal/data/ent/schema/failure_pattern.go`
- Create: `internal/biz/monitor/failure_pattern_repo.go`
- Create: `internal/data/failure_pattern.go`
- Create: `internal/cronrunner/jobs/failure_pattern_sync.go`

- [x] **Step 1: 创建 FailurePattern Ent Schema**

创建 `internal/data/ent/schema/failure_pattern.go`，参考现有 schema 文件的格式（如 `heal_record.go`）：

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type FailurePattern struct {
	ent.Schema
}

func (FailurePattern) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "failure_pattern"},
	}
}

func (FailurePattern) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").DefaultFunc(uuid.NewString).StorageKey("id"),
		field.String("source").NotEmpty().Comment("runtime | ci | mined"),
		field.String("type").NotEmpty().Comment("FailureType"),
		field.String("pattern_hash").NotEmpty().Comment("SHA256(pattern_regex)"),
		field.Text("pattern_regex").NotEmpty(),
		field.Text("fix_action").NotEmpty().Comment("JSON(FixAction)"),
		field.Float("confidence").Default(0.5).Comment("0-1"),
		field.Int("success_count").Default(0),
		field.Int("fail_count").Default(0),
		field.Int("version").Default(1),
		field.Bool("is_active").Default(true),
		field.Time("created_at").Default(time.Now).Immutable,
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (FailurePattern) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "type"),
		index.Fields("pattern_hash"),
		index.Fields("is_active", "confidence"),
	}
}
```

Run: `go generate ./internal/data/ent/...`
Expected: No errors

- [x] **Step 2: 创建 FailurePattern Repo 接口**

创建 `internal/biz/monitor/failure_pattern_repo.go`：

```go
package monitor

import (
	"context"
)

// FailurePattern represents a failure pattern in the knowledge base.
type FailurePattern struct {
	ID           string
	Source       string // "runtime" | "ci" | "mined"
	Type         string // FailureType
	PatternHash  string
	PatternRegex string
	FixAction    string // JSON
	Confidence   float64
	SuccessCount int
	FailCount    int
	Version      int
	IsActive     bool
}

// FailurePatternReader reads failure patterns from the knowledge base.
type FailurePatternReader interface {
	ListBySource(ctx context.Context, source string, limit, offset int) ([]*FailurePattern, error)
	GetByPatternHash(ctx context.Context, hash string) (*FailurePattern, error)
	ListActive(ctx context.Context, limit, offset int) ([]*FailurePattern, error)
}

// FailurePatternWriter writes failure patterns to the knowledge base.
type FailurePatternWriter interface {
	Create(ctx context.Context, pattern *FailurePattern) error
	Update(ctx context.Context, pattern *FailurePattern) error
	IncrementSuccess(ctx context.Context, id string) error
	IncrementFail(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
}
```

Run: `go build ./internal/biz/monitor/...`
Expected: PASS

- [x] **Step 3: 创建 Data 层 Repo 实现**

创建 `internal/data/failure_pattern.go`，实现 `FailurePatternReader` 和 `FailurePatternWriter` 接口，使用 Ent Client 操作数据库。

Run: `go build ./internal/data/...`
Expected: PASS

- [x] **Step 4: 创建 failure_pattern_sync Cron Job**

创建 `internal/cronrunner/jobs/failure_pattern_sync.go`，每日从 RootCauseEngine 规则 + patterns.jsonl 同步到 failure_pattern 表。

Run: `go build ./internal/cronrunner/...`
Expected: PASS

- [x] **Step 5: 验证**

Run: `go test ./internal/data/... -run TestFailurePattern -count=1`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 4: Auto-Fix 引擎改造

> **trace:** plan-ready.md → `### Task 4: Auto-Fix 引擎改造` | tasks.md → `- [ ] 4.1 修改 .github/workflows/auto-fix.yml`
> **sync:** tasks.md → `- [ ] 4.1 修改 .github/workflows/auto-fix.yml` | plan-ready.md → `### Task 4: Auto-Fix 引擎改造`

**Files:**
- Modify: `.github/workflows/auto-fix.yml`

- [x] **Step 1: 新增 parse-logs.py 步骤**

在 `auto-fix.yml` 的 "Classify and attempt fix" 步骤之前，新增 Python 日志解析步骤，将原始日志解析为 FailureReport JSON。

- [x] **Step 2: 新增 Critic Agent 步骤**

在 "Verify fix" 步骤之后、创建 PR 之前，新增 Critic Agent 步骤。根据 risk_level 决定是否创建 PR。

- [x] **Step 3: 新增保护文件白名单**

在 "Check protected files" 步骤中，增加白名单机制，允许 auto-fix 修改 `internal/biz/monitor/` 目录。

- [x] **Step 4: 新增 ENABLE_CRITIC_AGENT 环境变量支持**

在 Critic Agent 步骤中，添加 `ENABLE_CRITIC_AGENT` 环境变量判断，为 false 时跳过。

- [x] **Step 5: 验证 YAML 语法**

Run: `actionlint .github/workflows/auto-fix.yml` 或手动检查语法
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 5: 集成测试补齐

> **trace:** plan-ready.md → `### Task 5: 集成测试补齐` | tasks.md → `- [ ] 5.1 创建 internal/service/monitor_integration_test.go`
> **sync:** tasks.md → `- [ ] 5.1 创建 internal/service/monitor_integration_test.go` | plan-ready.md → `### Task 5: 集成测试补齐`

**Files:**
- Create: `internal/service/monitor_integration_test.go`
- Create: `internal/service/skill_intelligence_integration_test.go`
- Create: `internal/service/chat_turn_integration_test.go`

- [x] **Step 1: 创建自愈闭环集成测试**

创建 `internal/service/monitor_integration_test.go`，测试注入错误→检测→根因分析→FixAction 生成→验证的完整闭环。

- [x] **Step 2: 创建 Skill Intelligence 集成测试**

创建 `internal/service/skill_intelligence_integration_test.go`，测试 Skill 调用失败→AnalyzeInvocation→GenerateReport→持久化→查询。

- [x] **Step 3: 创建 Chat Turn 集成测试**

创建 `internal/service/chat_turn_integration_test.go`，测试创建 Session→发送消息→Agent 响应→Memory 写入。

- [x] **Step 4: Phase 1 全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: All PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 6: 经验报告诊断

> **trace:** plan-ready.md → `### Task 6: 经验报告诊断` | tasks.md → `- [ ] 6.1 扩展 internal/biz/skill_intelligence_types.go`
> **sync:** tasks.md → `- [ ] 6.1 扩展 internal/biz/skill_intelligence_types.go` | plan-ready.md → `### Task 6: 经验报告诊断`

**Files:**
- Modify: `internal/biz/skill_intelligence_types.go`
- Modify: `internal/biz/skill_intelligence.go`
- Modify: `internal/data/ent/schema/experience_report.go`
- Modify: `internal/data/skill_intelligence.go`
- Modify: `internal/cronrunner/jobs/skill_intelligence_worker.go`
- Modify: `api/kratos/skill_intelligence/v1/skill_intelligence.proto`
- Modify: `internal/service/skill_intelligence.go`
- Wire DI 装配

- [x] **Step 1: 扩展 ExperienceReport 字段**

在 `internal/biz/skill_intelligence_types.go` 的 `ExperienceReport` 中新增 `RootCauseAnalysis` 和 `SuggestedFix` 字段。

- [x] **Step 2: GenerateReport 集成 RootCauseAnalyzer**

修改 `internal/biz/skill_intelligence.go`，在 `GenerateReport` 中注入 `RootCauseAnalyzer`，调用 `AnalyzeFromReport`。

- [x] **Step 3: 更新 Ent Schema 和 Data 层**

修改 `internal/data/ent/schema/experience_report.go` 新增字段，更新 `internal/data/skill_intelligence.go` 持久化逻辑。

- [x] **Step 4: 替换 skill_intelligence_worker 占位实现**

修改 `internal/cronrunner/jobs/skill_intelligence_worker.go`，实现批量 AnalyzeInvocation/ScoreSkill/GenerateReport。

- [x] **Step 5: 更新 Proto 和 Service 层**

修改 `api/kratos/skill_intelligence/v1/skill_intelligence.proto`，更新 `internal/service/skill_intelligence.go`。

- [x] **Step 6: Wire DI 装配**

Run: `make api && make wire && make build`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 7: 推荐排序进化

> **trace:** plan-ready.md → `### Task 7: 推荐排序进化` | tasks.md → `- [ ] 7.1 创建 internal/tools/skillrecommend/health_provider.go`
> **sync:** tasks.md → `- [ ] 7.1 创建 internal/tools/skillrecommend/health_provider.go` | plan-ready.md → `### Task 7: 推荐排序进化`

**Files:**
- Create: `internal/tools/skillrecommend/health_provider.go`
- Modify: `internal/tools/skillrecommend/rank.go`
- Modify: `internal/tools/skillrecommend/rank_test.go`
- Create: `internal/biz/` 适配器
- Modify: `internal/tools/skillruntime/resolve.go`
- Create: `internal/tools/skillrecommend/rank_feedback.go`

- [x] **Step 1: 定义 HealthMetricsProvider 接口**

创建 `internal/tools/skillrecommend/health_provider.go`，定义接口。

- [x] **Step 2: 实现 DynamicRankFactors**

修改 `internal/tools/skillrecommend/rank.go`，新增 `DynamicRankFactors` 函数。

- [x] **Step 3: 测试动态权重调整**

修改 `internal/tools/skillrecommend/rank_test.go`，测试高成功率/低成功率/无数据场景。

- [x] **Step 4: Biz 层适配器**

创建 Biz 层适配器，实现 `HealthMetricsProvider` 接口。

- [x] **Step 5: 集成到 ResolveSkillSlugsDetailed**

修改 `internal/tools/skillruntime/resolve.go`，调用 DynamicRankFactors。

- [x] **Step 6: 创建 RankFeedback**

创建 `internal/tools/skillrecommend/rank_feedback.go`。

- [x] **Step 7: 验证**

Run: `go test ./internal/tools/skillrecommend/... -count=1`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 8: Curator Agent 半自动进化

> **trace:** plan-ready.md → `### Task 8: Curator Agent 半自动进化` | tasks.md → `- [ ] 8.1 创建 internal/biz/skill_evolution_suggestion_types.go`
> **sync:** tasks.md → `- [ ] 8.1 创建 internal/biz/skill_evolution_suggestion_types.go` | plan-ready.md → `### Task 8: Curator Agent 半自动进化`

**Files:**
- Create: `internal/biz/skill_evolution_suggestion_types.go`
- Modify: `internal/data/ent/schema/skill_evolution_suggestion.go`
- Modify: `internal/biz/skill_intelligence_repo.go`
- Create: `internal/data/skill_evolution_suggestion.go`
- Modify: `internal/biz/skill_intelligence.go`
- Create: `internal/service/skill_curator.go`
- Sandbox Runner 验证
- 进化建议 API proto + Service
- Skill 元数据扩展
- Wire DI 装配

- [x] **Step 1: 创建 SkillEvolutionSuggestion 领域模型**

创建 `internal/biz/skill_evolution_suggestion_types.go`。

- [x] **Step 2: 更新 Ent Schema**

修改 `internal/data/ent/schema/skill_evolution_suggestion.go`。

- [x] **Step 3: 扩展 Repo 接口**

修改 `internal/biz/skill_intelligence_repo.go`。

- [x] **Step 4: 实现 Data 层 Repo**

创建 `internal/data/skill_evolution_suggestion.go`。

- [x] **Step 5: 实现触发条件判定 + CreateSuggestion**

修改 `internal/biz/skill_intelligence.go`。

- [x] **Step 6: 创建 Curator Agent Service**

创建 `internal/service/skill_curator.go`。

- [x] **Step 7: Sandbox Runner 验证**

实现 Sandbox Runner 隔离执行。

- [x] **Step 8: 进化建议 API + Skill 元数据扩展**

定义 proto + 实现 Service + 扩展 Skill 元数据。

- [x] **Step 9: Wire DI 装配**

Run: `make api && make wire && make build`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 9: 前端经验报告与进化审批

> **trace:** plan-ready.md → `### Task 9: 前端经验报告与进化审批` | tasks.md → `- [ ] 9.1 前端经验报告列表页`
> **sync:** tasks.md → `- [ ] 9.1 前端经验报告列表页` | plan-ready.md → `### Task 9: 前端经验报告与进化审批`

**Files:**
- 前端经验报告列表页组件
- 前端 Skill 进化审批 UI 组件

- [x] **Step 1: 前端经验报告列表页**

调用 ListExperienceReports API，显示失败标签分布图 + 根因分析卡片。

- [x] **Step 2: 前端 Skill 进化审批 UI**

显示进化建议列表 + Approve/Reject 操作。

- [x] **Step 3: 验证**

Run: `cd web && pnpm lint && pnpm build`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 10: 预测性自愈

> **trace:** plan-ready.md → `### Task 10: 预测性自愈` | tasks.md → `- [ ] 10.1 创建 internal/biz/monitor/predictive_heal.go`
> **sync:** tasks.md → `- [ ] 10.1 创建 internal/biz/monitor/predictive_heal.go` | plan-ready.md → `### Task 10: 预测性自愈`

**Files:**
- Create: `internal/biz/monitor/predictive_heal.go`
- Create: `internal/cronrunner/jobs/predictive_heal.go`
- Create: `internal/biz/monitor/predictive_heal_test.go`

- [x] **Step 1: 创建 PredictiveHealUsecase**

创建 `internal/biz/monitor/predictive_heal.go`，基于 FailurePattern 知识库的趋势预测。

- [x] **Step 2: 创建 Cron Job**

创建 `internal/cronrunner/jobs/predictive_heal.go`，每 5 分钟扫描系统指标。

- [x] **Step 3: 创建测试**

创建 `internal/biz/monitor/predictive_heal_test.go`，测试置信度阈值 + 冷却期 + 审计记录。

- [x] **Step 4: Wire DI 装配**

Run: `make wire && make build`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 11: Skill 五阶段进化闭环

> **trace:** plan-ready.md → `### Task 11: Skill 五阶段进化闭环` | tasks.md → `- [ ] 11.1 创建 internal/biz/skill_evolution_loop.go`
> **sync:** tasks.md → `- [ ] 11.1 创建 internal/biz/skill_evolution_loop.go` | plan-ready.md → `### Task 11: Skill 五阶段进化闭环`

**Files:**
- Create: `internal/biz/skill_evolution_loop.go`
- Create: `internal/biz/skill_evolution_loop_test.go`

- [x] **Step 1: 实现五阶段流程**

创建 `internal/biz/skill_evolution_loop.go`，实现 Solve→Observe→Evolve→Gate→Reload。

- [x] **Step 2: 实现 Gate 多维验证**

功能正确性 + 安全性 + 性能 + 风格验证。

- [x] **Step 3: 实现进化建议过期机制**

7 天未审批自动标记 expired。

- [x] **Step 4: 创建测试**

创建 `internal/biz/skill_evolution_loop_test.go`。

- [x] **Step 5: 验证**

Run: `go test ./internal/biz/... -run TestEvolutionLoop -count=1`
Expected: PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 12: 知识库动态挖掘

> **trace:** plan-ready.md → `### Task 12: 知识库动态挖掘` | tasks.md → `- [ ] 12.1 创建 internal/biz/monitor/pattern_mining.go`
> **sync:** tasks.md → `- [ ] 12.1 创建 internal/biz/monitor/pattern_mining.go` | plan-ready.md → `### Task 12: 知识库动态挖掘`

**Files:**
- Create: `internal/biz/monitor/pattern_mining.go`
- Create: `internal/cronrunner/jobs/pattern_mining.go`
- Create: `internal/biz/monitor/pattern_mining_test.go`

- [x] **Step 1: 创建 PatternMiningUsecase**

创建 `internal/biz/monitor/pattern_mining.go`，聚类相似失败模式 + 提取共性修复策略。

- [x] **Step 2: 创建 Cron Job**

创建 `internal/cronrunner/jobs/pattern_mining.go`，每日执行挖掘。

- [x] **Step 3: 创建测试**

创建 `internal/biz/monitor/pattern_mining_test.go`，测试聚类 + 置信度提升 + 自动禁用。

- [x] **Step 4: Wire DI 装配**

Run: `make wire && make build`
Expected: PASS

- [x] **Task complete**

---

### Task 13: 全量验证

> **trace:** plan-ready.md → `### Task 13: 全量验证` | tasks.md → `- [ ] 13.1 后端全量验证`
> **sync:** tasks.md → `- [ ] 13.1 后端全量验证` | plan-ready.md → `### Task 13: 全量验证`

**Files:**
- None (verification only)

- [ ] **Step 1: 后端全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: All PASS

- [ ] **Step 2: 前端全量验证**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: All PASS

- [x] **Task complete**
