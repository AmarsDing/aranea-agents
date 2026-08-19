# P2 双跑切换：预设 Agent 全面切 MCP + 策略预授权 + 双层审批/守卫 E2E 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依据总纲 §3.1.2 P2 与 §3.2（双层审批 + 策略-工具风险联动 R2），完成：① 12 个预设运维 Agent 的 `tool_whitelist` 全部指向真实存在的 MCP 工具键（修正当前白名单中 11 个注册表不存在的幽灵键）；② remediate 图（incident-response）节点指令内的工具键由内置 twinops 名（`gns3_exec`/`gns3_fault_clear`/`twin_alarm_get`/`twin_line_events`/`gns3_health_check`）切换为 MCP 名（`gns3.exec`/`gns3.fault_clear`/`alarm.get`/`network.line_events`/`gns3.health_check`），budget/守卫语义不变；③ 14 新增策略预授权数据模型（`remediation_policy_grants` 表 + TTL），`execution_mode=auto` 且场景含 destructive 工具的策略未预授权禁止启用；④ 预授权命中时 14 消费 `ai.task.events(waiting_approval)` 经 13 系统级 Resume 直通，不产生人工审批待办；⑤ E2E 验证守卫/budget 在 MCP 通道不变形、L3 记忆（scope=agent）可写可查。

**Architecture:** 双层审批（总纲 §3.2）：第一层 14 策略级审批（`remediation_executions.status=pending_approval`），第二层 aranea 工具级 interrupt（`requires_confirmation=true` → webhook `run.waiting_approval` → 13 `ai_approvals` → 人工 Approve → `ResumeInterrupt`）。预授权是第二层的一次性让渡：14 侧 `remediation_policy_grants`（主库 ent，新表）记录「策略×场景×工具组合」授权（grant_policy=always + expire_at TTL）；14 `TaskEventConsumer` 在 `waiting_approval` 动作分支查授权，命中则调 13 新端点 `POST /api/v1/monitor/aiops/tasks/resume-interrupt`（内部完成 ① aranea `ResumeInterrupt(approved=true)` ② 该 interrupt 的 pending 审批系统置 approved），审计留痕 `source=preauth`。策略启用校验挂 `PolicyUsecase` 的 Create/Update/Toggle 三入口（`validatePolicyPayload` 是纯函数无依赖，校验逻辑放 usecase 层），场景工具风险经既有 `AiopsGetScenarioRisk` 外部端口扩展（13 场景详情返回 `max_tool_risk`）。12 预设 Agent 种子在 13 `agent_preset.go`，种子同步器 `SeedSynchronizer` 自带 `tool_whitelist` 漂移比对（`seedAgentDrifted`），改种子后重跑 seed-sync 即幂等推送 aranea。

**Tech Stack:** Go + Kratos（twinmonitor 13-aiops / 14-remediation）+ Ent（主库/日志库 schema 代码生成）+ NATS JetStream（ai.task.events）+ aranea-agents（PG `agents`/`agent_runtime_settings`/`tool_grants` 表 + test/ts10-gns3 E2E 环境）。

**前置依赖：** P0（aranea 在环最小闭环已通）+ P1（10 个新增 MCP 工具已注册、aranea `mcp_servers` 已登记 twinmonitor SSE、`ai_mcp_call_history.plane` 已上线）。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - twinmonitor: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/aiops/... ./app/remediation/...`
  - aranea: `cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`
- **ent 代码生成**（schema 改动后必跑，项目记忆「全量构建前先重生成」）：
  - 14 主库: `cd app/remediation/internal/data/ent && go run -mod=mod ./entc.go`
- **wire 重新生成**（新增 Provider 形参后必跑）：`go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/` 或 `./app/remediation/cmd/`
- **SQL 安全规约**：写操作先同条件 `SELECT COUNT(*)` 确认命中范围，显式事务包裹，执行后核验 affected rows；禁止 PowerShell 内联复杂引号串执行 SQL（用 sql 文件或 psql `-f`）。
- **commit 风格**：twinmonitor 仓库 `feat(aiops): ...` / `feat(remediation): ...`；aranea 仓库 `test(gns3): ...`（参照 `git log --oneline` 既有前缀惯例）。

---

## Task 1：T1 13 场景详情返回 `max_tool_risk`（策略-工具风险联动的判定源）

**目标**：13 `GET /api/v1/monitor/aiops/scenarios/{id}` 的响应补充 `max_tool_risk` 字段——由场景 `GraphSnapshot`（图定义 JSON）中 Agent 节点引用的工具键，对照 `ai_mcp_tools.risk_level` 取最大值。14 侧策略校验与 14 执行风险分级共用此判定源（总纲 §3.2 R2 联动表「场景含工具最高风险」列）。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/scenario.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/service/scenario.go`（DTO 透出，如字段已在 proto 响应则仅填充）
- Test: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/scenario_test.go`

- [ ] **Step 1.1 先写失败测试：GraphSnapshot 含 destructive 工具时 max_tool_risk=destructive**

在 `scenario_test.go` 追加（若无此文件则新建，包 `biz`）：

```go
func TestScenarioMaxToolRisk(t *testing.T) {
	snap := map[string]any{
		"nodes": []any{
			map[string]any{"id": "remediate", "type": "agent",
				"instruction": "调用 gns3.exec 取证后调 gns3.fault_clear 恢复端口"},
			map[string]any{"id": "diagnose", "type": "agent",
				"instruction": "调 alarm.get 与 network.line_events 取证"},
		},
	}
	risk := ScenarioMaxToolRisk(snap, map[string]string{
		"gns3.exec":           "high",
		"gns3.fault_clear":    "destructive",
		"alarm.get":           "readonly",
		"network.line_events": "readonly",
	})
	if risk != "destructive" {
		t.Fatalf("expect destructive, got %s", risk)
	}
	// 无快照/无命中 → high 以下默认返回空串（调用方按场景自身 risk_level 兜底）
	if r := ScenarioMaxToolRisk(nil, nil); r != "" {
		t.Fatalf("expect empty, got %s", r)
	}
}
```

运行确认失败：

```bash
cd f:/myproject/twinmonitor/TwinServer
go test ./app/aiops/internal/biz/ -run TestScenarioMaxToolRisk -v -count=1
# 预期：编译失败（ScenarioMaxToolRisk 未定义）
```

- [ ] **Step 1.2 实现 `ScenarioMaxToolRisk` 与工具风险目录加载**

在 `scenario.go` 末尾追加：

```go
// mcpToolRiskOrder 风险等级排序（取最大值用；与 ai_mcp_tools.risk_level 五级一致）。
var mcpToolRiskOrder = map[string]int{
	"readonly": 1, "low": 2, "medium": 3, "high": 4, "destructive": 5,
}

// ScenarioMaxToolRisk 扫描场景 GraphSnapshot 全部节点 instruction 中提及的工具键，
// 对照 toolRisks（工具键 → risk_level，源自 ai_mcp_tools）返回最高风险等级；
// 无快照或无命中返回空串（调用方按场景自身 risk_level 兜底）。
// 判定为「指令文本提及」是刻意的：图 Agent 的实际可用工具由其 aranea 侧
// tool_whitelist 决定，而 13 不持有该映射，指令提及是最保守上界（宁严勿宽）。
func ScenarioMaxToolRisk(snapshot map[string]any, toolRisks map[string]string) string {
	if len(snapshot) == 0 || len(toolRisks) == 0 {
		return ""
	}
	nodes, _ := snapshot["nodes"].([]any)
	best := ""
	bestOrd := 0
	for _, n := range nodes {
		node, _ := n.(map[string]any)
		instr, _ := node["instruction"].(string)
		if instr == "" {
			continue
		}
		for key, risk := range toolRisks {
			if !strings.Contains(instr, key) {
				continue
			}
			if mcpToolRiskOrder[risk] > bestOrd {
				bestOrd = mcpToolRiskOrder[risk]
				best = risk
			}
		}
	}
	return best
}
```

文件头部 import 补 `"strings"`。

- [ ] **Step 1.3 场景详情响应填充 `max_tool_risk`**

在 `ScenarioUsecase` 增加 MCP 工具风险目录依赖（biz 层只依赖端口）：

```go
// ScenarioUsecase 结构体追加字段：
	mcpTools McpToolCatalog // 可 nil（未装配时 max_tool_risk 返回空串）
```

`clients.go` 追加端口：

```go
// McpToolCatalog MCP 工具风险目录（data 层直查 ai_mcp_tools；只读）。
type McpToolCatalog interface {
	// ListToolRisks 返回 工具键 → risk_level 全量映射。
	ListToolRisks(ctx context.Context) (map[string]string, error)
}
```

`NewScenarioUsecase` 追加形参 `mcpTools McpToolCatalog` 并赋值。在 `Get` 方法返回前填充：

```go
// Get 查询场景详情（响应补 max_tool_risk，供 14 策略-工具风险联动判定）。
func (uc *ScenarioUsecase) Get(ctx context.Context, id uint32) (*Scenario, error) {
	s, err := uc.repo.Get(ctx, id)
	if err != nil || s == nil {
		return s, err
	}
	if uc.mcpTools != nil && s.GraphSnapshot != nil {
		if risks, rerr := uc.mcpTools.ListToolRisks(ctx); rerr == nil {
			s.MaxToolRisk = ScenarioMaxToolRisk(s.GraphSnapshot, risks)
		}
	}
	return s, nil
}
```

`Scenario` 结构体追加（非落库字段，仅响应透出）：

```go
	MaxToolRisk     string           // 非落库：图节点指令引用工具的最高 MCP 风险等级
```

- [ ] **Step 1.4 data 层实现 `McpToolCatalog` + wire 装配**

在 `app/aiops/internal/data/` 找到既有 MCP 工具仓储（`grep -n "ai_mcp_tools\|McpTool" app/aiops/internal/data/*.go | head` 定位，通常为 `mcp_repo.go`），为其追加方法实现：

```go
// ListToolRisks 实现 biz.McpToolCatalog：全量工具键 → risk_level。
func (r *mcpRepo) ListToolRisks(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT name, risk_level FROM ai_mcp_tools WHERE delete_time IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, risk string
		if err := rows.Scan(&name, &risk); err != nil {
			return nil, err
		}
		out[name] = risk
	}
	return out, rows.Err()
}
```

（若该仓储走 ent 而非裸 SQL，按既有风格改写为 ent 查询。）在 `data.ProviderSet` 补 `wire.Bind(new(biz.McpToolCatalog), new(*mcpRepo))`（参照既有 bind 模式），随后：

```bash
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/
go test ./app/aiops/internal/biz/ -run TestScenarioMaxToolRisk -v -count=1
# 预期：PASS
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 1.5 运行时冒烟（13 重启后）**

```bash
curl -s http://localhost:8100/api/v1/monitor/aiops/scenarios/<incident-response场景ID> \
  -H "Authorization: Bearer $TWIN_ADMIN_TOKEN" | jq '.max_tool_risk'
# 预期（T5 图指令切换 MCP 工具名后）："destructive"
# 切换前（指令仍为 gns3_exec 内置名，ai_mcp_tools 无此键）：""
```

- [ ] **Step 1.6 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/scenario.go app/aiops/internal/biz/clients.go app/aiops/internal/biz/scenario_test.go app/aiops/internal/data/ app/aiops/cmd/wire_gen.go
git commit -m "$(cat <<'EOF'
feat(aiops): 场景详情返回 max_tool_risk（策略-工具风险联动判定源）

- ScenarioMaxToolRisk 扫描 GraphSnapshot 节点指令提及的 MCP 工具键取最高风险
- McpToolCatalog 端口直查 ai_mcp_tools，14 策略校验与风险分级共用
- 总纲 §3.2 R2 联动表「场景含工具最高风险」列的判定实现

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.2
EOF
)"
```

---

## Task 2：T2 预授权数据模型（14 `remediation_policy_grants` 表 + TTL）

**目标**：14 主库新增 `remediation_policy_grants` 表（ent schema + 自动迁移 + init SQL 补齐），仓储与用例支持：授予（幂等 upsert）、撤销、按（policy_id, scenario_id）查生效授权（`expire_at > now` 或 NULL 永不过期）、过期惰性失效。语义对齐总纲 §3.2：「记录 grant_policy=always + approval_ttl」「授权过期自动回落为逐次确认」。

**Files:**
- Create: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/ent/schema/remediation_policy_grant.go`
- Generate: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/ent/`（ent 生成物）
- Create: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/policy_grant.go`
- Create: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/policy_grant_repo.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/docker/init-remediation.sql`

- [ ] **Step 2.1 ent schema 新建 `RemediationPolicyGrant`**

新建 `remediation_policy_grant.go`（与 `remediation_policy.go` 同目录同风格）：

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"
)

// RemediationPolicyGrant 策略预授权表（总纲 §3.2 R2：auto+destructive 联动规则）。
// 「一次审批、N 次执行」的让渡凭证：授权有效期内，该策略触发场景的 destructive
// 工具 interrupt 由系统级 Resume 直通，不再产生人工审批待办；过期自动回落逐次确认。
type RemediationPolicyGrant struct {
	ent.Schema
}

func (RemediationPolicyGrant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "remediation_policy_grants",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("策略预授权表：auto 策略关联 destructive 场景的「场景×工具组合」显式授权"),
	}
}

func (RemediationPolicyGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Uint32("policy_id").
			Comment("修复策略 ID（remediation_policies.id）"),
		field.Int64("scenario_id").
			Comment("关联修复场景模板ID（13 ai_scenario_templates.id）"),
		field.JSON("tool_keys", []string{}).
			Comment("授权覆盖的 destructive 工具键清单（如 [\"gns3.fault_clear\"]）"),
		field.String("grant_policy").
			Comment("授权策略：always-有效期内始终放行（一期仅此值）").
			NotEmpty().
			MaxLen(20).
			Default("always"),
		field.Time("expire_at").
			Comment("授权到期时间（approval_ttl）；NULL 表示永不过期").
			Optional().
			Nillable(),
		field.String("granted_by").
			Comment("授权人标识（值班审批员姓名/工号）").
			NotEmpty().
			MaxLen(64),
		field.Uint32("granted_by_id").
			Comment("授权人用户 ID").
			Optional(),
		field.Text("reason").
			Comment("授权事由（审计留痕）").
			Optional().
			Nillable(),
		field.Int16("status").
			Comment("状态：1-生效, 0-已撤销").
			Default(1),
	}
}

func (RemediationPolicyGrant) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.Time{},
	}
}

func (RemediationPolicyGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("policy_id", "scenario_id").
			StorageKey("idx_remediation_policy_grants_policy_scenario"),
		index.Fields("status", "expire_at").
			StorageKey("idx_remediation_policy_grants_status_expire"),
	}
}

func (RemediationPolicyGrant) Edges() []ent.Edge { return nil }

// Privacy 数据权限策略（与 RemediationPolicy 一致：平台级治理面放行）。
func (RemediationPolicyGrant) Privacy() privacy.Policy {
	return privacy.Policy{
		Query:    privacy.QueryPolicy{privacy.AlwaysAllowRule()},
		Mutation: privacy.MutationPolicy{privacy.AlwaysAllowRule()},
	}
}
```

- [ ] **Step 2.2 ent 代码生成 + init SQL 补齐**

```bash
cd f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/ent
go run -mod=mod ./entc.go
# 预期：无错误，ent/remediationpolicygrant/ 生成
```

`docker/init-remediation.sql` 的 `remediation_policies` 建表段之后追加（与既有 DDL 同风格，幂等）：

```sql
CREATE TABLE IF NOT EXISTS remediation_policy_grants (
    id              BIGSERIAL PRIMARY KEY,
    policy_id       BIGINT      NOT NULL,
    scenario_id     BIGINT      NOT NULL,
    tool_keys       JSONB       NOT NULL DEFAULT '[]',
    grant_policy    VARCHAR(20) NOT NULL DEFAULT 'always',
    expire_at       TIMESTAMPTZ NULL,
    granted_by      VARCHAR(64) NOT NULL,
    granted_by_id   BIGINT      NULL,
    reason          TEXT        NULL,
    status          SMALLINT    NOT NULL DEFAULT 1,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
    update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_remediation_policy_grants_policy_scenario ON remediation_policy_grants (policy_id, scenario_id);
CREATE INDEX IF NOT EXISTS idx_remediation_policy_grants_status_expire ON remediation_policy_grants (status, expire_at);
COMMENT ON TABLE remediation_policy_grants IS '策略预授权表：auto 策略关联 destructive 场景的显式授权（总纲 §3.2 R2）';
```

> 存量环境说明：服务启动时 ent 自动迁移会建表；init SQL 用于全新部署包，两侧字段定义必须一致（端口统一规划「源仓库与 app 部署包配置必须一致」同款约束）。

- [ ] **Step 2.3 biz 层领域模型与用例**

新建 `app/remediation/internal/biz/policy_grant.go`：

```go
package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// PolicyGrant 策略预授权领域模型（remediation_policy_grants，twinmonitor 主库）。
type PolicyGrant struct {
	ID          uint32
	PolicyID    uint32
	ScenarioID  int64
	ToolKeys    []string
	GrantPolicy string // always
	ExpireAt    *time.Time
	GrantedBy   string
	GrantedByID uint32
	Reason      string
	Status      int16
	CreatedAt   *time.Time
}

// PolicyGrantRepo 预授权仓储端口（data.policyGrantRepo 实现，主库）。
type PolicyGrantRepo interface {
	// Upsert 按 (policy_id, scenario_id) 幂等授予：已有生效行则续期/更新工具清单。
	Upsert(ctx context.Context, g *PolicyGrant) (*PolicyGrant, error)
	// GetActive 查 (policy_id, scenario_id) 的生效授权（status=1 且 expire_at 为空或未到期）。
	GetActive(ctx context.Context, policyID uint32, scenarioID int64) (*PolicyGrant, error)
	// Revoke 撤销（status=0），返回是否命中。
	Revoke(ctx context.Context, id uint32) (bool, error)
	// ListByPolicy 策略授权清单（含已撤销/已过期，审计视角）。
	ListByPolicy(ctx context.Context, policyID uint32) ([]*PolicyGrant, error)
}

// PolicyGrantUsecase 预授权用例：授予/撤销/生效判定（总纲 §3.2 R2）。
type PolicyGrantUsecase struct {
	repo PolicyGrantRepo
	log  *log.Helper
}

func NewPolicyGrantUsecase(repo PolicyGrantRepo, logger log.Logger) *PolicyGrantUsecase {
	return &PolicyGrantUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "policygrant/usecase")),
	}
}

// Grant 授予预授权（ttl<=0 表示永不过期）。
func (uc *PolicyGrantUsecase) Grant(ctx context.Context, policyID uint32, scenarioID int64, toolKeys []string, ttl time.Duration, grantedBy string, grantedByID uint32, reason string) (*PolicyGrant, error) {
	g := &PolicyGrant{
		PolicyID:    policyID,
		ScenarioID:  scenarioID,
		ToolKeys:    toolKeys,
		GrantPolicy: "always",
		GrantedBy:   grantedBy,
		GrantedByID: grantedByID,
		Reason:      reason,
		Status:      1,
	}
	if ttl > 0 {
		exp := time.Now().Add(ttl)
		g.ExpireAt = &exp
	}
	saved, err := uc.repo.Upsert(ctx, g)
	if err != nil {
		return nil, err
	}
	uc.log.WithContext(ctx).Infof("policy grant upserted: policy=%d scenario=%d tools=%v expire_at=%v by=%s",
		policyID, scenarioID, toolKeys, g.ExpireAt, grantedBy)
	return saved, nil
}

// HasActiveGrant 生效授权判定（过期自动回落：expire_at 已过即无授权，恢复逐次确认）。
func (uc *PolicyGrantUsecase) HasActiveGrant(ctx context.Context, policyID uint32, scenarioID int64) bool {
	g, err := uc.repo.GetActive(ctx, policyID, scenarioID)
	if err != nil {
		uc.log.WithContext(ctx).Warnf("query policy grant failed (fail-closed): policy=%d scenario=%d err=%v", policyID, scenarioID, err)
		return false
	}
	return g != nil
}
```

- [ ] **Step 2.4 data 层仓储实现**

新建 `app/remediation/internal/data/policy_grant_repo.go`（参照 `policy_repo.go` 既有风格：ent 客户端 + DeleteTimeIsNil 不适用——本表无软删列，直接 status 过滤）：

```go
package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"twinserver/app/remediation/internal/biz"
	"twinserver/app/remediation/internal/data/ent"
	"twinserver/app/remediation/internal/data/ent/remediationpolicygrant"
)

type policyGrantRepo struct {
	db  *ent.Client
	log *log.Helper
}

// NewPolicyGrantRepo 预授权仓储（主库 ent）。
func NewPolicyGrantRepo(db *ent.Client, logger log.Logger) biz.PolicyGrantRepo {
	return &policyGrantRepo{db: db, log: log.NewHelper(log.With(logger, "module", "data/policy-grant-repo"))}
}

func (r *policyGrantRepo) Upsert(ctx context.Context, g *biz.PolicyGrant) (*biz.PolicyGrant, error) {
	// 幂等：同 (policy_id, scenario_id) 已有行则更新工具清单/有效期/授权人并复活 status=1
	exist, err := r.db.RemediationPolicyGrant.Query().
		Where(
			remediationpolicygrant.PolicyID(g.PolicyID),
			remediationpolicygrant.ScenarioID(g.ScenarioID),
		).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if exist == nil {
		builder := r.db.RemediationPolicyGrant.Create().
			SetPolicyID(g.PolicyID).
			SetScenarioID(g.ScenarioID).
			SetToolKeys(g.ToolKeys).
			SetGrantPolicy(g.GrantPolicy).
			SetGrantedBy(g.GrantedBy).
			SetStatus(1)
		if g.GrantedByID > 0 {
			builder.SetGrantedByID(g.GrantedByID)
		}
		if g.ExpireAt != nil {
			builder.SetExpireAt(*g.ExpireAt)
		}
		if g.Reason != "" {
			builder.SetReason(g.Reason)
		}
		row, err := builder.Save(ctx)
		if err != nil {
			return nil, err
		}
		return grantToBiz(row), nil
	}
	builder := r.db.RemediationPolicyGrant.UpdateOneID(exist.ID).
		SetToolKeys(g.ToolKeys).
		SetGrantPolicy(g.GrantPolicy).
		SetGrantedBy(g.GrantedBy).
		SetStatus(1)
	if g.GrantedByID > 0 {
		builder.SetGrantedByID(g.GrantedByID)
	}
	if g.ExpireAt != nil {
		builder.SetExpireAt(*g.ExpireAt)
	} else {
		builder.ClearExpireAt()
	}
	if g.Reason != "" {
		builder.SetReason(g.Reason)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return grantToBiz(row), nil
}

func (r *policyGrantRepo) GetActive(ctx context.Context, policyID uint32, scenarioID int64) (*biz.PolicyGrant, error) {
	now := time.Now()
	row, err := r.db.RemediationPolicyGrant.Query().
		Where(
			remediationpolicygrant.PolicyID(policyID),
			remediationpolicygrant.ScenarioID(scenarioID),
			remediationpolicygrant.Status(1),
			remediationpolicygrant.Or(
				remediationpolicygrant.ExpireAtIsNil(),
				remediationpolicygrant.ExpireAtGT(now),
			),
		).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return grantToBiz(row), nil
}

func (r *policyGrantRepo) Revoke(ctx context.Context, id uint32) (bool, error) {
	n, err := r.db.RemediationPolicyGrant.Update().
		Where(
			remediationpolicygrant.ID(id),
			remediationpolicygrant.Status(1),
		).
		SetStatus(0).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *policyGrantRepo) ListByPolicy(ctx context.Context, policyID uint32) ([]*biz.PolicyGrant, error) {
	rows, err := r.db.RemediationPolicyGrant.Query().
		Where(remediationpolicygrant.PolicyID(policyID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*biz.PolicyGrant, 0, len(rows))
	for _, row := range rows {
		out = append(out, grantToBiz(row))
	}
	return out, nil
}

func grantToBiz(e *ent.RemediationPolicyGrant) *biz.PolicyGrant {
	g := &biz.PolicyGrant{
		ID:         uint32(e.ID),
		PolicyID:   e.PolicyID,
		ScenarioID: e.ScenarioID,
		ToolKeys:   e.ToolKeys,
		Status:     e.Status,
	}
	if e.GrantPolicy != "" {
		g.GrantPolicy = e.GrantPolicy
	}
	if e.ExpireAt != nil {
		g.ExpireAt = e.ExpireAt
	}
	g.GrantedBy = e.GrantedBy
	if e.GrantedByID != nil {
		g.GrantedByID = *e.GrantedByID
	}
	if e.Reason != nil {
		g.Reason = *e.Reason
	}
	if e.CreateTime != nil {
		g.CreatedAt = e.CreateTime
	}
	return g
}
```

在 `data.ProviderSet`（`app/remediation/internal/data/data.go`）追加 `NewPolicyGrantRepo`；在 `biz.ProviderSet` 追加 `NewPolicyGrantUsecase`。wire 重生成 + 编译 + 单测：

```bash
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/remediation/cmd/
go build ./app/remediation/...
# 预期：0 错误
```

- [ ] **Step 2.5 单测：生效/过期/撤销三态**

新建 `app/remediation/internal/biz/policy_grant_test.go`：

```go
package biz

import (
	"context"
	"testing"
	"time"
)

type fakePolicyGrantRepo struct {
	active *PolicyGrant
	err    error
}

func (f *fakePolicyGrantRepo) Upsert(_ context.Context, g *PolicyGrant) (*PolicyGrant, error) {
	return g, nil
}
func (f *fakePolicyGrantRepo) GetActive(_ context.Context, _ uint32, _ int64) (*PolicyGrant, error) {
	return f.active, f.err
}
func (f *fakePolicyGrantRepo) Revoke(_ context.Context, _ uint32) (bool, error) { return true, nil }
func (f *fakePolicyGrantRepo) ListByPolicy(_ context.Context, _ uint32) ([]*PolicyGrant, error) {
	return nil, nil
}

func TestPolicyGrantHasActive(t *testing.T) {
	uc := NewPolicyGrantUsecase(&fakePolicyGrantRepo{active: &PolicyGrant{ID: 1}}, testLogger())
	if !uc.HasActiveGrant(context.Background(), 7, 42) {
		t.Fatal("expect active grant")
	}
	uc2 := NewPolicyGrantUsecase(&fakePolicyGrantRepo{active: nil}, testLogger())
	if uc2.HasActiveGrant(context.Background(), 7, 42) {
		t.Fatal("expect no grant")
	}
	// 仓储错误 fail-closed
	uc3 := NewPolicyGrantUsecase(&fakePolicyGrantRepo{err: context.DeadlineExceeded}, testLogger())
	if uc3.HasActiveGrant(context.Background(), 7, 42) {
		t.Fatal("expect fail-closed false")
	}
}

func TestPolicyGrantGrantTTL(t *testing.T) {
	repo := &fakePolicyGrantRepo{}
	uc := NewPolicyGrantUsecase(repo, testLogger())
	g, err := uc.Grant(context.Background(), 7, 42, []string{"gns3.fault_clear"}, 24*time.Hour, "值班长A", 1001, "演练窗口授权")
	if err != nil {
		t.Fatal(err)
	}
	if g.ExpireAt == nil || time.Until(*g.ExpireAt) < 23*time.Hour {
		t.Fatalf("expect ~24h ttl, got %v", g.ExpireAt)
	}
	g2, _ := uc.Grant(context.Background(), 7, 42, []string{"gns3.fault_clear"}, 0, "值班长A", 1001, "长期授权")
	if g2.ExpireAt != nil {
		t.Fatalf("expect no expiry, got %v", g2.ExpireAt)
	}
}
```

（`testLogger()` 参照 `execution_test.go` 既有测试辅助；如无名称不同，以其为准。）

```bash
cd f:/myproject/twinmonitor/TwinServer
go test ./app/remediation/internal/biz/ -run TestPolicyGrant -v -count=1
# 预期：PASS
```

- [ ] **Step 2.6 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/remediation/internal/data/ent/schema/remediation_policy_grant.go app/remediation/internal/data/ent/ app/remediation/internal/biz/policy_grant.go app/remediation/internal/biz/policy_grant_test.go app/remediation/internal/data/policy_grant_repo.go app/remediation/internal/data/data.go app/remediation/internal/biz/biz.go app/remediation/cmd/wire_gen.go docker/init-remediation.sql
git commit -m "$(cat <<'EOF'
feat(remediation): 策略预授权数据模型 remediation_policy_grants（总纲 §3.2 R2）

- ent schema + init SQL：policy×scenario×tool_keys + grant_policy=always + expire_at TTL
- PolicyGrantUsecase：授予幂等 upsert / 撤销 / 生效判定（过期自动回落，仓储错误 fail-closed）
- 联动规则：auto+destructive 策略须持生效授权方可启用（T3 接线启用校验）

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.2
EOF
)"
```

---

## Task 3：T3 策略启用校验（auto+destructive 未预授权拒绝启用）

**目标**：14 `PolicyUsecase` 的 Create（status=enabled 时）/ Update（execution_mode/scenario_id/status 变更时）/ TogglePolicy（启用时）三入口统一校验：`execution_mode=auto` 且场景 `max_tool_risk=destructive`（经 `AiopsGetScenarioRisk` 扩展返回）时，必须持有生效预授权，否则返回明确错误。`validatePolicyPayload` 保持纯函数不动（无依赖注入），校验放 usecase 层。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/policy.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/external.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/external_clients.go`
- Test: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/policy_test.go`

- [ ] **Step 3.1 扩展外部端口：`AiopsGetScenarioRisk` 返回风险与工具风险**

`external.go` 中既有 `AiopsGetScenarioRisk func(ctx, scenarioID) (string, error)` 仅返回场景风险等级。改为返回结构（调用方仅 execution.go 一处，同步改）：

```go
	// AiopsGetScenarioRisk 13 场景模板风险查询（GET /api/v1/monitor/aiops/scenarios/{id}，
	// Redis 缓存 remediation:scenario-risk:{id} TTL 5min）。
	// 返回：risk_level=场景自身风险；max_tool_risk=图指令引用 MCP 工具最高风险（13 侧 T1 计算）。
	// 查询失败 best-effort 返回空值，调用方按兜底规则处理（不阻断分流/启用外的只读路径）。
	AiopsGetScenarioRisk func(ctx context.Context, scenarioID int64) (riskLevel string, maxToolRisk string, err error)
```

- [ ] **Step 3.2 data 层适配**

`external_clients.go` 中该函数实现由解析单字段改为解析两字段：

```go
// 原：out.RiskLevel 单字段；改为：
	var out struct {
		RiskLevel   string `json:"risk_level"`
		MaxToolRisk string `json:"max_tool_risk"`
	}
	// ... do 请求后：
	return out.RiskLevel, out.MaxToolRisk, nil
```

`execution.go` 中既有调用点（执行记录风险分级）由 `risk, err := uc.external.AiopsGetScenarioRisk(...)` 改为 `risk, _, err := ...`（工具风险在执行分级不使用，策略校验专用）。

- [ ] **Step 3.3 `PolicyUsecase` 注入依赖并加统一校验**

`policy.go` 的 `PolicyUsecase` 结构体追加：

```go
	grants   *PolicyGrantUsecase // 可 nil（未装配时 auto+destructive 一律拒绝启用，fail-closed）
	external *ExternalClients
```

`NewPolicyUsecase` 追加形参 `grants *PolicyGrantUsecase, external *ExternalClients` 并赋值（wire 重生成）。

追加校验函数：

```go
// checkAutoDestructiveGrant 策略-工具风险联动校验（总纲 §3.2 R2）：
// execution_mode=auto 且场景含 destructive 工具时，须持生效预授权；
// 查询失败/无授权一律拒绝（fail-closed），错误文案指导运维先完成预授权。
func (uc *PolicyUsecase) checkAutoDestructiveGrant(ctx context.Context, p *Policy) error {
	if p.ExecutionMode != ExecutionModeAuto || p.Status != PolicyStatusEnabled {
		return nil
	}
	maxToolRisk := ""
	if uc.external != nil && uc.external.AiopsGetScenarioRisk != nil {
		_, risk, err := uc.external.AiopsGetScenarioRisk(ctx, p.ScenarioID)
		if err != nil {
			uc.log.WithContext(ctx).Warnf("query scenario %d max_tool_risk failed (fail-closed for auto policy): %v", p.ScenarioID, err)
			return remediationV1.ErrorPolicyInvalid("无法确认场景 %d 的工具风险等级，auto 策略禁止启用（请稍后重试）", p.ScenarioID)
		}
		maxToolRisk = risk
	}
	if maxToolRisk != "destructive" {
		return nil // ≤high：全自动无 interrupt，无需预授权
	}
	if uc.grants == nil || !uc.grants.HasActiveGrant(ctx, p.ID, p.ScenarioID) {
		return remediationV1.ErrorPolicyInvalid(
			"execution_mode=auto 且场景 %d 含 destructive 工具，须先完成策略预授权（grant_policy=always + approval_ttl）方可启用；未预授权的 auto 策略会被第二层 interrupt 卡住，全自动名存实亡", p.ScenarioID)
	}
	return nil
}
```

> 注意：`CreatePolicy` 时 `p.ID` 尚未分配——授权记录以 policy_id 为键，新建策略天然无授权，auto+destructive 新建即启用必被拒（引导先建为禁用 → 授权 → 再启用，状态机清晰）。Update/Toggle 时 `p.ID` 有效。

- [ ] **Step 3.4 三入口接线**

`CreatePolicy` 中 `created, err := uc.repo.Create(ctx, p)` 之前追加：

```go
	if err = uc.checkAutoDestructiveGrant(ctx, p); err != nil {
		return nil, err
	}
```

`UpdatePolicy` 中 `updated, err := uc.repo.Update(ctx, p, fields)` 之前追加同一调用。

`TogglePolicy` 中 `updated, err := uc.repo.Update(...)` 之前追加：

```go
	if status == 1 {
		if err := uc.checkAutoDestructiveGrant(ctx, p); err != nil {
			return nil, err
		}
	}
```

- [ ] **Step 3.5 单测：四象限行为**

`policy_test.go` 追加（fake 参照既有 `execution_test.go` 模式）：

```go
func TestCheckAutoDestructiveGrant(t *testing.T) {
	newUC := func(maxToolRisk string, hasGrant bool) *PolicyUsecase {
		grantUC := NewPolicyGrantUsecase(&fakePolicyGrantRepo{active: func() *PolicyGrant {
			if hasGrant {
				return &PolicyGrant{ID: 1}
			}
			return nil
		}()}, testLogger())
		return &PolicyUsecase{
			grants: grantUC,
			external: &ExternalClients{
				AiopsGetScenarioRisk: func(context.Context, int64) (string, string, error) {
					return "high", maxToolRisk, nil
				},
			},
			log: log.NewHelper(log.DefaultLogger),
		}
	}
	mk := func() *Policy {
		return &Policy{ID: 7, Name: "p", AlertSource: "internal", ExecutionMode: ExecutionModeAuto,
			ScenarioID: 42, Status: PolicyStatusEnabled}
	}
	// auto + destructive + 无授权 → 拒绝
	if err := newUC("destructive", false).checkAutoDestructiveGrant(context.Background(), mk()); err == nil {
		t.Fatal("expect reject: auto+destructive without grant")
	}
	// auto + destructive + 有授权 → 放行
	if err := newUC("destructive", true).checkAutoDestructiveGrant(context.Background(), mk()); err != nil {
		t.Fatalf("expect pass with grant: %v", err)
	}
	// auto + high → 放行（无需授权）
	if err := newUC("high", false).checkAutoDestructiveGrant(context.Background(), mk()); err != nil {
		t.Fatalf("expect pass for high: %v", err)
	}
	// approval 模式 → 放行（第一层审批覆盖）
	p := mk()
	p.ExecutionMode = ExecutionModeApproval
	if err := newUC("destructive", false).checkAutoDestructiveGrant(context.Background(), p); err != nil {
		t.Fatalf("expect pass for approval mode: %v", err)
	}
	// 禁用状态 → 放行（仅启用时校验）
	p2 := mk()
	p2.Status = PolicyStatusDisabled
	if err := newUC("destructive", false).checkAutoDestructiveGrant(context.Background(), p2); err != nil {
		t.Fatalf("expect pass when disabled: %v", err)
	}
}
```

```bash
cd f:/myproject/twinmonitor/TwinServer
go test ./app/remediation/internal/biz/ -run TestCheckAutoDestructiveGrant -v -count=1
# 预期：PASS
go build ./app/remediation/... ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 3.6 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/remediation/internal/biz/policy.go app/remediation/internal/biz/external.go app/remediation/internal/biz/execution.go app/remediation/internal/biz/policy_test.go app/remediation/internal/data/external_clients.go app/remediation/cmd/wire_gen.go
git commit -m "$(cat <<'EOF'
feat(remediation): auto+destructive 策略启用校验（未预授权拒绝启用）

- AiopsGetScenarioRisk 扩展返回 max_tool_risk（13 T1 判定源）
- Create/Update/Toggle 三入口统一 checkAutoDestructiveGrant，查询失败 fail-closed
- 总纲 §3.2 R2 联动表：auto+destructive 须策略预授权，≤high 全自动

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.2
EOF
)"
```

---

## Task 4：T4 预授权命中自动 Resume 直通（14 消费 waiting_approval → 13 系统级 Resume）

**目标**：总纲 §3.2「授权有效期内 destructive 工具调用自动放行（aranea 侧经系统级 Resume 直通，不再产生审批待办）」。链路：13 webhook 收到 `run.waiting_approval` 照常建 pending 审批（保底，防直通失败悬空）→ 13 发布 `ai.task.events(action=waiting_approval)` → 14 `TaskEventConsumer` 命中策略预授权 → 调 13 新端点 `POST /api/v1/monitor/aiops/tasks/resume-interrupt` → 13 内部原子完成：aranea `ResumeInterrupt(approved=true, comment="策略预授权直通")` + pending 审批置 `approved`（approver=system，备注 source=preauth 留痕）。幂等：审批已处理时仅补 Resume；aranea 侧 interrupt_id 不匹配时记录告警不重试。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/external.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/task_events.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/biz/execution.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/remediation/internal/data/external_clients.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/service/task.go`（新 HTTP 端点）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/task.go`（系统级 Resume 用例方法）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/approval.go`（按 interrupt 系统审批）

- [ ] **Step 4.1 14 外部端口新增 `AiopsResumeInterrupt`**

`external.go` 的 `ExternalClients` 追加：

```go
	// AiopsResumeInterrupt 13 系统级 Resume 直通（POST /api/v1/monitor/aiops/tasks/resume-interrupt）：
	// 预授权命中时由本模块发起，13 内部完成 aranea ResumeInterrupt + pending 审批系统置通过。
	// 入参 run_id/interrupt_id 取自 ai.task.events(waiting_approval) 载荷；comment 留痕授权依据。
	AiopsResumeInterrupt func(ctx context.Context, runID, interruptID, comment string) error
```

- [ ] **Step 4.2 13 端点与用例**

`app/aiops/internal/biz/task.go` 的 `TaskUsecase` 追加（或直接放 `ApprovalUsecase`，按既有分工选 `ApprovalUsecase` 内聚审批语义）：

```go
// SystemResumeInterrupt 系统级 Resume 直通（总纲 §3.2 R2 预授权放行）：
// ① aranea ResumeInterrupt(approved=true)；② 该 interrupt 的 pending 审批置 approved（system）。
// 幂等：审批非 pending 跳过状态回写仍执行 Resume；Resume 失败整体报错由 14 侧留痕重试。
func (uc *ApprovalUsecase) SystemResumeInterrupt(ctx context.Context, runID, interruptID, comment string) error {
	if runID == "" || interruptID == "" {
		return ErrBadRequest
	}
	if err := uc.port.ResumeInterrupt(ctx, runID, interruptID, &ResumeDecision{
		Approved: true,
		Comment:  comment,
	}); err != nil {
		uc.log.WithContext(ctx).Errorf("system resume interrupt run=%s interrupt=%s failed: %v", runID, interruptID, err)
		return err
	}
	if a, err := uc.repo.GetByInterrupt(ctx, interruptID); err == nil && a != nil && a.Status == ApprovalStatusPending {
		now := time.Now()
		if uerr := uc.repo.UpdateStatus(ctx, a.ID, ApprovalStatusApproved, nil, "system(preauth)", comment, &now); uerr != nil {
			uc.log.WithContext(ctx).Warnf("mark approval %d system-approved failed (resumed): %v", a.ID, uerr)
		}
		uc.removeExpire(ctx, a.ID)
	}
	uc.log.WithContext(ctx).Infof("system resume interrupt done: run=%s interrupt=%s comment=%s", runID, interruptID, comment)
	return nil
}
```

`ApprovalRepo` 接口追加：

```go
	// GetByInterrupt 按 interrupt_id 查审批（系统级 Resume 回写用）。
	GetByInterrupt(ctx context.Context, interruptID string) (*Approval, error)
```

data 层 `approval_repo.go` 实现（按 `ExistsPendingByInterrupt` 同款 where 条件，去掉 status 过滤取最新一条）。

`app/aiops/internal/service/` 注册端点（找到 task 相关路由文件，`grep -n "tasks" app/aiops/internal/service/http.go | head` 定位）：

```go
// POST /api/v1/monitor/aiops/tasks/resume-interrupt（14 预授权直通；内网服务间调用，走既有 JWT/ACL）
type resumeInterruptReq struct {
	RunID       string `json:"run_id"`
	InterruptID string `json:"interrupt_id"`
	Comment     string `json:"comment"`
}
// handler：解析 → approvalUC.SystemResumeInterrupt → 200 {"ok":true}；错误映射 4xx/5xx
```

- [ ] **Step 4.3 14 `ApplyTaskEvent` 的 waiting_approval 分支接预授权判定**

`task_events.go` 的 `case "waiting_approval":` 分支改为：

```go
	case "waiting_approval":
		// 策略预授权直通（总纲 §3.2 R2）：auto 策略持生效授权 → 系统级 Resume，不产生人工待办
		if uc.tryPreauthResume(ctx, e, payload) {
			e.appendLog("scenario_approval", "done", "策略预授权命中，系统级 Resume 直通（无需人工审批）")
			return uc.updateExecution(ctx, e)
		}
		e.appendLog("scenario_approval", "waiting", "场景内审批中，请前往 13 审批中心处理")
		return uc.updateExecution(ctx, e)
```

`ExecutionUsecase` 追加（`execution.go`；结构体需注入 `grants *PolicyGrantUsecase`，NewExecutionUsecase 加形参，wire 重生成）：

```go
// tryPreauthResume 预授权命中时发起系统级 Resume 直通。返回 true=已直通。
// 判定链：execution_mode=auto → 预授权生效 → 事件载荷含 run_id/interrupt_id → 调 13 直通端点。
// 任一环不满足/调用失败返回 false（回落人工审批路径，保底不悬空）。
func (uc *ExecutionUsecase) tryPreauthResume(ctx context.Context, e *Execution, payload map[string]any) bool {
	if e.ExecutionMode != ExecutionModeAuto || uc.grants == nil {
		return false
	}
	if !uc.grants.HasActiveGrant(ctx, e.PolicyID, e.ScenarioID) {
		return false
	}
	runID := strField(payload, "", "aranea_run_id", "araneaRunId", "run_id", "runId")
	interruptID := strField(payload, "", "interrupt_id", "interruptId")
	if runID == "" || interruptID == "" || uc.external == nil || uc.external.AiopsResumeInterrupt == nil {
		return false
	}
	comment := fmt.Sprintf("策略预授权直通（policy_id=%d, execution=%s）", e.PolicyID, e.ExecutionNo)
	if err := uc.external.AiopsResumeInterrupt(ctx, runID, interruptID, comment); err != nil {
		uc.log.WithContext(ctx).Warnf("preauth resume failed (fallback to manual approval): execution=%s err=%v", e.ExecutionNo, err)
		return false
	}
	uc.log.WithContext(ctx).Infof("preauth resume done: execution=%s run=%s interrupt=%s", e.ExecutionNo, runID, interruptID)
	return true
}
```

> 载荷键名核对：13 `ai.task.events` 的 waiting_approval 载荷由 `TaskUsecase` 发布段决定，实现前先 `grep -rn "waiting_approval" app/aiops/internal/biz/task.go` 确认实际键名（aranea_run_id / interrupt_id），以实际为准调整 `strField` 候选键。

- [ ] **Step 4.4 单测：直通命中/未命中/失败回落**

`task_events_test.go`（无则新建）：

```go
func TestApplyTaskEventWaitingApprovalPreauth(t *testing.T) {
	resumed := false
	uc, repo := newTestExecutionUsecase(t) // 参照 execution_test.go 既有构造
	uc.grants = NewPolicyGrantUsecase(&fakePolicyGrantRepo{active: &PolicyGrant{ID: 1}}, testLogger())
	uc.external = &ExternalClients{
		AiopsResumeInterrupt: func(_ context.Context, runID, iid, comment string) error {
			resumed = true
			if runID != "run-1" || iid != "remediate" {
				t.Errorf("unexpected resume args: %s %s", runID, iid)
			}
			return nil
		},
	}
	e := seedRunningExecution(t, repo, ExecutionModeAuto) // 测试辅助：造 running 执行记录
	err := uc.ApplyTaskEvent(context.Background(), map[string]any{
		"trigger_ref_id": e.ExecutionNo,
		"task_id":        *e.AiTaskID,
		"action":         "waiting_approval",
		"aranea_run_id":  "run-1",
		"interrupt_id":   "remediate",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("expect preauth resume called")
	}
	// 无授权 → 不直通
	resumed = false
	uc.grants = NewPolicyGrantUsecase(&fakePolicyGrantRepo{}, testLogger())
	_ = uc.ApplyTaskEvent(context.Background(), map[string]any{
		"trigger_ref_id": e.ExecutionNo,
		"task_id":        *e.AiTaskID,
		"action":         "waiting_approval",
		"aranea_run_id":  "run-2",
		"interrupt_id":   "remediate",
	})
	if resumed {
		t.Fatal("expect no resume without grant")
	}
}
```

（测试辅助函数名以 `execution_test.go` 既有 fake 为准适配；核心是构造 running Execution + fake repo。）

```bash
cd f:/myproject/twinmonitor/TwinServer
go test ./app/remediation/internal/biz/ -run TestApplyTaskEventWaitingApprovalPreauth -v -count=1
go test ./app/aiops/internal/biz/ -run TestApproval -v -count=1
go build ./app/aiops/... ./app/remediation/...
# 预期：全 PASS，0 编译错误
```

- [ ] **Step 4.5 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/remediation/internal/biz/external.go app/remediation/internal/biz/task_events.go app/remediation/internal/biz/execution.go app/remediation/internal/biz/task_events_test.go app/remediation/internal/data/external_clients.go app/remediation/cmd/wire_gen.go app/aiops/internal/biz/task.go app/aiops/internal/biz/approval.go app/aiops/internal/data/approval_repo.go app/aiops/internal/service/
git commit -m "$(cat <<'EOF'
feat(remediation): 预授权命中系统级 Resume 直通（auto+destructive 免人工待办）

- 14 消费 ai.task.events(waiting_approval)，授权生效即调 13 resume-interrupt 端点
- 13 SystemResumeInterrupt：aranea ResumeInterrupt + pending 审批系统置通过（preauth 留痕）
- 直通失败回落人工审批路径，保底不悬空；授权过期自动恢复逐次确认

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.2
EOF
)"
```

---

## Task 5：T5 12 预设 Agent 白名单切 MCP（幽灵键修正 + 种子同步幂等推送）

**目标**：`agent_preset.go` 12 个预设 Agent 的 `tool_whitelist` 当前引用 11 个 `ai_mcp_tools` 注册表不存在的键（`alarm.detail/asset.detail/asset.list/server.top/server.disk_usage/server.service_status/server.dmesg/server.journalctl/database.health_check/database.slow_query/database.session_list`）。按总纲附录 A 与注册表实际目录修正映射；`alarm.severity_assess` 按总纲 §2「改为本地规则」从白名单移除；故障诊断/变更执行 Agent 补 P1 新增域工具。种子同步器自带漂移比对，重跑 seed-sync 即幂等推送 aranea。

**映射表（幽灵键 → 注册表真实键）：**

| 现白名单键 | 修正后 | 依据 |
|-----------|--------|------|
| `alarm.detail` | `alarm.get` | 注册表 L163 |
| `asset.detail` | `asset.get` | 注册表 L180 |
| `asset.list` | `asset.get` | 注册表无 list，`asset.get` 支持列表查询（P1 附录 A #7 同此映射） |
| `server.top` | `server.process_list` | 注册表 L210（进程/负载视角最接近 top） |
| `server.disk_usage` | `server.exec_command` | 注册表无专用键；`exec_command`（high，白名单 `df -h` 在 command_whitelist 内）承载 |
| `server.service_status` | `server.exec_command` | 同上（`ss`/查询类命令在白名单） |
| `server.dmesg` | `server.exec_command` | 同上（`dmesg` 在白名单） |
| `server.journalctl` | `server.exec_command` | 同上（`journalctl` 在白名单） |
| `database.health_check` | `db.health_check` | 注册表 L239 |
| `database.slow_query` | `db.slow_query` | 注册表 L242 |
| `database.session_list` | `db.slow_query` | 注册表无会话键；slow_query 含会话/锁视角最接近 |
| `alarm.severity_assess` | （移除） | 总纲 §2：严重度评估改本地规则，消除 MCP↔aranea 循环调用 |

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/agent_preset.go`

- [ ] **Step 5.1 修改 12 个白名单（完整替换 presetAgentSeeds 中对应行）**

```go
// 告警处理 Agent：
ToolWhitelist: []string{"alarm.query", "alarm.get", "metric.query", "asset.get"},
// 故障诊断 Agent（补 P1 network 域取证工具）：
ToolWhitelist: []string{"asset.get", "metric.query", "alarm.query", "alarm.get", "network.line_status", "network.line_events", "server.process_list", "server.exec_command"},
// 日志分析 Agent：
ToolWhitelist: []string{"server.exec_command", "metric.query"},
// 系统巡检 Agent：
ToolWhitelist: []string{"server.process_list", "server.exec_command", "metric.query"},
// 变更执行 Agent（补 gns3 域；destructive 键由 aranea 侧 requires_confirmation 管控）：
ToolWhitelist: []string{"server.exec_command", "server.restart_service", "metric.query", "asset.get", "gns3.exec", "gns3.health_check", "gns3.fault_inject", "gns3.fault_clear"},
// 文档生成 Agent：不变
ToolWhitelist: []string{"knowledge.search"},
// 合规检查 Agent：
ToolWhitelist: []string{"server.exec_command", "asset.get"},
// 服务器命令执行 Agent：
ToolWhitelist: []string{"server.process_list", "server.exec_command"},
// 命令生成专家：不变（空白名单）
// 自动巡检 Agent：
ToolWhitelist: []string{"server.process_list", "server.exec_command", "metric.query", "asset.get"},
// 网络巡检专家：不变（三键均真实存在）
ToolWhitelist: []string{"network.device_list", "network.interface_status", "network.config_backup"},
// 数据库运维 Agent：
ToolWhitelist: []string{"db.health_check", "db.slow_query"},
```

- [ ] **Step 5.2 编译 + 种子漂移比对自测**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
go test ./app/aiops/internal/biz/ -run TestSeed -v -count=1
# 预期：0 错误，种子相关测试 PASS（seedAgentDrifted 对 tool_whitelist 无序比对）
```

- [ ] **Step 5.3 触发种子同步并验证 aranea 侧白名单**

```bash
# 13 服务运行中触发全量种子同步
curl -s -X POST http://localhost:8100/api/v1/monitor/aiops/agents/seed-sync \
  -H "Authorization: Bearer $TWIN_ADMIN_TOKEN" | jq '{total, created, updated, failed}'
# 预期：updated>=11（白名单漂移的 Agent 全部原位更新），failed=0

# aranea 侧验证（12 个 Agent 的 definition.tool_whitelist 全为注册表真实键）
psql "$ARANEA_PG_DSN" -c "
SELECT name, jsonb_array_elements_text(definition->'tool_whitelist') AS tool
FROM agents WHERE name LIKE '%Agent' OR name LIKE '%专家'
ORDER BY name, tool;" > /tmp/aranea_tools.txt
# 逐行核对：不存在 alarm.detail/asset.detail/server.top 等幽灵键
```

- [ ] **Step 5.4 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/agent_preset.go
git commit -m "$(cat <<'EOF'
feat(aiops): 12 预设 Agent 白名单全面切 MCP 真实工具键（总纲 P2）

- 修正 11 个注册表幽灵键：alarm.detail→alarm.get、asset.detail/list→asset.get、
  server.top→server.process_list、server.*→server.exec_command（白名单命令承载）、
  database.*→db.*
- alarm.severity_assess 移除（总纲 §2 改本地规则）
- 故障诊断/变更执行补 network/gns3 新域工具；种子漂移比对幂等推送 aranea

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 附录 A
EOF
)"
```

---

## Task 6：T6 remediate 图节点工具键切换 + aranea 侧 Agent 工具绑定切换

**目标**：① `test/ts10-gns3/incident_response_graph.json` 四节点指令内的内置 twinops 工具名全部替换为 MCP 名（语义/budget/守卫规则不变）；② `agent_preset.go` 的 `systemScenarioGraphJSON["incident-response"]` 同步；③ aranea 侧 remediate 图用 Agent（ops_fault_diagnosis/ops_change_execution/ops_system_inspection/ops_doc_generation）的 `agent_runtime_settings.tools_enabled` 由内置 twinops 键切换为 `twinmonitor.*` MCP 工具键，gns3.fault_* 保持 `requires_confirmation=true`；④ 种子内容戳漂移自动原位更新 aranea 图。

**替换映射（指令文本逐词替换）：**

| 旧（内置 twinops） | 新（MCP） |
|---|---|
| `twin_alarm_get` | `alarm.get` |
| `twin_line_events` | `network.line_events` |
| `gns3_health_check` | `gns3.health_check` |
| `gns3_exec` | `gns3.exec` |
| `gns3_fault_clear` | `gns3.fault_clear` |

**Files:**
- Modify: `f:/myproject/aranea-agents/test/ts10-gns3/incident_response_graph.json`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/agent_preset.go`（systemScenarioGraphJSON 的 incident-response 串）
- Run: aranea PG 配置更新（agents 表 + agent_runtime_settings）

- [ ] **Step 6.1 切换 ts10 图 JSON 指令工具键**

对 `incident_response_graph.json` 做精确替换（5 个键全部全局替换，保持指令其余文字不动）：

```bash
cd f:/myproject/aranea-agents
# 校验替换前出现次数（预期各 ≥1）
grep -c "twin_alarm_get\|twin_line_events\|gns3_health_check\|gns3_exec\|gns3_fault_clear" test/ts10-gns3/incident_response_graph.json
```

编辑后预期指令要点（供 diff 核对）：
- diagnose 节点：`1) alarm.get（告警 ID 用输入中的告警编号）…2) network.line_events 看该线路中断/恢复历史；3) gns3.health_check 探测仿真设备健康`
- remediate 节点：`调用1＝gns3.exec(device=\"sw1\", cmd=\"ip link show\")…调用2＝gns3.fault_clear(port=…)…调用3＝gns3.exec(device=\"sw1\", cmd=\"ip link show <该端口>\")`（硬时序/预算/拦截解读文案不变）
- verify 节点：`1) gns3.exec(…)——端口 state UP 是唯一硬性判据；2) alarm.get…3) gns3.health_check…禁止调用 network.line_probe`
- postmortem 节点：`knowledge_write` 保留不变（aranea 内部工具，非 MCP 通道）

```bash
# 替换后校验：旧键 0 命中
grep -c "twin_alarm_get\|twin_line_events\|gns3_health_check\|gns3_exec\|gns3_fault_clear" test/ts10-gns3/incident_response_graph.json
# 预期：0
grep -c "gns3\.exec\|gns3\.fault_clear\|gns3\.health_check\|alarm\.get\|network\.line_events" test/ts10-gns3/incident_response_graph.json
# 预期：≥10
```

- [ ] **Step 6.2 同步 `systemScenarioGraphJSON["incident-response"]`**

`agent_preset.go` 中 incident-response 内置图串当前指令未点名工具（简化版），但 remediate 节点写「本场景为演练环境，不真正下发破坏性命令」与真实图语义脱节。对齐真实图语义（仍保持简化版 4 节点、不含条件边）：

```go
// remediate 节点 instruction 改为：
"instruction":"你是故障处置闭环的第二环「修复执行」。输入是上游的根因诊断结论。请给出：1) 分步修复执行计划（每步标注操作与预期结果，设备操作经 MCP 工具 gns3.exec / gns3.fault_clear 发起）；2) 每步风险与回退方法；3) 执行结论。输出中文 Markdown，控制在 300 字内。"
// verify 节点 instruction 改为：
"instruction":"你是故障处置闭环的第三环「效果验证」。输入是修复执行记录。请给出客观验证方案：1) 验证项清单（gns3.exec 端口状态 / alarm.get 告警状态 / gns3.health_check 设备健康）；2) 每项的通过判据；3) 验证结论（通过/不通过 + 证据）。输出中文 Markdown，控制在 250 字内。"
```

- [ ] **Step 6.3 aranea 侧 Agent 工具绑定切换（agents 表 + agent_runtime_settings）**

写迁移 SQL 文件 `f:/myproject/aranea-agents/test/ts10-gns3/p2_mcp_cutover.sql`（写操作三层校验：先 SELECT COUNT，事务包裹，核验 affected rows）：

```sql
-- P2 MCP 切换：remediate 图 Agent 工具绑定 内置 twinops → twinmonitor MCP
BEGIN;

-- ① 前置核对（预期 4 行：ops_fault_diagnosis/ops_change_execution/ops_system_inspection/ops_doc_generation）
SELECT agent_key, tools_enabled FROM agent_runtime_settings
WHERE agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation');

-- ② 工具键切换（tools_enabled JSONB 数组内逐键替换；仅命中含旧键的行）
UPDATE agent_runtime_settings
SET tools_enabled = (
  SELECT jsonb_agg(
    CASE elem
      WHEN '"gns3_exec"' THEN '"twinmonitor.gns3.exec"'
      WHEN '"gns3_fault_clear"' THEN '"twinmonitor.gns3.fault_clear"'
      WHEN '"gns3_fault_inject"' THEN '"twinmonitor.gns3.fault_inject"'
      WHEN '"gns3_health_check"' THEN '"twinmonitor.gns3.health_check"'
      WHEN '"twin_alarm_get"' THEN '"twinmonitor.alarm.get"'
      WHEN '"twin_alarm_query"' THEN '"twinmonitor.alarm.query"'
      WHEN '"twin_line_events"' THEN '"twinmonitor.network.line_events"'
      WHEN '"twin_line_status"' THEN '"twinmonitor.network.line_status"'
      ELSE elem
    END::jsonb
  )
  FROM jsonb_array_elements(tools_enabled) AS elem
),
updated_at = now()
WHERE agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation')
  AND tools_enabled::text ~ '(gns3_exec|gns3_fault_clear|gns3_fault_inject|gns3_health_check|twin_alarm_get|twin_alarm_query|twin_line_events|twin_line_status)';

-- ③ requires_confirmation 高危键切换（fault_inject/fault_clear 保持 true）
UPDATE agent_runtime_settings
SET tools_confirmation = (
  SELECT jsonb_object_agg(
    CASE key
      WHEN 'gns3_fault_clear' THEN 'twinmonitor.gns3.fault_clear'
      WHEN 'gns3_fault_inject' THEN 'twinmonitor.gns3.fault_inject'
      ELSE key
    END, value
  )
  FROM jsonb_each(tools_confirmation)
),
updated_at = now()
WHERE agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation')
  AND tools_confirmation::text ~ '(gns3_fault_clear|gns3_fault_inject)';

-- ④ agents 表 definition.tool_whitelist 同步（若有该字段且含旧键）+ bump updated_at 防构建缓存假阴性
-- （项目记忆：直改 agents 表必须同步 bump updated_at，否则 buildKeyFP 命中旧构建）
UPDATE agents
SET updated_at = to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
WHERE agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation');

-- ⑤ 核验（预期：旧键 0 命中；②+③ affected rows ≥1）
SELECT agent_key, tools_enabled, tools_confirmation FROM agent_runtime_settings
WHERE agent_key IN ('ops_fault_diagnosis','ops_change_execution','ops_system_inspection','ops_doc_generation');

COMMIT;
```

> 注：MCP 工具在 aranea 侧的可见名 = `{toolset_name}_{remote}`（NamedToolSet 前缀规则），toolset 名以 P1 登记的 `mcp_servers` 行实际 `server_key` 为准——若 P1 用 `server_key=twinmonitor`，则前缀为 `twinmonitor_` 还是 `twinmonitor.` 取决于框架 NamedToolSet 拼接实现（`_` 拼接）。**执行前先查证**：`SELECT server_key FROM mcp_servers;` 并在 aranea 日志确认实际注册工具名（`tools/list` 返回名），按真实前缀修正本 SQL 的键名。若实际前缀为 `twinmonitor_gns3.exec` 形态，全部 `twinmonitor.` 改为 `twinmonitor_`。

执行：

```bash
psql "$ARANEA_PG_DSN" -f f:/myproject/aranea-agents/test/ts10-gns3/p2_mcp_cutover.sql
# 预期：②/③ UPDATE 各 ≥1 行，⑤ 查询结果无旧键
```

- [ ] **Step 6.4 种子同步推送图漂移更新 + 验证**

```bash
curl -s -X POST http://localhost:8100/api/v1/monitor/aiops/agents/seed-sync \
  -H "Authorization: Bearer $TWIN_ADMIN_TOKEN" | jq '.failed'
# 预期：0（incident-response 图内容戳漂移 → 原位 UpdateGraph，aranea 图 ID 不变）

# aranea 图定义验证
curl -s http://localhost:8000/api/v1/graphs/<incident-response图ID> \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq -r '.definition' | grep -c "gns3_exec"
# 预期：0
```

- [ ] **Step 6.5 git commit（双仓库）**

```bash
cd f:/myproject/aranea-agents
git add test/ts10-gns3/incident_response_graph.json test/ts10-gns3/p2_mcp_cutover.sql
git commit -m "$(cat <<'EOF'
test(gns3): incident-response 图指令工具键切 MCP（总纲 P2）

- twin_alarm_get→alarm.get / twin_line_events→network.line_events
- gns3_exec→gns3.exec / gns3_fault_clear→gns3.fault_clear / gns3_health_check→gns3.health_check
- budget/守卫/拦截解读语义不变；新增 p2_mcp_cutover.sql 切换 aranea 侧 Agent 工具绑定

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P2
EOF
)"

cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/agent_preset.go
git commit -m "$(cat <<'EOF'
feat(aiops): incident-response 内置图指令对齐 MCP 工具名

- remediate/verify 节点指令点名 gns3.exec/gns3.fault_clear/alarm.get/gns3.health_check
- 内容戳漂移驱动种子同步原位更新 aranea 图（ID 不变）

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P2
EOF
)"
```

---

## Task 7：T7 E2E 验证（守卫/budget MCP 通道不变形 + 双层审批 + 预授权直通 + L3 记忆）

**目标**：test/ts10-gns3 环境全链路验证 P2 切换后的关键行为：① 循环守卫对 MCP 工具名生效（签名哈希含工具名，换名不影响语义）；② remediate budget 规则（取证≤2 → 第 3 次必须 `gns3.fault_clear`）在 MCP 延迟下不变形；③ 双层审批：未预授权时 `gns3.fault_clear` 触发 interrupt → 13 审批中心 → Resume → 闭环；④ 预授权直通：授予后同场景全程无 interrupt；⑤ L3 记忆 scope=agent 写入可查。

**Files:**
- Create: `f:/myproject/aranea-agents/test/ts10-gns3/e2e_p2_mcp_cutover.ps1`
- Create: `f:/myproject/aranea-agents/test/ts10-gns3/evidence-p2/README.md`（验证记录）

- [ ] **Step 7.1 E2E 脚本：注入故障 → 触发图执行 → 断言工具序列与守卫**

新建 `e2e_p2_mcp_cutover.ps1`（复用 ts10 既有驱动模式：llm_relay.py 抓包 + analyze_capture.py 分析）：

```powershell
# P2 MCP 切换 E2E（test/ts10-gns3，aranea 8810 / aiops 8100 / gns3_agent 18081）
# 前置：llm_relay.py 已启动（:8899→deepseek）、aranea/aiops/remediation 已重启加载新配置
param(
  [string]$Aranea = "http://localhost:8810",
  [string]$Aiops  = "http://localhost:8100",
  [string]$EvidenceDir = "$PSScriptRoot/evidence-p2"
)
$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force $EvidenceDir | Out-Null

# ① 注入故障（eth1 down），制造真实修复场景
Invoke-RestMethod -Method Post -Uri "http://localhost:18081/fault/sw1-port" `
  -ContentType "application/json" -Body '{"port":"eth1","state":"down"}' | Out-Null
Start-Sleep -Seconds 2

# ② 创建图执行（incident-response；告警上下文注入）
$run = Invoke-RestMethod -Method Post -Uri "$Aranea/api/v1/runs" `
  -Headers @{ Authorization = "Bearer $env:ARANEA_TWINOPENAPI_TOKEN" } `
  -ContentType "application/json" -Body (@{
    graph_id = $env:P2_GRAPH_ID
    params   = @{ input = "告警标题: SW1-eth1 线路中断`n告警ID: ALM-P2-E2E-001`n资产ID: 0" }
    webhook_url = "http://aiops:8100/api/v1/monitor/aiops/webhooks/aranea"
    idempotency_key = "p2-e2e-" + [guid]::NewGuid().ToString("N").Substring(0,8)
  } | ConvertTo-Json -Depth 6)
$runId = $run.run_id
"run_id=$runId" | Out-File "$EvidenceDir/run-id.txt"

# ③ 等待终态（含 interrupt 等待 + 人工/直通 Resume，预算 600s）
$deadline = (Get-Date).AddSeconds(600)
$final = $null
while ((Get-Date) -lt $deadline) {
  $r = Invoke-RestMethod -Uri "$Aranea/api/v1/runs/$runId" `
    -Headers @{ Authorization = "Bearer $env:ARANEA_TWINOPENAPI_TOKEN" }
  if ($r.status -in @("completed","failed","cancelled")) { $final = $r; break }
  if ($r.status -eq "waiting_approval") {
    # 双层审批路径（未预授权）：调 13 审批中心通过首个 pending
    $pending = Invoke-RestMethod -Uri "$Aiops/api/v1/monitor/aiops/approvals?status=pending&page_size=1" `
      -Headers @{ Authorization = "Bearer $env:TWIN_ADMIN_TOKEN" }
    if ($pending.items.Count -gt 0) {
      Invoke-RestMethod -Method Post -Uri "$Aiops/api/v1/monitor/aiops/approvals/$($pending.items[0].id)/approve" `
        -Headers @{ Authorization = "Bearer $env:TWIN_ADMIN_TOKEN" } `
        -ContentType "application/json" -Body '{"comment":"P2 E2E 双层审批验证通过"}' | Out-Null
      "approved_interrupt_at=$(Get-Date -Format o)" | Out-File "$EvidenceDir/approval.txt"
    }
  }
  Start-Sleep -Seconds 5
}
$final | ConvertTo-Json -Depth 8 | Out-File "$EvidenceDir/run-final.json"
if ($final.status -ne "completed") { throw "run not completed: $($final.status)" }

# ④ 抓包分析：MCP 工具序列 + 守卫拦截 + budget
python "$PSScriptRoot/analyze_capture.py" --capture "$PSScriptRoot/llm-capture.jsonl" `
  --out "$EvidenceDir/tool-sequence.json"
# 断言在 Step 7.2 手工核对（见下）
"E2E PASS" | Out-File "$EvidenceDir/verdict.txt"
```

- [ ] **Step 7.2 断言清单（对照 tool-sequence.json 人工/脚本核对）**

```bash
# ① 全部 gns3/alarm/network 调用均为 MCP 名（无内置 twinops 名）
grep -c "gns3_exec\|gns3_fault_clear\|twin_alarm_get\|twin_line_events" evidence-p2/tool-sequence.json
# 预期：0
grep -c "gns3\.exec\|gns3\.fault_clear\|alarm\.get" evidence-p2/tool-sequence.json
# 预期：≥3（取证+恢复+复核）

# ② 守卫：remediate 节点同参第 3 次调用被拦截（消息含「⚠ 系统拦截」），或 budget 内正常推进未触发
grep -c "系统拦截" evidence-p2/tool-sequence.json
# 预期：≥0（触发则核对拦截后模型按纠偏推进 fault_clear，未重发被拦调用）

# ③ budget：remediate 节点工具调用总数 ≤4，且 fault_clear 在取证之后
# ④ 13 审计：ai_mcp_call_history 含本次 gns3 域调用且 plane=gns3_sim
psql "$TWIN_PG_DSN" -c "
SELECT tool_name, plane, status FROM twinmonitor_log.ai_mcp_call_history
WHERE create_time > now() - interval '15 minutes' AND domain = 'gns3' ORDER BY id DESC LIMIT 10;"
# 预期：gns3.exec/gns3.fault_clear 行存在，plane=gns3_sim

# ⑤ 审批闭环（未预授权路径）：ai_approvals 有 approved 记录且 approver_name 为审批人
psql "$TWIN_PG_DSN" -c "
SELECT source, status, approver_name FROM twinmonitor_log.ai_approvals
WHERE create_time > now() - interval '15 minutes' ORDER BY id DESC LIMIT 3;"
# 预期：source=graph, status=approved
```

- [ ] **Step 7.3 预授权直通验证**

```bash
# ① 授予预授权（14 库直写，或后续治理 API；此处 SQL 三层校验）
psql "$TWIN_PG_DSN" -c "
BEGIN;
SELECT COUNT(*) FROM remediation_policies WHERE id = <POLICY_ID>;
INSERT INTO remediation_policy_grants (policy_id, scenario_id, tool_keys, grant_policy, expire_at, granted_by, granted_by_id, reason, status, create_time, update_time)
VALUES (<POLICY_ID>, <SCENARIO_ID>, '[\"gns3.fault_clear\"]', 'always', now() + interval '24 hours', '值班长E2E', 1, 'P2 直通验证', 1, now(), now());
SELECT policy_id, tool_keys, expire_at FROM remediation_policy_grants WHERE policy_id = <POLICY_ID>;
COMMIT;"

# ② 再次注入故障并触发图执行（同 Step 7.1 ①②③，跳过审批动作）
# ③ 断言：全程无 waiting_approval 停留（run 状态不进入等待或秒级直通）；
#    ai_approvals 该 interrupt 行为 approved 且 approver_name='system(preauth)'
psql "$TWIN_PG_DSN" -c "
SELECT status, approver_name, reject_reason FROM twinmonitor_log.ai_approvals
WHERE aranea_interrupt_id IS NOT NULL ORDER BY id DESC LIMIT 1;"
# 预期：status=approved, approver_name=system(preauth)
```

- [ ] **Step 7.4 L3 记忆写入验证（scope=agent）**

```bash
# ① 写入（scope=agent，scope_id=agent 标识；经 twinmonitor OpenAPI 门面）
curl -s -X POST http://localhost:8810/api/v1/memory/facts \
  -H "Authorization: Bearer $ARANEA_TWINOPENAPI_TOKEN" \
  -H "Content-Type: application/json" -d '{
    "scope": "agent",
    "key": "ops_change_execution",
    "content": "P2 E2E：SW1 eth1 端口 DOWN → gns3.fault_clear 恢复有效（MTTR≈40s）",
    "metadata": {"source": "twinmonitor_remediation", "policy_id": "1", "success": true, "mttr_seconds": 40}
  }'
# 预期：{"ok":true}

# ② aranea facts 表可查（scope_type=agent）
psql "$ARANEA_PG_DSN" -c "
SELECT scope_type, scope_id, statement FROM facts
WHERE scope_type = 'agent' AND scope_id = 'ops_change_execution'
ORDER BY id DESC LIMIT 3;"
# 预期：1+ 行，statement 含 gns3.fault_clear
```

- [ ] **Step 7.5 证据归档 + git commit**

```bash
cd f:/myproject/aranea-agents
git add test/ts10-gns3/e2e_p2_mcp_cutover.ps1 test/ts10-gns3/evidence-p2/
git commit -m "$(cat <<'EOF'
test(gns3): P2 MCP 切换 E2E 验证（守卫/budget/双层审批/预授权直通/L3 记忆）

- MCP 通道下 remediate budget 规则不变形，守卫按新工具名正常判定
- 未预授权：fault_clear interrupt → 13 审批 → Resume → 闭环
- 预授权：系统级 Resume 直通，approver=system(preauth)，无人工待办
- L3 记忆 scope=agent 写入 facts 表可查

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 §3.2
EOF
)"
```

---

## 验收清单（Sign-off）

- [ ] T1：13 场景详情返回 `max_tool_risk`，单测 PASS，incident-response 场景实测 `destructive`。
- [ ] T2：`remediation_policy_grants` 表建成（ent 自动迁移 + init SQL），授予/生效/过期/撤销语义单测 PASS。
- [ ] T3：auto+destructive 未预授权策略启用被拒（Create/Update/Toggle 三入口），错误文案含预授权指引；四象限单测 PASS。
- [ ] T4：预授权命中时 `waiting_approval` 系统级 Resume 直通，审批记录 approver=`system(preauth)`；直通失败回落人工审批；单测 PASS。
- [ ] T5：12 预设 Agent 白名单无幽灵键，seed-sync 后 aranea 侧定义一致，`alarm.severity_assess` 已移除。
- [ ] T6：incident-response 图（ts10 + 13 种子串）指令全部 MCP 工具名；aranea 侧 4 个 Agent 工具绑定完成切换，`gns3.fault_*` 保持 requires_confirmation；agents 表 updated_at 已 bump。
- [ ] T7：E2E 证据归档——MCP 工具序列正确、守卫/budget 不变形、双层审批闭环、预授权直通、L3 scope=agent 可查、`ai_mcp_call_history.plane=gns3_sim`。
- [ ] 全局：`go build ./app/aiops/... ./app/remediation/...`（twinmonitor）与 `go build ./cmd/... ./internal/...`（aranea）无编译错误。

---

## 发现的总纲与代码不一致之处

1. **12 预设 Agent 白名单含 11 个幽灵键**：总纲假设白名单已是合法 MCP 键，实际 `agent_preset.go` 引用的 `alarm.detail/asset.detail/asset.list/server.top/server.disk_usage/server.service_status/server.dmesg/server.journalctl/database.health_check/database.slow_query/database.session_list` 在 `ai_mcp_tools` 注册表（24 内置工具）中不存在——总纲 F6「MCP：server.top / server.disk_usage / server.service_status」与注册表（仅 `server.process_list/exec_command/restart_service`）也不一致。本计划按注册表真实键映射修正（server.* 查询类由 `server.exec_command` 白名单命令承载）。
2. **webhook 事件名**：总纲 §5.2 事件表写 `run.interrupted`，aranea 实际发送 `run.waiting_approval`（twin_openapi_compat.go `OnRunWaitingApproval`），13 消费常量亦为 `run.waiting_approval`。本计划按代码实际名实现，总纲事件表需下一版修订。
3. **`alarm.severity_assess` 仍在注册表且被告警处理 Agent 引用**：总纲 §2 明确「严重度评估改为本地规则，消除 MCP↔aranea 循环调用」，但工具仍在 `mcp_registry.go` 注册（readonly）。本计划仅从 Agent 白名单移除该键，注册表保留（供其他调用方），是否下线工具本身留待 P3 退役阶段裁定。
4. **MCP 工具在 aranea 侧的可见名前缀待实证**：框架 `NamedToolSet` 以 `_` 拼接 toolset 名与远端工具名（`{name}_{remote}`），与总纲附录 A 的 `gns3.exec` 点分风格不同；P1 登记 `server_key=twinmonitor` 时 aranea 侧实际名可能为 `twinmonitor_gns3.exec`。T6 Step 6.3 的 SQL 已内置「先查证再执行」守卫，执行时以 `tools/list` 实际返回名为准。
