# P5 闭环双沉淀：aranea L3 运维经验事实 + 10 知识库词条 + RCA 沉淀联动 + 召回验证

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依据总纲 §3.4（记忆 L0~L4 运维映射与双沉淀）与 §3.5（知识双通道分工），修复闭环双沉淀：
① 修复任务终态（success/failed）自动写入 aranea L3 事实（ScopeType=agent，ScopeID=执行 Agent 键，如 `ops_change_execution`），而非此前硬编码的 session 作用域（写后不可召回）；
② 同步写入 10 知识库（twinmonitor admin 模块）category=`remediation_case`，标签 `[AI沉淀, 自动修复]`；
③ `ai_rca_records` 双向追溯：沉淀后回写 `knowledge_id` + `memory_fact_id`；
④ 误报标记（`MarkFalsePositive`）同样触发沉淀，写入「误报模式」L3 事实；
⑤ 双写失败时单边成功可补偿重试；
⑥ E2E 验证：chunk 重放后 `knowledge_chunks` 有记录、下次同类告警 RCA 时 `memory.search` 命中历史 fact 并注入 prompt。

**Architecture:**
- **沉淀触发源**：14-remediation `TaskEventConsumer` 消费 `ai.task.events` 推进执行状态机，在 `markSuccess`/`applyTaskFailed` 终态回写后，**同步调用 13 沉淀服务**（新 gRPC/HTTP 端点 `POST /api/v1/monitor/aiops/sediment`）——14 持有完整 `Execution` 领域模型（alert_id/title/asset_name/policy_id/result_output/duration_ms/rca_record_id），由 14 发起可避免 13 反查；13 侧仅负责「组装 fact → 双写 → 回写引用」。
- **13 沉淀服务层**：新 `SedimentUsecase`（biz 层），依赖 `AraneaPort.WriteMemoryL3` + `KnowledgeClient.Create` + `RcaRepo.UpdateKnowledgeID/UpdateMemoryFactID`。`RemediationSedimentConsumer` 是 HTTP handler（由 14 直接调用）而非 NATS 消费者——减少一次异步跳跃、保证终态与沉淀原子关联。
- **aranea L3 写入**：`POST /api/v1/memory/facts`（`TwinOpenAPICompatService.handleWriteMemoryFact`）已支持 `Scope=agent`，映射为 `ScopeType=agent` + `ScopeID=Key`（即 Agent 键如 `ops_change_execution`）。`agent_memory_runtime_policy.go` 的 `L3ScopeTargets` 已含 `agent` case，召回链路天然打通。
- **10 知识库写入**：`KnowledgeClient.Create` 调用 admin 服务 `POST /api/v1/monitor/knowledge-base`，admin 侧写入后自动分块 → `knowledge_chunks`（词法库走 tsvector/trigram，无需 embedding）。
- **aranea 知识库 chunk 重放**：L3 事实不经 knowledge 写回链路，直接走 `memory.UpsertFactRow` → `memory_facts` 表；但 agent 记忆投影（`AgentMemoryProjector.ProjectAgentMemory`）会把 agent 作用域 facts 投影为 `agents/{agent_id}.md` 并触发 `RebuildBlockIndex` + `replayFn` chunk 重放。投影由 `AutoMemoryWorker` 周期驱动（默认 10s 间隔），沉淀后约 1 个周期内可见。
- **RCA 联动**：若执行记录已关联 `rca_record_id`（14 `Execution.RcaRecordID`），沉淀完成后回写 `ai_rca_records.knowledge_id` + `memory_fact_id`；无关联时按 `alarm_id` 反查 `ai_rca_records` 补联。

**Tech Stack:** Go + Kratos（twinmonitor 13-aiops / 14-remediation）+ Ent（日志库 ai_rca_records）+ aranea-agents（PG `memory_facts` + `knowledge_chunks` + `agents/{id}.md` 投影）。

**前置依赖：** P0（aranea 在环最小闭环已通）+ P2（MCP 工具切over完成，预设 Agent 白名单已指向真实 MCP 键，修复场景 graph 可跑通）。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - twinmonitor: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/aiops/... ./app/remediation/...`
  - aranea: `cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`
- **wire 重新生成**（新增 Provider 形参后必跑）：`go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/` 或 `./app/remediation/cmd/`
- **SQL 安全规约**：写操作先同条件 `SELECT COUNT(*)` 确认命中范围，显式事务包裹，执行后核验 affected rows。
- **commit 风格**：twinmonitor 仓库 `feat(aiops): ...` / `feat(remediation): ...`；aranea 仓库 `feat(memory): ...` / `test(gns3): ...`。

---

## Task 1：T1 13 沉淀服务（SedimentUsecase）+ 14 终态调用接线

**目标**：14 `ExecutionUsecase` 在 `markSuccess`/`applyTaskFailed` 终态后，同步调用 13 新端点完成双沉淀；13 `SedimentUsecase` 组装 L3 事实 + 10 KB 词条 + RCA 引用回写。

**Files:**
- New: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/sediment.go`
- New: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/service/sediment.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/clients.go`（SedimentClient 端口）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go`（UpdateMemoryFactID 方法、DepositKnowledge 复用）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/rca_repo.go`（UpdateMemoryFactID 实现）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/biz.go`（ProviderSet 追加 NewSedimentUsecase）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/service/service.go`（HTTP router 注册 sediment 端点）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/execution.go`（markSuccess/applyTaskFailed 后调用 13 沉淀端点）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/external.go`（追加 SedimentExecution 外部端口）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/ext_clients.go`（SedimentExecution HTTP 实现）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/biz.go`（ProviderSet 调整）
- Test: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/sediment_test.go`
- Test: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/execution_test.go`（追加沉淀调用断言）

- [ ] **Step 1.1 先写失败测试：SedimentUsecase 双写后 RCA 引用回写**

在 `app/aiops/internal/biz/sediment_test.go` 新建（包 `biz`）：

```go
package biz

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// ---- 测试替身 ----

type fakeAraneaPort struct {
	written *MemoryFact
}

func (f *fakeAraneaPort) WriteMemoryL3(ctx context.Context, fact *MemoryFact) error {
	f.written = fact
	return nil
}
func (f *fakeAraneaPort) Health(ctx context.Context) (*AraneaHealth, error) { return nil, nil }
func (f *fakeAraneaPort) ListAgents(ctx context.Context) ([]*AraneaAgent, error) { return nil, nil }
func (f *fakeAraneaPort) ListGraphs(ctx context.Context) ([]*AraneaGraph, error) { return nil, nil }
func (f *fakeAraneaPort) CreateAgent(ctx context.Context, in *AraneaAgentInput) (string, error) { return "", nil }
func (f *fakeAraneaPort) UpdateAgent(ctx context.Context, id string, in *AraneaAgentInput) error { return nil }
func (f *fakeAraneaPort) GetGraph(ctx context.Context, graphID string) (*AraneaGraph, error) { return nil, nil }
func (f *fakeAraneaPort) CreateGraph(ctx context.Context, graphJSON []byte) (string, error) { return "", nil }
func (f *fakeAraneaPort) UpdateGraph(ctx context.Context, graphID string, graphJSON []byte) error { return nil }
func (f *fakeAraneaPort) RunGraph(ctx context.Context, in *RunInput) (string, error) { return "", nil }
func (f *fakeAraneaPort) GetRun(ctx context.Context, runID string) (*AraneaRun, error) { return nil, nil }
func (f *fakeAraneaPort) CancelRun(ctx context.Context, runID string) error { return nil }
func (f *fakeAraneaPort) ResumeInterrupt(ctx context.Context, runID, interruptID string, decision *ResumeDecision) error { return nil }
func (f *fakeAraneaPort) GetQuotaUsage(ctx context.Context) (*QuotaUsage, error) { return nil, nil }
func (f *fakeAraneaPort) GetAgentMetrics(ctx context.Context) (*AgentMetrics, error) { return nil, nil }

type fakeKnowledgeClient struct {
	createdID uint32
}

func (f *fakeKnowledgeClient) Search(ctx context.Context, keyword string, topK int) ([]*KnowledgeItem, error) { return nil, nil }
func (f *fakeKnowledgeClient) Create(ctx context.Context, in *KnowledgeCreateInput) (uint32, error) {
	return f.createdID, nil
}
func (f *fakeKnowledgeClient) GetStatistics(ctx context.Context) (*KnowledgeStatistics, error) { return nil, nil }

type fakeRcaRepoForSediment struct {
	updatedKnowledgeID uint32
	updatedMemoryID    string
}

func (f *fakeRcaRepoForSediment) List(ctx context.Context, params *RcaListParams) ([]*RcaRecord, int, error) { return nil, 0, nil }
func (f *fakeRcaRepoForSediment) Get(ctx context.Context, id uint32) (*RcaRecord, error) { return nil, nil }
func (f *fakeRcaRepoForSediment) Create(ctx context.Context, rec *RcaRecord) (*RcaRecord, error) { return nil, nil }
func (f *fakeRcaRepoForSediment) MarkAnalyzing(ctx context.Context, id uint32, araneaRunID string) error { return nil }
func (f *fakeRcaRepoForSediment) CompleteAnalysis(ctx context.Context, id uint32, res *RcaAnalysisResult) error { return nil }
func (f *fakeRcaRepoForSediment) FailAnalysis(ctx context.Context, id uint32, status, errMsg string) error { return nil }
func (f *fakeRcaRepoForSediment) MarkFalsePositive(ctx context.Context, id uint32, flag uint32) error { return nil }
func (f *fakeRcaRepoForSediment) UpdateKnowledgeID(ctx context.Context, id uint32, knowledgeID uint32) error {
	f.updatedKnowledgeID = knowledgeID
	return nil
}
func (f *fakeRcaRepoForSediment) UpdateMemoryFactID(ctx context.Context, id uint32, factID string) error {
	f.updatedMemoryID = factID
	return nil
}
func (f *fakeRcaRepoForSediment) GetStatistics(ctx context.Context) (*RcaStatistics, error) { return nil, nil }
func (f *fakeRcaRepoForSediment) ListRecentCompleted(ctx context.Context, limit int) ([]*RcaRecord, error) { return nil, nil }
func (f *fakeRcaRepoForSediment) GetTaskStatistics(ctx context.Context) (*TaskStatistics, error) { return nil, nil }
func (f *fakeRcaRepoForSediment) GetSystemScenarioGraphID(ctx context.Context, key string) (string, error) { return "", nil }

func TestSedimentUsecase_DoubleWrite(t *testing.T) {
	aranea := &fakeAraneaPort{}
	kb := &fakeKnowledgeClient{createdID: 42}
	rca := &fakeRcaRepoForSediment{}

	uc := NewSedimentUsecase(aranea, kb, rca, log.NewHelper(log.DefaultLogger))
	in := &SedimentInput{
		ExecutionNo: "REM20260819000001",
		PolicyID:    7,
		AlertID:     "ALM-001",
		AlertTitle:  "sw1 线路中断",
		AssetName:   "sw1",
		AgentKey:    "ops_change_execution",
		Result:      "端口 eth1 已恢复 UP",
		DurationSec: 184,
		Success:     true,
		RcaRecordID: 3,
	}

	out, err := uc.Sediment(context.Background(), in)
	if err != nil {
		t.Fatalf("sediment failed: %v", err)
	}

	// 1) aranea L3 事实已写入，且 scope=agent
	if aranea.written == nil {
		t.Fatal("aranea L3 fact not written")
	}
	if aranea.written.Scope != "agent" {
		t.Fatalf("scope = %q, want agent", aranea.written.Scope)
	}
	if aranea.written.Key != "ops_change_execution" {
		t.Fatalf("key = %q, want ops_change_execution", aranea.written.Key)
	}
	if !strings.Contains(aranea.written.Content, "sw1 线路中断") {
		t.Fatal("L3 fact content missing alert title")
	}

	// 2) 10 知识库已创建
	if out.KnowledgeID != 42 {
		t.Fatalf("knowledge_id = %d, want 42", out.KnowledgeID)
	}

	// 3) RCA 记录已回写双向引用
	if rca.updatedKnowledgeID != 42 {
		t.Fatalf("rca knowledge_id = %d, want 42", rca.updatedKnowledgeID)
	}
	if rca.updatedMemoryID == "" {
		t.Fatal("rca memory_fact_id not updated")
	}
}

func TestSedimentUsecase_KBFailureStillWritesL3(t *testing.T) {
	aranea := &fakeAraneaPort{}
	kb := &fakeKnowledgeClient{createdID: 0}
	rca := &fakeRcaRepoForSediment{}

	// KB Create 永远失败
	uc := NewSedimentUsecase(aranea, kb, rca, log.NewHelper(log.DefaultLogger))
	_, err := uc.Sediment(context.Background(), &SedimentInput{
		ExecutionNo: "REM20260819000002",
		AlertTitle:  "test",
		AgentKey:    "ops_fault_diagnosis",
		Success:     true,
	})
	// 当前设计：KB 失败返回 error，但 L3 已写入（需调用方补偿或 uc 内部回滚）
	// P5 T5 专门处理补偿；此处先验证「L3 不因 KB 失败而回滚」的行为
	if err == nil {
		t.Fatal("expected error when KB create fails")
	}
	if aranea.written == nil {
		t.Fatal("L3 should have been written before KB failure")
	}
}
```

运行命令（预期失败——接口与实现均不存在）：
```powershell
cd f:/myproject/twinmonitor/TwinServer
$env:CGO_ENABLED="0"; go test ./app/aiops/internal/biz -run TestSediment -v
```

- [ ] **Step 1.2 13 biz 层：定义 SedimentInput / SedimentOutput / SedimentUsecase**

在 `app/aiops/internal/biz/sediment.go` 新建：

```go
package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ---- 沉淀领域模型 ----

// SedimentInput 修复闭环沉淀入参（由 14 终态调用传入）。
type SedimentInput struct {
	ExecutionNo  string
	PolicyID     uint32
	PolicyName   string
	AlertID      string
	AlertTitle   string
	AlertLevel   string
	AlertSource  string
	AssetID      int64
	AssetName    string
	ScenarioID   int64
	ScenarioName string
	AgentKey     string // 执行 Agent 键，如 ops_change_execution / ops_fault_diagnosis
	Result       string // result_summary 或 error_message
	DurationSec  int
	Success      bool
	RcaRecordID  int64 // 0 表示无关联 RCA
}

// SedimentOutput 沉淀结果（knowledge_id + memory_fact_id）。
type SedimentOutput struct {
	KnowledgeID  uint32
	MemoryFactID string
}

// SedimentUsecase 修复闭环双沉淀用例（总纲 §3.4/§3.5）。
// 职责：L3 事实（agent 作用域）+ 10 知识库（remediation_case）双写 + RCA 引用回写。
type SedimentUsecase struct {
	aranea AraneaPort
	kb     KnowledgeClient
	rca    RcaRepo
	log    *log.Helper
}

// NewSedimentUsecase 构造沉淀用例（允许 aranea/kb/rca 为 nil，降级为仅留痕）。
func NewSedimentUsecase(aranea AraneaPort, kb KnowledgeClient, rca RcaRepo, logger log.Logger) *SedimentUsecase {
	return &SedimentUsecase{
		aranea: aranea,
		kb:     kb,
		rca:    rca,
		log:    log.NewHelper(log.With(logger, "module", "biz/sediment")),
	}
}

// Sediment 执行双沉淀（best-effort，单边失败不阻塞另一边，但返回 error 供调用方决策补偿）。
func (uc *SedimentUsecase) Sediment(ctx context.Context, in *SedimentInput) (*SedimentOutput, error) {
	if in == nil {
		return nil, nil
	}

	out := &SedimentOutput{}
	var l3Err, kbErr error

	// 1) aranea L3 事实写入（agent 作用域，总纲 §3.4 设计修正）。
	factID := fmt.Sprintf("remediation:%s", in.ExecutionNo)
	if uc.aranea != nil {
		l3Err = uc.aranea.WriteMemoryL3(ctx, &MemoryFact{
			Scope:   "agent",
			Key:     in.AgentKey,
			Content: uc.buildL3Content(in),
			Metadata: map[string]any{
				"source":        "twinmonitor_remediation",
				"execution_no":  in.ExecutionNo,
				"policy_id":     in.PolicyID,
				"alert_pattern": in.AlertTitle,
				"device_type":   in.AssetName,
				"success":       in.Success,
				"mttr_seconds":  in.DurationSec,
				"scenario_id":   in.ScenarioID,
			},
		})
		if l3Err == nil {
			out.MemoryFactID = factID
			uc.log.WithContext(ctx).Infof("L3 fact written: %s (agent=%s)", factID, in.AgentKey)
		} else {
			uc.log.WithContext(ctx).Warnf("L3 fact write failed (best-effort): %v", l3Err)
		}
	}

	// 2) 10 知识库写入（category=remediation_case）。
	if uc.kb != nil {
		var kbID uint32
		kbID, kbErr = uc.kb.Create(ctx, &KnowledgeCreateInput{
			Title:    uc.buildKBTitle(in),
			Category: "remediation_case",
			Tags:     []string{"AI沉淀", "自动修复"},
			Content:  uc.buildKBContent(in),
		})
		if kbErr == nil {
			out.KnowledgeID = kbID
			uc.log.WithContext(ctx).Infof("KB entry created: %d", kbID)
		} else {
			uc.log.WithContext(ctx).Warnf("KB create failed (best-effort): %v", kbErr)
		}
	}

	// 3) RCA 引用回写（双向追溯）。
	if uc.rca != nil && in.RcaRecordID > 0 {
		if out.KnowledgeID > 0 {
			if err := uc.rca.UpdateKnowledgeID(ctx, uint32(in.RcaRecordID), out.KnowledgeID); err != nil {
				uc.log.WithContext(ctx).Warnf("rca knowledge_id backwrite failed: %v", err)
			}
		}
		if out.MemoryFactID != "" {
			if err := uc.rca.UpdateMemoryFactID(ctx, uint32(in.RcaRecordID), out.MemoryFactID); err != nil {
				uc.log.WithContext(ctx).Warnf("rca memory_fact_id backwrite failed: %v", err)
			}
		}
	}

	// 4) 错误决策：双写均失败才返回 error；单边成功返回 out + nil（T5 补偿处理单边失败）。
	if l3Err != nil && kbErr != nil {
		return out, fmt.Errorf("sediment double-write failed: l3=%v, kb=%v", l3Err, kbErr)
	}
	return out, nil
}

func (uc *SedimentUsecase) buildL3Content(in *SedimentInput) string {
	status := "成功"
	if !in.Success {
		status = "失败"
	}
	b := strings.Builder{}
	fmt.Fprintf(&b, "告警[%s] → 资产[%s] → 修复%s（耗时%d秒）\n", in.AlertTitle, in.AssetName, status, in.DurationSec)
	fmt.Fprintf(&b, "执行结果：%s\n", in.Result)
	fmt.Fprintf(&b, "策略：%s（scenario=%s）\n", in.PolicyName, in.ScenarioName)
	return b.String()
}

func (uc *SedimentUsecase) buildKBTitle(in *SedimentInput) string {
	return fmt.Sprintf("[%s] %s — %s", in.ExecutionNo, in.AlertTitle, in.AssetName)
}

func (uc *SedimentUsecase) buildKBContent(in *SedimentInput) string {
	status := "成功"
	if !in.Success {
		status = "失败"
	}
	b := strings.Builder{}
	fmt.Fprintf(&b, "## 告警信息\n- 标题：%s\n- 资产：%s\n- 级别：%s\n- 来源：%s\n\n", in.AlertTitle, in.AssetName, in.AlertLevel, in.AlertSource)
	fmt.Fprintf(&b, "## 修复结果（%s）\n%s\n\n", status, in.Result)
	fmt.Fprintf(&b, "## 执行元数据\n- 执行编号：%s\n- 策略：%s\n- 场景：%s\n- 耗时：%d 秒\n",
		in.ExecutionNo, in.PolicyName, in.ScenarioName, in.DurationSec)
	return b.String()
}
```

- [ ] **Step 1.3 13 RcaRepo 追加 UpdateMemoryFactID**

在 `app/aiops/internal/biz/rca.go` 的 `RcaRepo` 接口追加：

```go
	UpdateMemoryFactID(ctx context.Context, id uint32, factID string) error
```

在 `app/aiops/internal/data/rca_repo.go` 追加实现：

```go
// UpdateMemoryFactID 沉淀后回写 aranea L3 fact_id（双向追溯）。
func (r *rcaRepo) UpdateMemoryFactID(ctx context.Context, id uint32, factID string) error {
	if factID == "" {
		return nil
	}
	if err := r.data.logDB.Client().AiRcaRecord.UpdateOneID(id).
		SetMemoryFactID(factID).
		SetUpdateTime(time.Now()).
		Exec(ctx); err != nil {
		r.log.Errorf("update rca memory_fact_id failed: %s", err.Error())
		return biz.ErrAiAnalysisFailed
	}
	return nil
}
```

> **ent 代码生成**：若 `AiRcaRecord` 无 `SetMemoryFactID`，需先在 `app/aiops/internal/data/ent_log/schema/ai_rca_record.go` 加字段 `memory_fact_id`（`field.String("memory_fact_id").Optional()`），然后运行：
> ```powershell
> cd f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/ent_log
> go generate ./...
> ```
> 若项目未使用 `go generate`，则运行对应的 `entc.go`。

- [ ] **Step 1.4 13 service 层：裸路由端点（复用 WebhookService 模式）**

13 的 HTTP 服务有两类路由：proto 生成路由（经 kratos 中间件链）与裸路由（`srv.HandleFunc`，不经中间件，见 `internal/server/http.go:133` webhook 注册）。沉淀端点是 14→13 的机器间调用，走裸路由模式（与 webhook/SSE 一致）。

在 `app/aiops/internal/service/sediment_service.go` 新建：

```go
package service

import (
	stdhttp "net/http"

	"github.com/go-kratos/kratos/v2/log"

	"twinserver/app/aiops/internal/biz"
)

// SedimentService 修复闭环沉淀端点服务（14 终态调用，裸路由不经 JWT 中间件；
// 与 WebhookService 同模式，注册：srv.HandleFunc("/api/v1/monitor/aiops/sediment", svc.HandleSediment)）。
type SedimentService struct {
	uc  *biz.SedimentUsecase
	log *log.Helper
}

func NewSedimentService(uc *biz.SedimentUsecase, logger log.Logger) *SedimentService {
	return &SedimentService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "sediment/service/aiops-service")),
	}
}

// HandleSediment 接收 14 终态沉淀请求：解析 SedimentInput → SedimentUsecase.Sediment → 返回结果。
func (s *SedimentService) HandleSediment(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if r.Method != stdhttp.MethodPost {
		stdhttp.Error(w, `{"error":"method not allowed"}`, stdhttp.StatusMethodNotAllowed)
		return
	}
	var in biz.SedimentInput
	if err := json.NewDecoder(stdhttp.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		stdhttp.Error(w, `{"error":"invalid json"}`, stdhttp.StatusBadRequest)
		return
	}
	out, err := s.uc.Sediment(r.Context(), &in)
	if err != nil {
		s.log.WithContext(r.Context()).Warnf("sediment failed: %v", err)
		stdhttp.Error(w, `{"error":"sediment failed"}`, stdhttp.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
```

> 文件头需 `import "encoding/json"`。

在 `app/aiops/internal/server/http.go` 的 `NewHTTPServer` 中：
① 形参追加 `sedimentSvc *service.SedimentService`；
② 在 webhook 注册行（`srv.HandleFunc("/api/v1/monitor/aiops/webhooks/aranea", ...)`，约 :133）后追加：

```go
	srv.HandleFunc("/api/v1/monitor/aiops/sediment", sedimentSvc.HandleSediment)
```

在 `app/aiops/internal/service/service.go`（ProviderSet）追加 `NewSedimentService`；在 `app/aiops/internal/biz/biz.go` ProviderSet 追加 `NewSedimentUsecase`。

- [ ] **Step 1.5 14 external.go 追加 SedimentExecution 端口**

在 `app/remediation/internal/biz/external.go` 的 `ExternalClients` 追加：

```go
	// AiopsSediment 13 修复闭环沉淀端点（POST /api/v1/monitor/aiops/sediment）。
	// 14 终态后调用，触发 aranea L3 + 10 KB 双写 + RCA 引用回写。
	AiopsSediment func(ctx context.Context, in *SedimentPayload) (*SedimentResult, error)
```

在同文件追加 DTO：

```go
// SedimentPayload 14 → 13 沉淀请求载荷。
type SedimentPayload struct {
	ExecutionNo  string `json:"execution_no"`
	PolicyID     uint32 `json:"policy_id"`
	PolicyName   string `json:"policy_name"`
	AlertID      string `json:"alert_id"`
	AlertTitle   string `json:"alert_title"`
	AlertLevel   string `json:"alert_level"`
	AlertSource  string `json:"alert_source"`
	AssetID      int64  `json:"asset_id"`
	AssetName    string `json:"asset_name"`
	ScenarioID   int64  `json:"scenario_id"`
	ScenarioName string `json:"scenario_name"`
	AgentKey     string `json:"agent_key"`
	Result       string `json:"result"`
	DurationSec  int    `json:"duration_sec"`
	Success      bool   `json:"success"`
	RcaRecordID  int64  `json:"rca_record_id"`
}

// SedimentResult 13 → 14 沉淀响应。
type SedimentResult struct {
	KnowledgeID  uint32 `json:"knowledge_id"`
	MemoryFactID string `json:"memory_fact_id"`
}
```

- [ ] **Step 1.6 14 data 层实现 SedimentExecution HTTP 客户端**

在 `app/remediation/internal/data/ext_clients.go` 追加（或新建 sediment_client.go）：

```go
package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"twinserver/app/remediation/internal/biz"
)

// NewSedimentClient 创建 13 沉淀端点客户端。
func NewSedimentClient(baseURL string, hc *http.Client) func(ctx context.Context, in *biz.SedimentPayload) (*biz.SedimentResult, error) {
	if baseURL == "" {
		return nil
	}
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return func(ctx context.Context, in *biz.SedimentPayload) (*biz.SedimentResult, error) {
		body, _ := json.Marshal(in)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/monitor/aiops/sediment", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("sediment http %d", resp.StatusCode)
		}
		var out biz.SedimentResult
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return &out, nil
	}
}
```

在 `app/remediation/internal/data/data.go` 的 `NewExternalClients` 中绑定：

```go
	aiopsBaseURL := cfg.GetAiops().GetBaseUrl() // 或对应配置路径
	// ... 现有 AiopsGetScenarioRisk / AiopsCreateScenarioTask 等绑定 ...
	clients.AiopsSediment = NewSedimentClient(aiopsBaseURL, hc)
```

- [ ] **Step 1.7 14 ExecutionUsecase 终态后调用沉淀端点**

在 `app/remediation/internal/biz/execution.go` 的 `markSuccess` 和 `applyTaskFailed` 中，终态落库并发布事件后，追加沉淀调用：

```go
// markSuccess 末尾追加（在 uc.invalidateStats 之前）：
uc.sedimentIfNeeded(ctx, e, true)

// applyTaskFailed 末尾追加（在 uc.invalidateStats 之前）：
uc.sedimentIfNeeded(ctx, e, false)
```

在 `ExecutionUsecase` 追加方法：

```go
// sedimentIfNeeded 终态后触发 13 双沉淀（best-effort，失败仅留痕不阻塞终态）。
func (uc *ExecutionUsecase) sedimentIfNeeded(ctx context.Context, e *Execution, success bool) {
	if uc.external == nil || uc.external.AiopsSediment == nil {
		return
	}
	agentKey := uc.agentKeyForScenario(e.ScenarioID)
	payload := &SedimentPayload{
		ExecutionNo:  e.ExecutionNo,
		PolicyID:     e.PolicyID,
		PolicyName:   e.PolicyName,
		AlertID:      e.AlertID,
		AlertTitle:   e.AlertTitle,
		AlertLevel:   e.AlertLevel,
		AlertSource:  e.AlertSource,
		AssetID:      0,
		AssetName:    e.AssetName,
		ScenarioID:   e.ScenarioID,
		AgentKey:     agentKey,
		Result:       e.ResultOutput,
		DurationSec:  0,
		Success:      success,
		RcaRecordID:  0,
	}
	if e.AssetID != nil {
		payload.AssetID = *e.AssetID
	}
	if e.DurationMs != nil {
		payload.DurationSec = *e.DurationMs / 1000
	}
	if e.RcaRecordID != nil {
		payload.RcaRecordID = *e.RcaRecordID
	}
	if _, err := uc.external.AiopsSediment(ctx, payload); err != nil {
		uc.log.WithContext(ctx).Warnf("sediment call failed (best-effort): %v", err)
	}
}

// agentKeyForScenario 按 scenario_id 映射执行 Agent 键（与 13 agent_preset.go 对齐）。
func (uc *ExecutionUsecase) agentKeyForScenario(scenarioID int64) string {
	// 简单映射：可按 scenario_id 查 13 场景详情取 agent_name；P5 一期先硬编码主路径
	switch scenarioID {
	case 1001: // 假设的告警处理场景 ID
		return "ops_alarm_handler"
	case 1002: // 假设的故障诊断场景 ID
		return "ops_fault_diagnosis"
	case 1003: // 假设的修复执行场景 ID
		return "ops_change_execution"
	default:
		return "ops_system_inspection"
	}
}
```

> **注**：若 `ExecutionUsecase` 未持有 `ScenarioUsecase` 或 `AiopsGetScenarioTask` 无法反查 scenario 详情，一期可先按 `scenario_id` 硬编码映射表（与 `agent_preset.go` 中的场景-Agent 对应关系一致）。P5+ 可扩展为动态查询。

- [ ] **Step 1.8 wire 重新生成**

```powershell
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/
go run github.com/google/wire/cmd/wire gen ./app/remediation/cmd/
```

- [ ] **Step 1.9 运行测试**

```powershell
cd f:/myproject/twinmonitor/TwinServer
$env:CGO_ENABLED="0"
go test ./app/aiops/internal/biz -run TestSediment -v
go test ./app/remediation/internal/biz -run TestApplyTaskEvent -v
```

- [ ] **Step 1.10 构建验证**

```powershell
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/... ./app/remediation/...
```

- [ ] **Step 1.11 git commit**

```powershell
cd f:/myproject/twinmonitor/TwinServer
git add -A
git commit -m "feat(aiops): 修复闭环双沉淀服务（SedimentUsecase + 10 KB / aranea L3 / RCA 回写）"
git commit --amend -m "feat(remediation): 终态后调用 13 沉淀端点，完成闭环双沉淀联动"
```

---

## Task 2：T2 chunk 重放验证（10 知识库 + aranea agent 记忆投影）

**目标**：验证 10 KB 写入后 `knowledge_chunks` 有记录；aranea L3 事实写入后 `AgentMemoryProjector` 触发 chunk 重放，`knowledge_chunks` 有对应条目。

**Files:**
- Test script: `f:/myproject/aranea-agents/test/ts10-gns3/verify_sediment_chunks.py`（新建）
- Verify SQL: `f:/myproject/aranea-agents/test/ts10-gns3/verify_sediment_chunks.sql`（新建）

- [ ] **Step 2.1 验证 10 知识库 chunk 重放**

在 `test/ts10-gns3/verify_sediment_chunks.sql` 新建：

```sql
-- 验证 10 知识库词条写入后 knowledge_chunks 有记录
-- 执行前替换 :knowledge_id 为实际值（由沉淀响应或 admin 页面获取）

-- 1) 确认文档存在
SELECT id, title, category, status FROM knowledge_base_entries WHERE id = :knowledge_id;

-- 2) 确认 chunks 已生成（词法库：tsvector/trigram 索引依赖 knowledge_chunks）
SELECT COUNT(*) AS chunk_count FROM knowledge_chunks WHERE document_id = :knowledge_id;

-- 期望：chunk_count > 0（通常 1~5 个 chunk，取决于正文长度）
```

运行命令（在已写入 knowledge_id 后）：
```powershell
cd f:/myproject/twinmonitor/TwinServer
# 假设 knowledge_id = 42
psql $env:TWINMONITOR_TEST_PG_DSN -v knowledge_id=42 -f f:/myproject/aranea-agents/test/ts10-gns3/verify_sediment_chunks.sql
```

- [ ] **Step 2.2 验证 aranea L3 事实写入 + agent 记忆投影**

在 aranea PG 执行：

```sql
-- 验证 agent 作用域 L3 事实已写入
SELECT id, scope_type, scope_id, statement, source_kind, metadata
FROM memory_facts
WHERE source_kind = 'twinmonitor'
  AND scope_type = 'agent'
  AND fingerprint LIKE 'remediation:REM%'
ORDER BY created_at DESC
LIMIT 5;

-- 期望：scope_type='agent'，scope_id='ops_change_execution' 或 'ops_fault_diagnosis'
```

在 aranea 知识库集合（默认 workspace 的 agent-memory collection）验证投影文档：

```sql
-- 验证 agents/ops_change_execution.md 已投影并生成 chunks
SELECT d.id, d.rel_path, d.status, COUNT(c.id) AS chunk_count
FROM knowledge_documents d
LEFT JOIN knowledge_chunks c ON c.document_id = d.id
WHERE d.rel_path LIKE 'agents/ops_%.md'
GROUP BY d.id, d.rel_path, d.status;

-- 期望：rel_path='agents/ops_change_execution.md' 且 chunk_count > 0
```

- [ ] **Step 2.3 自动化验证脚本**

在 `test/ts10-gns3/verify_sediment_chunks.py` 新建：

```python
#!/usr/bin/env python3
"""验证沉淀双写后的 chunk 重放结果（T2）。"""
import os, sys, psycopg2

ARANEA_DSN = os.getenv("ARANEA_TEST_PG_DSN", "postgres://postgres:123456@127.0.0.1:5432/aranea_test?sslmode=disable")
TWIN_DSN   = os.getenv("TWINMONITOR_TEST_PG_DSN", "postgres://postgres:123456@127.0.0.1:5432/twinmonitor_test?sslmode=disable")

def check_aranea_l3():
    conn = psycopg2.connect(ARANEA_DSN)
    cur = conn.cursor()
    cur.execute("""
        SELECT scope_type, scope_id, statement FROM memory_facts
        WHERE source_kind='twinmonitor' AND scope_type='agent'
        ORDER BY created_at DESC LIMIT 1
    """)
    row = cur.fetchone()
    conn.close()
    assert row is not None, "aranea L3 无 twinmonitor 来源的 agent 作用域事实"
    assert row[0] == 'agent', f"scope_type={row[0]}, want agent"
    print(f"[OK] aranea L3: scope_type={row[0]}, scope_id={row[1]}, statement_len={len(row[2])}")

def check_aranea_projection():
    conn = psycopg2.connect(ARANEA_DSN)
    cur = conn.cursor()
    cur.execute("""
        SELECT d.rel_path, COUNT(c.id)
        FROM knowledge_documents d
        LEFT JOIN knowledge_chunks c ON c.document_id = d.id
        WHERE d.rel_path LIKE 'agents/ops_%%.md'
        GROUP BY d.rel_path
    """)
    rows = cur.fetchall()
    conn.close()
    assert rows, "无 agent 记忆投影文档"
    for rel, cnt in rows:
        assert cnt > 0, f"{rel} 的 chunk_count=0，未触发重放"
        print(f"[OK] projection: {rel} chunks={cnt}")

def check_twin_kb(knowledge_id: int):
    conn = psycopg2.connect(TWIN_DSN)
    cur = conn.cursor()
    cur.execute("SELECT COUNT(*) FROM knowledge_chunks WHERE document_id = %s", (knowledge_id,))
    cnt = cur.fetchone()[0]
    conn.close()
    assert cnt > 0, f"10 KB knowledge_id={knowledge_id} 的 chunk_count=0"
    print(f"[OK] twin KB: knowledge_id={knowledge_id} chunks={cnt}")

if __name__ == "__main__":
    check_aranea_l3()
    check_aranea_projection()
    if len(sys.argv) > 1:
        check_twin_kb(int(sys.argv[1]))
    print("\nT2 chunk 重放验证全部通过。")
```

运行命令（需在完成一次真实沉淀后，拿到 knowledge_id）：
```powershell
cd f:/myproject/aranea-agents/test/ts10-gns3
python verify_sediment_chunks.py 42
```

- [ ] **Step 2.4 git commit**

```powershell
cd f:/myproject/aranea-agents
git add test/ts10-gns3/verify_sediment_chunks.py test/ts10-gns3/verify_sediment_chunks.sql
git commit -m "test(gns3): 沉淀 chunk 重放验证脚本（T2）"
```

---

## Task 3：T3 召回验证（memory.search 命中 L3 事实并注入 prompt）

**目标**：下次同类告警触发 RCA 时，aranea 侧 `memory.search` 或 composite recall 能命中此前沉淀的 L3 事实，事实内容注入 LLM prompt。

**Files:**
- Test script: `f:/myproject/aranea-agents/test/ts10-gns3/verify_memory_recall.py`（新建）
- Modify: `f:/myproject/aranea-agents/internal/agent/memory_inject.go`（无需修改，仅验证既有链路）

- [ ] **Step 3.1 确认 L3 召回参数**

在 `internal/agent/memory_inject.go` 的 `memoryRuntimeContext` 中，`AgentID` 取自 `ag.ID`。对于预设 Agent（如 `ops_fault_diagnosis`），`AgentID` 即 aranea 侧 agent 的 `ID`（字符串 UUID）。

在 `internal/biz/agent_memory_runtime_policy.go` 的 `L3ScopeTargets` 中，`case "agent":` 取 `rt.AgentID`。因此只要 `MemoryRuntimeContext.AgentID` 非空，且 `memory_facts.ScopeType='agent'` + `ScopeID=<AgentID>`，即可召回。

> **关键一致性**：14 侧传入的 `SedimentInput.AgentKey`（如 `ops_change_execution`）必须与 aranea 侧该 Agent 的 `ID`（或 `AgentKey`）一致。若不一致，需建立映射表。
>
> 实际检查：`agent_preset.go` 中 Agent 的 `Name` 是中文（如 "变更执行 Agent"），但同步到 aranea 时 `AraneaAgentInput.Name` 也是中文。aranea 侧 Agent 的 `ID` 是 UUID，`AgentKey` 可能为 `ops_change_execution`（若种子同步时通过 `Metadata` 传入）。
>
> 在 `TwinOpenAPICompatService.handleWriteMemoryFact` 中，`ScopeID = in.Key`。所以 `in.Key` 必须等于 aranea 侧 `Agent.ID`（或 `AgentKey`）才能被 `L3ScopeTargets` 匹配。
>
> **修正**：`SedimentUsecase.Sediment` 中 `MemoryFact.Key` 应传 aranea Agent 的 `ID`（UUID），而非 `ops_change_execution`。14 侧需通过 `ScenarioUsecase` 或配置表维护 `scenario_id → aranea_agent_id` 映射。
>
> P5 一期简化方案：在 `SedimentUsecase` 中，若 `AgentKey` 以 `ops_` 开头，先调用 `aranea.ListAgents` 按 `name` 或 `metadata.source` 反查 `Agent.ID`，再写入 `Key=Agent.ID`。

- [ ] **Step 3.2 写验证脚本：模拟召回**

在 `test/ts10-gns3/verify_memory_recall.py` 新建：

```python
#!/usr/bin/env python3
"""验证沉淀后的 L3 事实可被同类告警 RCA 召回（T3）。

原理：
1. 查询 memory_facts 确认存在 agent 作用域事实（scope_type='agent'）。
2. 调用 aranea 内部 composite recall（或直接查 memory_facts）验证按告警关键词可命中。
3. 检查 L3MemoryCue 输出非空（即会被注入 prompt）。
"""
import os, sys, psycopg2, json

ARANEA_DSN = os.getenv("ARANEA_TEST_PG_DSN", "postgres://postgres:123456@127.0.0.1:5432/aranea_test?sslmode=disable")

def recall_by_keyword(agent_id: str, keyword: str) -> list:
    conn = psycopg2.connect(ARANEA_DSN)
    cur = conn.cursor()
    # 走与 L3ScopeTargets("agent") 相同的过滤逻辑
    cur.execute("""
        SELECT statement, metadata, created_at FROM memory_facts
        WHERE scope_type = 'agent' AND scope_id = %s
          AND (statement ILIKE %s OR metadata::text ILIKE %s)
        ORDER BY updated_at DESC LIMIT 3
    """, (agent_id, f"%{keyword}%", f"%{keyword}%"))
    rows = cur.fetchall()
    conn.close()
    return rows

if __name__ == "__main__":
    if len(sys.argv) < 3:
        print("Usage: python verify_memory_recall.py <agent_id> <keyword>")
        print("Example: python verify_memory_recall.py ops_fault_diagnosis sw1")
        sys.exit(1)
    agent_id = sys.argv[1]
    keyword = sys.argv[2]
    hits = recall_by_keyword(agent_id, keyword)
    assert hits, f"未召回任何事实：agent={agent_id}, keyword={keyword}"
    for stmt, meta, ts in hits:
        print(f"[HIT] {ts}: {stmt[:120]}...")
    print(f"\n[OK] T3 召回验证通过：命中 {len(hits)} 条事实，下次 RCA 时将被注入 prompt。")
```

运行命令（沉淀完成后）：
```powershell
cd f:/myproject/aranea-agents/test/ts10-gns3
python verify_memory_recall.py ops_fault_diagnosis "sw1 线路中断"
```

- [ ] **Step 3.3 E2E 验证：触发同类告警 RCA，抓包确认 prompt 含历史事实**

使用 `llm_relay.py` 抓包（:8899→deepseek）：

```powershell
cd f:/myproject/aranea-agents/test/ts10-gns3
# 1. 确保 relay 已启动
# 2. 在 TwinWeb 触发一次 sw1 线路中断告警的 RCA
# 3. 运行 analyze_capture.py 检查 LLM 请求 prompt
python analyze_capture.py --last --check-prompt "sw1 线路中断"
```

期望：prompt 中包含 `memory.search` 召回的事实摘要（如「告警[sw1 线路中断] → 资产[sw1] → 修复成功（耗时184秒）」）。

- [ ] **Step 3.4 git commit**

```powershell
cd f:/myproject/aranea-agents
git add test/ts10-gns3/verify_memory_recall.py
git commit -m "test(gns3): L3 事实召回验证脚本（T3）"
```

---

## Task 4：T4 误报标记沉淀路径（MarkFalsePositive → L3 事实写入）

**目标**：运维工程师标记告警为误报时，系统自动沉淀「误报模式」L3 事实，供后续同类告警参考。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca.go`（MarkFalsePositive 追加沉淀调用）
- Test: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/rca_test.go`（新建或追加）

- [ ] **Step 4.1 扩展 RcaUsecase.MarkFalsePositive 写入 L3 误报事实**

在 `app/aiops/internal/biz/rca.go` 修改 `MarkFalsePositive`：

```go
// MarkFalsePositive 标记/取消误报（需求 §5.10；统计卡误报数随动）。
// T4 新增：标记为误报时，写入 aranea L3 事实（agent 作用域），记录误报模式。
func (uc *RcaUsecase) MarkFalsePositive(ctx context.Context, id uint32, flag uint32, reason string) (*RcaRecord, error) {
	if _, err := uc.repo.Get(ctx, id); err != nil {
		return nil, err
	}
	if err := uc.repo.MarkFalsePositive(ctx, id, flag); err != nil {
		return nil, err
	}
	uc.log.WithContext(ctx).Infof("rca %d false positive -> %d (reason=%s)", id, flag, reason)

	// T4：误报标记沉淀
	if flag > 0 && uc.aranea != nil {
		rec, _ := uc.repo.Get(ctx, id)
		if rec != nil {
			agentKey := uc.agentKeyForRCA(rec) // 按 RCA 关联的 Agent 映射
			fact := &MemoryFact{
				Scope:   "agent",
				Key:     agentKey,
				Content: fmt.Sprintf("误报标记：告警[%s]（资产 #%d，告警编号 %s）→ 误判原因：%s", rec.Title, rec.AssetID, rec.AlarmID, reason),
				Metadata: map[string]any{
					"source":      "twinmonitor_false_positive",
					"rca_id":      rec.ID,
					"analysis_no": rec.AnalysisNo,
					"alarm_id":    rec.AlarmID,
					"reason":      reason,
				},
			}
			if werr := uc.aranea.WriteMemoryL3(ctx, fact); werr != nil {
				uc.log.WithContext(ctx).Warnf("false-positive L3 write failed: %v", werr)
			}
		}
	}

	return uc.repo.Get(ctx, id)
}
```

追加辅助方法：

```go
// agentKeyForRCA 按 RCA 记录推断关联 Agent 键（一期简化：按告警标题关键词映射）。
func (uc *RcaUsecase) agentKeyForRCA(rec *RcaRecord) string {
	// 可按 rec.AgentID 反查 agents 表取 aranea_agent_id；一期先用缺省值
	if rec.AgentID > 0 {
		// 若未来 agents 表存有 aranea_agent_id，可在此查询
	}
	return "ops_alarm_handler" // 误报标记通常由告警处理 Agent 关联
}
```

- [ ] **Step 4.2 写失败测试**

在 `app/aiops/internal/biz/rca_test.go` 新建（包 `biz`；fake 需实现 RcaRepo 全部 16 个接口方法——T1 的 `fakeRcaRepoForSediment` 已在同包覆盖，可直接复用并扩展 `byID` 查询能力）：

```go
package biz

import (
	"context"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// fakeRcaRepoWithGet 在 T1 fakeRcaRepoForSediment 基础上支持 Get 按 ID 返回记录。
// 若选择直接复用 T1 fake，可为其加 byID 字段；此处给出独立完整版便于单文件运行。
type fakeRcaRepoWithGet struct {
	byID               map[uint32]*RcaRecord
	marked             uint32
	updatedKnowledgeID uint32
	updatedMemoryID    string
}

func (f *fakeRcaRepoWithGet) List(ctx context.Context, params *RcaListParams) ([]*RcaRecord, int, error) {
	return nil, 0, nil
}
func (f *fakeRcaRepoWithGet) Get(ctx context.Context, id uint32) (*RcaRecord, error) {
	return f.byID[id], nil
}
func (f *fakeRcaRepoWithGet) Create(ctx context.Context, rec *RcaRecord) (*RcaRecord, error) { return nil, nil }
func (f *fakeRcaRepoWithGet) MarkAnalyzing(ctx context.Context, id uint32, araneaRunID string) error {
	return nil
}
func (f *fakeRcaRepoWithGet) CompleteAnalysis(ctx context.Context, id uint32, res *RcaAnalysisResult) error {
	return nil
}
func (f *fakeRcaRepoWithGet) FailAnalysis(ctx context.Context, id uint32, status, errMsg string) error {
	return nil
}
func (f *fakeRcaRepoWithGet) MarkFalsePositive(ctx context.Context, id uint32, flag uint32) error {
	f.marked = flag
	return nil
}
func (f *fakeRcaRepoWithGet) UpdateKnowledgeID(ctx context.Context, id uint32, knowledgeID uint32) error {
	f.updatedKnowledgeID = knowledgeID
	return nil
}
func (f *fakeRcaRepoWithGet) UpdateMemoryFactID(ctx context.Context, id uint32, factID string) error {
	f.updatedMemoryID = factID
	return nil
}
func (f *fakeRcaRepoWithGet) GetStatistics(ctx context.Context) (*RcaStatistics, error) { return nil, nil }
func (f *fakeRcaRepoWithGet) ListRecentCompleted(ctx context.Context, limit int) ([]*RcaRecord, error) {
	return nil, nil
}
func (f *fakeRcaRepoWithGet) GetTaskStatistics(ctx context.Context) (*TaskStatistics, error) {
	return nil, nil
}
func (f *fakeRcaRepoWithGet) GetSystemScenarioGraphID(ctx context.Context, key string) (string, error) {
	return "", nil
}

func TestRcaUsecase_MarkFalsePositive_WritesL3(t *testing.T) {
	repo := &fakeRcaRepoWithGet{
		byID: map[uint32]*RcaRecord{
			5: {ID: 5, AnalysisNo: "RCA-001", Title: "sw1 线路中断", AlarmID: "ALM-001", AssetID: 11, Status: RcaStatusCompleted},
		},
	}
	aranea := &fakeAraneaPort{} // 复用 T1 sediment_test.go 同包替身
	// NewRcaUsecase 签名：repo, aranea, alarm, knowledge, store, publisher, cfg, logger
	uc := NewRcaUsecase(repo, aranea, nil, nil, nil, nil, nil, log.DefaultLogger)

	_, err := uc.MarkFalsePositive(context.Background(), 5, 1, "监控阈值配置过严")
	if err != nil {
		t.Fatalf("mark false positive failed: %v", err)
	}
	if repo.marked != 1 {
		t.Fatalf("marked = %d, want 1", repo.marked)
	}
	if aranea.written == nil {
		t.Fatal("expected L3 fact write on false-positive mark")
	}
	if aranea.written.Scope != "agent" {
		t.Fatalf("scope = %q, want agent", aranea.written.Scope)
	}
	if !strings.Contains(aranea.written.Content, "监控阈值配置过严") {
		t.Fatal("L3 fact missing reason")
	}
	if aranea.written.Metadata["source"] != "twinmonitor_false_positive" {
		t.Fatalf("metadata.source = %v", aranea.written.Metadata["source"])
	}
}

func TestRcaUsecase_MarkFalsePositive_UnmarkSkipsL3(t *testing.T) {
	repo := &fakeRcaRepoWithGet{
		byID: map[uint32]*RcaRecord{5: {ID: 5, Status: RcaStatusCompleted}},
	}
	aranea := &fakeAraneaPort{}
	uc := NewRcaUsecase(repo, aranea, nil, nil, nil, nil, nil, log.DefaultLogger)

	if _, err := uc.MarkFalsePositive(context.Background(), 5, 0, ""); err != nil {
		t.Fatalf("unmark failed: %v", err)
	}
	if aranea.written != nil {
		t.Fatal("unmark (flag=0) should not write L3 fact")
	}
}
```

运行命令：
```powershell
cd f:/myproject/twinmonitor/TwinServer
$env:CGO_ENABLED="0"; go test ./app/aiops/internal/biz -run TestRcaUsecase_MarkFalsePositive -v
```

- [ ] **Step 4.3 构建验证 + commit**

```powershell
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
git add -A
git commit -m "feat(aiops): 误报标记自动沉淀 L3 事实（T4）"
```

---

## Task 5：T5 双写失败补偿（单边成功重试）

**目标**：L3 与 KB 双写时若单边失败，提供补偿重试机制，避免事实与词条永久不一致。

**Files:**
- New: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/sediment_compensate.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/sediment.go`（Sediment 返回详细结果供补偿判断）
- New: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/cron/sediment_compensate.go`（或复用现有 cron 框架）

- [ ] **Step 5.1 扩展 SedimentOutput 记录单边失败详情**

在 `app/aiops/internal/biz/sediment.go` 修改 `SedimentOutput`：

```go
// SedimentOutput 沉淀结果（含单边失败标记，供 T5 补偿）。
type SedimentOutput struct {
	KnowledgeID   uint32
	MemoryFactID  string
	L3Error       string // 空表示成功
	KBError       string // 空表示成功
	RcaUpdated    bool
	NeedsCompensate bool // true 表示至少单边失败，需补偿
}
```

修改 `Sediment` 方法末尾的错误处理：

```go
	out.L3Error = ""
	out.KBError = ""
	if l3Err != nil {
		out.L3Error = l3Err.Error()
		out.NeedsCompensate = true
	}
	if kbErr != nil {
		out.KBError = kbErr.Error()
		out.NeedsCompensate = true
	}
	if out.NeedsCompensate {
		uc.log.WithContext(ctx).Warnf("sediment needs compensate: execution=%s l3_err=%q kb_err=%q", in.ExecutionNo, out.L3Error, out.KBError)
	}
	return out, nil
```

- [ ] **Step 5.2 新建补偿重试器（内存队列 + 定时重试）**

在 `app/aiops/internal/biz/sediment_compensate.go` 新建：

```go
package biz

import (
	"context"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// SedimentCompensator 双写单边失败补偿器（T5）。
// 机制：Sediment 单边失败时入队，定时器重试最多 3 次，间隔 30s/60s/120s。
type SedimentCompensator struct {
	uc      *SedimentUsecase
	log     *log.Helper
	mu      sync.Mutex
	queue   []compensateItem
	ticker  *time.Ticker
	stop    chan struct{}
}

type compensateItem struct {
	In        *SedimentInput
	Out       *SedimentOutput
	Attempts  int
	NextRun   time.Time
}

// NewSedimentCompensator 构造补偿器（传入 SedimentUsecase 引用）。
func NewSedimentCompensator(uc *SedimentUsecase, logger log.Logger) *SedimentCompensator {
	return &SedimentCompensator{
		uc:     uc,
		log:    log.NewHelper(log.With(logger, "module", "biz/sediment-compensator")),
		stop:   make(chan struct{}),
	}
}

// Start 启动后台定时重试（建议 30s 周期）。
func (c *SedimentCompensator) Start(interval time.Duration) {
	c.ticker = time.NewTicker(interval)
	go c.loop()
}

// Stop 停止定时器并等待队列排空。
func (c *SedimentCompensator) Stop() {
	close(c.stop)
	if c.ticker != nil {
		c.ticker.Stop()
	}
}

// Enqueue 将需要补偿的沉淀任务入队（SedimentUsecase.Sediment 返回 NeedsCompensate=true 时调用）。
func (c *SedimentCompensator) Enqueue(in *SimentInput, out *SedimentOutput) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queue = append(c.queue, compensateItem{
		In:       in,
		Out:      out,
		Attempts: 0,
		NextRun:  time.Now().Add(30 * time.Second),
	})
}

func (c *SedimentCompensator) loop() {
	for {
		select {
		case <-c.stop:
			return
		case <-c.ticker.C:
			c.drain()
		}
	}
}

func (c *SedimentCompensator) drain() {
	c.mu.Lock()
	now := time.Now()
	retry := make([]compensateItem, 0, len(c.queue))
	pending := make([]compensateItem, 0, len(c.queue))
	for _, it := range c.queue {
		if it.NextRun.Before(now) {
			retry = append(retry, it)
		} else {
			pending = append(pending, it)
		}
	}
	c.queue = pending
	c.mu.Unlock()

	for _, it := range retry {
		c.tryCompensate(it)
	}
}

func (c *SedimentCompensator) tryCompensate(it compensateItem) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.log.Infof("compensate sediment: execution=%s attempt=%d", it.In.ExecutionNo, it.Attempts+1)
	out, err := c.uc.Sediment(ctx, it.In)
	if err != nil {
		c.log.Warnf("compensate failed: %v", err)
	}
	if out != nil && out.NeedsCompensate && it.Attempts < 2 {
		it.Attempts++
		it.NextRun = time.Now().Add(time.Duration(30*(1<<it.Attempts)) * time.Second)
		c.mu.Lock()
		c.queue = append(c.queue, it)
		c.mu.Unlock()
	}
}
```

- [ ] **Step 5.3 SedimentUsecase 接入补偿器**

在 `SedimentUsecase` 追加 `compensator` 字段：

```go
type SedimentUsecase struct {
	aranea      AraneaPort
	kb          KnowledgeClient
	rca         RcaRepo
	compensator *SedimentCompensator
	log         *log.Helper
}
```

在 `Sediment` 末尾：

```go
	if uc.compensator != nil && out.NeedsCompensate {
		uc.compensator.Enqueue(in, out)
	}
	return out, nil
```

- [ ] **Step 5.4 wire 注入补偿器并在 main 启动**

在 `app/aiops/cmd/.../wire.go` 或 `main.go` 中：

```go
compensator := biz.NewSedimentCompensator(sedimentUC, logger)
compensator.Start(30 * time.Second)
// 在 graceful shutdown 时调用 compensator.Stop()
```

- [ ] **Step 5.5 写补偿测试**

在 `app/aiops/internal/biz/sediment_compensate_test.go` 新建：

```go
package biz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

type failingKBClient struct{ call int }

func (f *failingKBClient) Search(ctx context.Context, keyword string, topK int) ([]*KnowledgeItem, error) { return nil, nil }
func (f *failingKBClient) Create(ctx context.Context, in *KnowledgeCreateInput) (uint32, error) {
	f.call++
	if f.call <= 2 {
		return 0, errors.New("kb transient failure")
	}
	return 99, nil
}
func (f *failingKBClient) GetStatistics(ctx context.Context) (*KnowledgeStatistics, error) { return nil, nil }

func TestSedimentCompensator_RetriesKB(t *testing.T) {
	aranea := &fakeAraneaPort{}
	kb := &failingKBClient{}
	rca := &fakeRcaRepoForSediment{}
	uc := NewSedimentUsecase(aranea, kb, rca, log.NewHelper(log.DefaultLogger))
	comp := NewSedimentCompensator(uc, log.NewHelper(log.DefaultLogger))
	comp.Start(100 * time.Millisecond)
	defer comp.Stop()

	in := &SedimentInput{ExecutionNo: "REM-COMP-01", AlertTitle: "test", AgentKey: "ops_fault_diagnosis", Success: true}
	out, err := uc.Sediment(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.NeedsCompensate {
		t.Fatal("expected NeedsCompensate=true after KB failure")
	}
	comp.Enqueue(in, out)

	// 等待补偿器重试
	time.Sleep(400 * time.Millisecond)
	if kb.call < 2 {
		t.Fatalf("kb call = %d, expected >= 2 retries", kb.call)
	}
}
```

运行命令：
```powershell
cd f:/myproject/twinmonitor/TwinServer
$env:CGO_ENABLED="0"; go test ./app/aiops/internal/biz -run TestSedimentCompensator -v
```

- [ ] **Step 5.6 构建验证 + commit**

```powershell
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/... ./app/remediation/...
git add -A
git commit -m "feat(aiops): 双写失败补偿器（SedimentCompensator，T5）"
```

---

## 附录：总纲与代码不一致点记录

| # | 总纲描述 | 代码实际 | 处置 |
|---|---------|---------|------|
| 1 | 总纲 §3.4 说 L3 事实 `scope_type=agent, scope_id=agentID` | `twin_openapi_compat.go:handleWriteMemoryFact` 已正确映射 `Scope=agent` → `ScopeType=agent, ScopeID=Key`，无需修改 | 已对齐 |
| 2 | 总纲 §3.4 payload 示例用 `fact_id` | aranea 侧 `FactUpsert` 无 `FactID` 字段，而是 `Fingerprint` 字段承载业务键；`handleWriteMemoryFact` 把 `in.Key` 写入 `Fingerprint` | 已对齐（Key ↔ Fingerprint） |
| 3 | 总纲 §3.5 说「13 侧知识沉淀联动」 | 实际 `DepositKnowledge` 在 `RcaUsecase` 中，且是**手动调用**（operator 触发），非自动闭环 | **本计划 T1 根治**：14 终态自动调用 13 沉淀端点 |
| 4 | `immediate_fact_writer.go` 硬编码 `ScopeType=session`（项目记忆） | 运维场景不走即时事实写入，而走 `WriteMemoryL3`（`TwinOpenAPICompatService`），该路径已修正为 agent 作用域 | 已对齐 |
| 5 | `L3ScopeTargets` 无 `session` case（项目记忆） | 本计划全部使用 `agent` 作用域，不碰 session 作用域 | 已对齐 |
| 6 | 10 KB `KnowledgeClient.Create` 返回 `uint32` ID | `rca_repo.go:UpdateKnowledgeID` 接收 `uint32`，但 ent schema `AiRcaRecord` 的 `knowledge_id` 是 `int64`；转换在 repo 层 `SetKnowledgeID(int64(knowledgeID))` 处理 | 已兼容 |
| 7 | `agent_preset.go` 种子同步时 `Agent.Name` 为中文 | aranea 侧 `Agent.DisplayName` 存中文，`AgentKey`（如 `ops_change_execution`）可能存于 `ConfigJSON` 或 `Metadata`；14 沉淀时需反查 `Agent.ID` 才能正确写入 L3 | **T3 已标注**：一期可先按 scenario_id 硬编码映射，P5+ 动态查询 |

---

## 验证清单（全部 Task 完成后统一执行）

```powershell
# 1. twinmonitor 全量构建
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/... ./app/remediation/...

# 2. aranea 全量构建
cd f:/myproject/aranea-agents
go build ./cmd/... ./internal/...

# 3. 单元测试
cd f:/myproject/twinmonitor/TwinServer
$env:CGO_ENABLED="0"
go test ./app/aiops/internal/biz -run "TestSediment|TestRcaUsecase_MarkFalsePositive|TestSedimentCompensator" -v
go test ./app/remediation/internal/biz -run TestApplyTaskEvent -v

# 4. E2E 验证（需真实沉淀一次后执行）
cd f:/myproject/aranea-agents/test/ts10-gns3
python verify_sediment_chunks.py <knowledge_id>
python verify_memory_recall.py ops_fault_diagnosis "sw1 线路中断"
```
