# P1 MCP 工具扩展与 aranea 侧登记实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 依据总纲 §3.1 决策 1（MCP 单通道收敛）与附录 A 工具映射全表，在 twinmonitor 13-aiops MCP-Server 新增 10 个内置工具（gns3 域 4 个：`gns3.health_check` / `gns3.exec` / `gns3.fault_inject` / `gns3.fault_clear`；network 域 3 个：`network.line_status` / `network.line_events` / `network.line_probe`；ops 域 3 个：`ops.remediation_status` / `ops.collector_status` / `ops.inspection_query`），并完成 aranea 侧 `mcp_servers` 表登记 twinmonitor MCP-Server（SSE 通道）。新增工具全部经 13 内部 HTTP 客户端转发到对应业务模块（gns3 → gns3_agent；network → 08 线路监控；ops → 14/03/16），实现审计与安全策略收口。

**Architecture:** 13-aiops 的 MCP 工具分两层：`internal/biz/mcp_registry.go` 的 `builtinMcpTools()` 声明内置工具目录（启动时 upsert 进 `ai_mcp_tools`，幂等保留治理调整），`internal/biz/mcp_call.go` 的 `executeTool` 实现各工具的后端转发逻辑（经 `AssetClient`/`AlarmClient`/`OpsToolClient` 等端口调对应模块 HTTP API）。`ai_mcp_call_history` 表（`internal/data/ent_log/schema/mcp_call_history.go`）需新增 `plane` 字段标记目标平面（`gns3_sim` / `production` / `readonly`），满足总纲 §3.1.1 审计要求。aranea 侧通过 `mcp_servers` 表（`internal/data/ent/schema/platform_mcp_server.go`）登记 SSE 端点，`MCPVersionHash` 按 server_key + ID + ConfigJSON 计算（项目既有规则）。

**Tech Stack:** Go + Kratos（twinmonitor 13-aiops）+ Ent（`ent_log` schema 代码生成）+ aranea-agents（PG `mcp_servers` 表 + MCP Host SSE 客户端）。

**前置依赖：** P0（aranea 在环最小闭环已通，`GET /api/v1/health` 200）。

---

## 全局约定

- **TDD 铁律**：每个 Task 先写失败测试/验证脚本，再补实现。
- **验证命令**（每个 Task 收尾必跑）：
  - twinmonitor: `cd f:/myproject/twinmonitor/TwinServer && go build ./app/aiops/...`
  - aranea: `cd f:/myproject/aranea-agents && go build ./cmd/... ./internal/...`
- **ent 代码生成**：twinmonitor 侧 ent schema 改动后必须跑代码生成（项目记忆「全量构建前先重生成」）：`cd app/aiops/internal/data/ent_log && go run -mod=mod ./entc.go`
- **commit 风格**：twinmonitor 仓库用 `feat(aiops): ...`；aranea 仓库用 `feat(mcp): ...`（参照 `git log --oneline` 既有前缀惯例）。

---

## Task 1：T1 `ai_mcp_call_history` 表新增 `plane` 字段（ent schema + 代码生成）

**目标**：在 `ai_mcp_call_history` 表新增 `plane` 字段（目标平面标记），用于审计 gns3 仿真平面与生产平面隔离（总纲 §3.1.1：「审计记录含目标平面标记 `plane=gns3_sim`」）。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/ent_log/schema/mcp_call_history.go`
- Generate: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/ent_log/`（ent 生成物）

- [ ] **Step 1.1 修改 ent schema 新增 `plane` 字段**

在 `mcp_call_history.go` 的 `Fields()` 中 `trace_id` 字段之后追加：

```go
		field.String("plane").
			Comment("目标平面标记：gns3_sim=GNS3 仿真演练平面 / production=生产通道 / readonly=纯只读查询").
			Optional().
			Nillable().
			MaxLen(32),
```

- [ ] **Step 1.2 重新生成 ent 代码**

```bash
cd f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/ent_log
go run -mod=mod ./entc.go
```

预期输出：无错误，`ent_log/mcpcallhistory/` 目录下生成物更新（含 `Plane` 字段与 `SetPlane`/`PlaneEQ` 等方法）。

- [ ] **Step 1.3 编译验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 1.4 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/data/ent_log/schema/mcp_call_history.go app/aiops/internal/data/ent_log/
git commit -m "$(cat <<'EOF'
feat(aiops): ai_mcp_call_history 新增 plane 字段（总纲 P1）

- 目标平面标记：gns3_sim / production / readonly
- 满足总纲 §3.1.1 gns3 仿真平面审计隔离要求

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1
EOF
)"
```

---

## Task 2：T2 `McpCallRecord` 结构体与仓储层补 `plane` 字段透传

**目标**：`McpCallRecord`（biz 层）与 `mcpCallHistoryRepo`（data 层）补 `Plane` 字段，确保 `executeTool` 中 gns3 域工具写入 `plane=gns3_sim`。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp.go`（`McpCallRecord` 结构体）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/data/mcp_call_history_repo.go`

- [ ] **Step 2.1 `McpCallRecord` 结构体新增 `Plane` 字段**

在 `mcp.go` 的 `McpCallRecord` 结构体 `TraceID` 之后追加：

```go
	Plane         string
```

- [ ] **Step 2.2 `mcpCallHistoryRepo.Create` 补 `plane` 写入**

在 `mcp_call_history_repo.go` 的 `Create` 方法中 `if rec.TraceID != ""` 之后追加：

```go
	if rec.Plane != "" {
		builder.SetPlane(rec.Plane)
	}
```

- [ ] **Step 2.3 `mcpCallRecordToBiz` 补 `plane` 回读**

在 `mcp_call_history_repo.go` 的 `mcpCallRecordToBiz` 中 `if e.TraceID != nil` 之后追加：

```go
	if e.Plane != nil {
		rec.Plane = *e.Plane
	}
```

- [ ] **Step 2.4 编译验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 2.5 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/mcp.go app/aiops/internal/data/mcp_call_history_repo.go
git commit -m "$(cat <<'EOF'
feat(aiops): McpCallRecord 与仓储层补 plane 字段透传

- biz 层 McpCallRecord 新增 Plane 字段
- data 层 Create/回读同步补 plane

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1
EOF
)"
```

---

## Task 3：T3 `builtinMcpTools()` 新增 gns3 域 4 个工具声明

**目标**：在 `mcp_registry.go` 的 `builtinMcpTools()` 中追加 gns3 域 4 个工具（`gns3.health_check` / `gns3.exec` / `gns3.fault_inject` / `gns3.fault_clear`），风险等级与总纲附录 A 完全一致。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_registry.go`

- [ ] **Step 3.1 追加 gns3 域工具声明**

在 `builtinMcpTools()` 的 `config.render` 条目之后、`return` 之前追加：

```go
		// ---- gns3 仿真演练域（与 16 opstools 生产通道物理隔离，plane=gns3_sim；总纲 §3.1.1 R1）----
		{Name: "gns3.health_check", Domain: "gns3", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "GNS3 仿真设备健康检查（gns3_agent HTTP 业务级探测）",
			ParamsSchema: schema(nil, map[string]any{
				"device": map[string]any{"type": "string", "description": "设备名（如 sw1/pc1）；不传返回全部设备健康"},
			})},
		{Name: "gns3.exec", Domain: "gns3", RiskLevel: McpRiskHigh, IsReadonly: 0,
			Description: "在 GNS3 仿真设备控制台执行命令（只读白名单：ping/show/ip 查询/traceroute/arp/cat/echo/curl 等；写操作一律拒绝）",
			ParamsSchema: schema([]string{"device", "cmd"}, map[string]any{
				"device": map[string]any{"type": "string", "description": "目标设备名（gns3_agent 已纳管）"},
				"cmd":    map[string]any{"type": "string", "description": "控制台命令（只读白名单）"},
			})},
		{Name: "gns3.fault_inject", Domain: "gns3", RiskLevel: McpRiskDestructive, IsReadonly: 0,
			Description: "【高危】向 SW1 指定端口注入故障（端口 down），仅演练环境；aranea 侧须配置 requires_confirmation=true 触发 HITL interrupt",
			ParamsSchema: schema([]string{"port"}, map[string]any{
				"port": map[string]any{"type": "string", "enum": []any{"eth0", "eth1", "eth2", "eth3"}, "description": "SW1 演练端口"},
			})},
		{Name: "gns3.fault_clear", Domain: "gns3", RiskLevel: McpRiskDestructive, IsReadonly: 0,
			Description: "【高危】恢复 SW1 指定端口（端口 up），清除已注入故障；aranea 侧须配置 requires_confirmation=true",
			ParamsSchema: schema([]string{"port"}, map[string]any{
				"port": map[string]any{"type": "string", "enum": []any{"eth0", "eth1", "eth2", "eth3"}, "description": "SW1 演练端口"},
			})},
```

- [ ] **Step 3.2 编译验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 3.3 运行既有测试确保无回归**

```bash
cd f:/myproject/twinmonitor/TwinServer
go test ./app/aiops/internal/biz/... -run TestMcp -v -count=1
# 预期：PASS（既有 MCP 测试不受新增工具声明影响）
```

- [ ] **Step 3.4 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/mcp_registry.go
git commit -m "$(cat <<'EOF'
feat(aiops): MCP 注册表新增 gns3 域 4 个工具声明（总纲 P1）

- gns3.health_check=readonly / gns3.exec=high
- gns3.fault_inject / gns3.fault_clear=destructive
- 与 16 opstools 生产通道物理隔离（plane=gns3_sim）

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1 附录 A
EOF
)"
```

---

## Task 4：T4 `executeTool` 新增 gns3 域 4 个工具后端转发（gns3_agent HTTP）

**目标**：在 `mcp_call.go` 的 `executeTool` 中实现 gns3 域 4 个工具的后端转发逻辑，调用 gns3_agent HTTP 端点（与 aranea 内置 `twinops` 工具的 gns3 端点契约一致：`GET /health[/device]`、`POST /exec`、`POST /fault/sw1-port`），并在调用历史写入 `plane=gns3_sim`。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_call.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/clients.go`（新增 `GNS3AgentClient` 端口）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp.go`（`McpUsecase` 注入新端口）

- [ ] **Step 4.1 新增 `GNS3AgentClient` 端口定义**

在 `clients.go` 的 `OpsToolClient` 定义之后追加：

```go
// GNS3AgentClient GNS3 仿真演练平面后端（gns3_agent，默认 http://127.0.0.1:18081）。
// 与 16 opstools 生产通道物理隔离（总纲 §3.1.1 R1：演练流量不得误入生产通道）。
// 端点契约与 aranea 内置 twinops 工具一致（internal/tools/twinops/twinops.go）。
type GNS3AgentClient interface {
	// Health 设备健康检查：device 为空查全部，否则查单台（GET /health[/device]）。
	Health(ctx context.Context, device string) (map[string]any, error)
	// Exec 控制台命令执行（POST /exec，body={"device","cmd"}；只读白名单本模块侧先过滤）。
	Exec(ctx context.Context, device, cmd string) (map[string]any, error)
	// SetPortState SW1 端口状态切换（POST /fault/sw1-port，body={"port","state":"up"|"down"}）。
	SetPortState(ctx context.Context, port, state string) (map[string]any, error)
}
```

- [ ] **Step 4.2 `McpUsecase` 注入 `gns3` 字段**

在 `mcp.go` 的 `McpUsecase` 结构体 `ops OpsToolClient` 之后追加字段：

```go
	gns3        GNS3AgentClient // 可 nil（gns3 域工具未装配时返回明确错误）
```

并在 `NewMcpUsecase` 函数签名 `ops OpsToolClient,` 之后追加形参：

```go
	gns3 GNS3AgentClient,
```

在函数体 `ops: ops,` 之后追加：

```go
		gns3:        gns3,
```

> **注意**：`NewMcpUsecase` 由 wire 依赖注入装配，新增形参后需同步在 wire ProviderSet 中提供 `GNS3AgentClient` 实现（见 Step 4.5）。

- [ ] **Step 4.3 `executeTool` 新增 gns3 域工具执行分支**

在 `mcp_call.go` 的 `executeTool` 中 `// ---- 配置模板 ----` 分支之后、脚本动态工具分支之前追加：

```go
	// ---- gns3 仿真演练域（gns3_agent，plane=gns3_sim）----
	case "gns3.health_check":
		if uc.gns3 == nil {
			return nil, fmt.Errorf("gns3 域工具未装配（GNS3AgentClient 为 nil）")
		}
		return uc.gns3.Health(ctx, stringParam(params, "device"))
	case "gns3.exec":
		if uc.gns3 == nil {
			return nil, fmt.Errorf("gns3 域工具未装配（GNS3AgentClient 为 nil）")
		}
		return uc.gns3.Exec(ctx, stringParam(params, "device"), stringParam(params, "cmd"))
	case "gns3.fault_inject":
		if uc.gns3 == nil {
			return nil, fmt.Errorf("gns3 域工具未装配（GNS3AgentClient 为 nil）")
		}
		return uc.gns3.SetPortState(ctx, stringParam(params, "port"), "down")
	case "gns3.fault_clear":
		if uc.gns3 == nil {
			return nil, fmt.Errorf("gns3 域工具未装配（GNS3AgentClient 为 nil）")
		}
		return uc.gns3.SetPortState(ctx, stringParam(params, "port"), "up")
```

- [ ] **Step 4.4 `CallTool` 中 gns3 域写入 `plane=gns3_sim`**

在 `mcp_call.go` 的 `CallTool` 中 `rec.Domain, rec.RiskLevel, rec.Source = tool.Domain, tool.RiskLevel, tool.Source` 之后追加：

```go
	// 目标平面标记（总纲 §3.1.1：gns3 域审计含 plane=gns3_sim）
	if tool.Domain == "gns3" {
		rec.Plane = "gns3_sim"
	} else if tool.IsReadonly == 1 {
		rec.Plane = "readonly"
	} else {
		rec.Plane = "production"
	}
```

- [ ] **Step 4.5 gns3_exec 命令白名单复用（checkCommandSafety 补 gns3.exec 分支）**

在 `mcp_call.go` 的 `checkCommandSafety` 中 `case tool.Name == "server.exec_command":` 之后追加：

```go
	case tool.Name == "gns3.exec":
		// gns3.exec 复用 server.exec_command 的白/黑名单过滤（只读命令集一致；
		// gns3_agent 侧二次过滤兜底，与 aranea twinops checkExecWhitelist 语义对齐）。
		return uc.filterCommand(stringParam(params, "cmd"))
```

- [ ] **Step 4.6 wire 装配 `GNS3AgentClient` 实现**

在 data 层新增 gns3_agent HTTP 客户端实现（参照既有 `opsToolClient` 实现模式）：

```bash
# 找到既有 opsToolClient 实现文件作为模板参照
cd f:/myproject/twinmonitor/TwinServer
grep -rn "OpsToolClient" app/aiops/internal/data/ | head -5
# 预期命中：app/aiops/internal/data/xxx_client.go 中的 NewXxxClient 构造函数
```

参照该文件新建 `app/aiops/internal/data/gns3_agent_client.go`，实现 `GNS3AgentClient` 接口（HTTP 客户端，`base_url` 从 `conf.Bootstrap.Clients` 新增 `gns3agent` 配置项读取，默认 `http://127.0.0.1:18081`），并在 `data.ProviderSet` 中注册。同时修改 `configs/config.yaml` 的 `clients:` 段追加：

```yaml
  gns3agent: { base_url: "http://127.0.0.1:18081", timeout_seconds: 30 } # GNS3 仿真演练平面（gns3_agent）
```

- [ ] **Step 4.7 wire 重新生成**

```bash
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/
# 预期：wire_gen.go 更新，无错误（项目记忆：wire_gen 参数不齐时须重生成）
```

- [ ] **Step 4.8 编译验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 4.9 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/clients.go app/aiops/internal/biz/mcp.go app/aiops/internal/biz/mcp_call.go app/aiops/internal/data/gns3_agent_client.go app/aiops/configs/config.yaml app/aiops/cmd/wire_gen.go
git commit -m "$(cat <<'EOF'
feat(aiops): gns3 域 4 个工具后端转发实现（gns3_agent HTTP 客户端）

- 新增 GNS3AgentClient 端口与 data 层实现
- executeTool 新增 gns3.health_check/exec/fault_inject/fault_clear 分支
- CallTool 写入 plane=gns3_sim 审计标记
- gns3.exec 复用命令白/黑名单过滤

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1 附录 A
EOF
)"
```

---

## Task 5：T5 `builtinMcpTools()` 新增 network 域 3 个工具 + `executeTool` 转发（08 线路监控）

**目标**：新增 `network.line_status` / `network.line_events` / `network.line_probe` 工具声明与后端转发，经 13 内部 HTTP 客户端调 08 线路监控 API（端点契约与 aranea 内置 twinops 工具一致：`/api/v1/monitor/linemonitor/lines*`、`/events`、`/probe-test`）。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_registry.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/clients.go`（新增 `LineMonitorClient` 端口）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp.go`（`McpUsecase` 注入）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_call.go`（`executeTool` 分支）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/configs/config.yaml`（新增 `linemonitor` 客户端配置）

- [ ] **Step 5.1 `clients.go` 新增 `LineMonitorClient` 端口**

```go
// LineMonitorClient 08 线路监控（127.0.0.1:8080 网关聚合）。
// 端点契约与 aranea 内置 twinops 工具一致（internal/tools/twinops/twinops.go）。
type LineMonitorClient interface {
	// GetLineRealtime 单条线路实时状态（GET /api/v1/monitor/linemonitor/lines/{id}/realtime）。
	GetLineRealtime(ctx context.Context, lineID uint32) (map[string]any, error)
	// ListLines 全部线路状态列表（GET /api/v1/monitor/linemonitor/lines?status=-1&page=&pageSize=）。
	ListLines(ctx context.Context, page, pageSize int) (map[string]any, error)
	// ListLineEvents 线路事件历史（GET /api/v1/monitor/linemonitor/events?lineId=&eventType=&status=&keyword=&page=&pageSize=）。
	ListLineEvents(ctx context.Context, params map[string]any) (map[string]any, error)
	// ProbeLine 主动触发一次线路探测（POST /api/v1/monitor/linemonitor/lines/{id}/probe-test）。
	ProbeLine(ctx context.Context, lineID uint32) (map[string]any, error)
}
```

- [ ] **Step 5.2 `McpUsecase` 注入 `lines` 字段**

参照 Task 4 Step 4.2 模式，在 `McpUsecase` 结构体与 `NewMcpUsecase` 中追加：

```go
	lines       LineMonitorClient // 可 nil
```

- [ ] **Step 5.3 `builtinMcpTools()` 追加 network 域 3 个工具声明**

在 gns3 域声明之后追加：

```go
		// ---- network 线路监控域（08 线路监控；line_probe 为主动探测有副作用，RiskLevel=low 非只读）----
		{Name: "network.line_status", Domain: "network", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "查询线路实时探测状态；传 line_id 查单条，不传返回全部线路状态列表",
			ParamsSchema: schema(nil, map[string]any{
				"line_id":   map[string]any{"type": "integer", "description": "线路 ID；不传返回全部"},
				"page":      map[string]any{"type": "integer", "default": 1},
				"page_size": map[string]any{"type": "integer", "default": 50},
			})},
		{Name: "network.line_events", Domain: "network", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "查询线路中断/恢复事件历史（outage/recovered），用于故障时间线取证",
			ParamsSchema: schema(nil, map[string]any{
				"line_id":    map[string]any{"type": "integer", "description": "线路 ID 过滤"},
				"event_type": map[string]any{"type": "string", "description": "事件类型过滤（outage/recovered）"},
				"status":     map[string]any{"type": "string", "description": "事件状态过滤（active/recovered）"},
				"keyword":    map[string]any{"type": "string", "description": "关键字过滤"},
				"page":       map[string]any{"type": "integer", "default": 1},
				"page_size":  map[string]any{"type": "integer", "default": 20},
			})},
		{Name: "network.line_probe", Domain: "network", RiskLevel: McpRiskLow, IsReadonly: 0,
			Description: "主动触发一次线路探测（不等探测周期），返回本次探测结果，用于处置后快速验证",
			ParamsSchema: schema([]string{"line_id"}, map[string]any{
				"line_id": map[string]any{"type": "integer", "description": "线路 ID"},
			})},
```

- [ ] **Step 5.4 `executeTool` 追加 network 域 3 个工具分支**

在 gns3 域分支之后追加：

```go
	// ---- network 线路监控域（08 线路监控）----
	case "network.line_status":
		if uc.lines == nil {
			return nil, fmt.Errorf("network 域工具未装配（LineMonitorClient 为 nil）")
		}
		if lineID := uint32Param(params, "line_id"); lineID > 0 {
			return uc.lines.GetLineRealtime(ctx, lineID)
		}
		page, pageSize := intParam(params, "page"), intParam(params, "page_size")
		if page <= 0 {
			page = 1
		}
		if pageSize <= 0 {
			pageSize = 50
		}
		return uc.lines.ListLines(ctx, page, pageSize)
	case "network.line_events":
		if uc.lines == nil {
			return nil, fmt.Errorf("network 域工具未装配（LineMonitorClient 为 nil）")
		}
		query := map[string]any{}
		for _, k := range []string{"line_id", "event_type", "status", "keyword", "page", "page_size"} {
			if v, ok := params[k]; ok {
				query[k] = v
			}
		}
		return uc.lines.ListLineEvents(ctx, query)
	case "network.line_probe":
		if uc.lines == nil {
			return nil, fmt.Errorf("network 域工具未装配（LineMonitorClient 为 nil）")
		}
		return uc.lines.ProbeLine(ctx, uint32Param(params, "line_id"))
```

- [ ] **Step 5.5 data 层实现 `LineMonitorClient` + wire 装配 + config.yaml 追加**

参照 Task 4 Step 4.6 模式，新建 `app/aiops/internal/data/line_monitor_client.go`，实现 `LineMonitorClient` 接口（HTTP 客户端，`base_url` 从 `conf.Bootstrap.Clients.LineMonitor` 读取，默认 `http://127.0.0.1:8080`），并在 `data.ProviderSet` 注册。`configs/config.yaml` 的 `clients:` 段追加：

```yaml
  linemonitor: { base_url: "http://127.0.0.1:8080", timeout_seconds: 30 } # 08 线路监控
```

wire 重新生成：

```bash
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/
```

- [ ] **Step 5.6 编译验证**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误
```

- [ ] **Step 5.7 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/mcp_registry.go app/aiops/internal/biz/clients.go app/aiops/internal/biz/mcp.go app/aiops/internal/biz/mcp_call.go app/aiops/internal/data/line_monitor_client.go app/aiops/configs/config.yaml app/aiops/cmd/wire_gen.go
git commit -m "$(cat <<'EOF'
feat(aiops): network 域 3 个工具声明与后端转发（08 线路监控）

- network.line_status / line_events=readonly；line_probe=low（主动探测有副作用）
- 新增 LineMonitorClient 端口与 data 层实现
- 端点契约与 aranea twinops 工具一致

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1 附录 A
EOF
)"
```

---

## Task 6：T6 `builtinMcpTools()` 新增 ops 域 3 个工具 + `executeTool` 转发（14/03/16）

**目标**：新增 `ops.remediation_status` / `ops.collector_status` / `ops.inspection_query` 工具声明与后端转发，分别调 14 执行记录、03 采集状态、16 巡检记录 API。

**Files:**
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_registry.go`
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/clients.go`（新增 `OpsQueryClient` 端口）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp.go`（`McpUsecase` 注入）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/internal/biz/mcp_call.go`（`executeTool` 分支）
- Modify: `f:/myproject/twinmonitor/TwinServer/app/aiops/configs/config.yaml`

- [ ] **Step 6.1 `clients.go` 新增 `OpsQueryClient` 端口**

```go
// OpsQueryClient 运维运营域聚合查询（14 执行记录 / 03 采集状态 / 16 巡检记录）。
// 端点契约与 aranea 内置 twinops 工具一致（internal/tools/twinops/twinops.go）。
type OpsQueryClient interface {
	// GetRemediationExecution 单条执行单详情（GET /api/v1/monitor/remediation/executions/{id}）。
	GetRemediationExecution(ctx context.Context, executionID uint32) (map[string]any, error)
	// ListRemediationExecutions 执行单列表（GET /api/v1/monitor/remediation/executions?status=&page=&pageSize=）。
	ListRemediationExecutions(ctx context.Context, params map[string]any) (map[string]any, error)
	// GetCollectorStatus 设备采集层状态（GET /api/v1/monitor/collector/devices/{id}/status）。
	GetCollectorStatus(ctx context.Context, deviceID uint32) (map[string]any, error)
	// ListCollectorFailures 未恢复采集失败记录（GET /api/v1/monitor/collector/devices/{id}/failures?unresolvedOnly=true）。
	ListCollectorFailures(ctx context.Context, deviceID uint32) (map[string]any, error)
	// ListInspectionRecords 巡检记录（GET /api/v1/monitor/opstools/inspection/records?keyword=&status=&taskId=&page=&pageSize=）。
	ListInspectionRecords(ctx context.Context, params map[string]any) (map[string]any, error)
}
```

- [ ] **Step 6.2 `McpUsecase` 注入 `opsQuery` 字段**

参照 Task 4 Step 4.2 模式追加：

```go
	opsQuery    OpsQueryClient // 可 nil
```

- [ ] **Step 6.3 `builtinMcpTools()` 追加 ops 域 3 个工具声明**

在 network 域声明之后追加：

```go
		// ---- ops 运维运营域（14 执行记录 / 03 采集状态 / 16 巡检记录，全只读）----
		{Name: "ops.remediation_status", Domain: "ops", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "查询故障处置执行单状态与日志摘要；传 execution_id 查详情，不传按状态列出",
			ParamsSchema: schema(nil, map[string]any{
				"execution_id": map[string]any{"type": "integer", "description": "执行单 ID；不传按状态列出"},
				"status":       map[string]any{"type": "string", "description": "执行单状态过滤（列表模式）"},
				"page":         map[string]any{"type": "integer", "default": 1},
				"page_size":    map[string]any{"type": "integer", "default": 20},
			})},
		{Name: "ops.collector_status", Domain: "ops", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "查询设备采集层状态（在线/连续失败次数/变更原因）与未恢复采集失败记录，区分设备故障与采集故障",
			ParamsSchema: schema([]string{"device_id"}, map[string]any{
				"device_id": map[string]any{"type": "integer", "description": "设备 ID"},
			})},
		{Name: "ops.inspection_query", Domain: "ops", RiskLevel: McpRiskReadonly, IsReadonly: 1,
			Description: "查询巡检记录（按关键词/结果/任务过滤），用于验证环节核对与复盘取证",
			ParamsSchema: schema(nil, map[string]any{
				"keyword":   map[string]any{"type": "string", "description": "关键词（匹配资产名/IP/摘要）"},
				"status":    map[string]any{"type": "string", "description": "按结果过滤（success/failed/partial）"},
				"task_id":   map[string]any{"type": "integer", "description": "按巡检任务 ID 过滤"},
				"page":      map[string]any{"type": "integer", "default": 1},
				"page_size": map[string]any{"type": "integer", "default": 20},
			})},
```

- [ ] **Step 6.4 `executeTool` 追加 ops 域 3 个工具分支**

在 network 域分支之后追加：

```go
	// ---- ops 运维运营域（14/03/16，全只读）----
	case "ops.remediation_status":
		if uc.opsQuery == nil {
			return nil, fmt.Errorf("ops 域工具未装配（OpsQueryClient 为 nil）")
		}
		if execID := uint32Param(params, "execution_id"); execID > 0 {
			return uc.opsQuery.GetRemediationExecution(ctx, execID)
		}
		query := map[string]any{}
		for _, k := range []string{"status", "page", "page_size"} {
			if v, ok := params[k]; ok {
				query[k] = v
			}
		}
		return uc.opsQuery.ListRemediationExecutions(ctx, query)
	case "ops.collector_status":
		if uc.opsQuery == nil {
			return nil, fmt.Errorf("ops 域工具未装配（OpsQueryClient 为 nil）")
		}
		deviceID := uint32Param(params, "device_id")
		status, err := uc.opsQuery.GetCollectorStatus(ctx, deviceID)
		if err != nil {
			return nil, err
		}
		failures, ferr := uc.opsQuery.ListCollectorFailures(ctx, deviceID)
		if ferr != nil {
			return nil, ferr
		}
		return map[string]any{
			"device_id":           deviceID,
			"collector_status":    status,
			"unresolved_failures": failures,
		}, nil
	case "ops.inspection_query":
		if uc.opsQuery == nil {
			return nil, fmt.Errorf("ops 域工具未装配（OpsQueryClient 为 nil）")
		}
		query := map[string]any{}
		for _, k := range []string{"keyword", "status", "task_id", "page", "page_size"} {
			if v, ok := params[k]; ok {
				query[k] = v
			}
		}
		return uc.opsQuery.ListInspectionRecords(ctx, query)
```

- [ ] **Step 6.5 data 层实现 `OpsQueryClient` + wire 装配 + config.yaml 追加**

参照 Task 4 Step 4.6 模式，新建 `app/aiops/internal/data/ops_query_client.go`，实现 `OpsQueryClient` 接口（三个子端分别走 `clients.gateway`/`opstool` 已有配置或新增 `opsquery` 配置项，按网关真实拓扑选择），并在 `data.ProviderSet` 注册。wire 重新生成：

```bash
cd f:/myproject/twinmonitor/TwinServer
go run github.com/google/wire/cmd/wire gen ./app/aiops/cmd/
```

- [ ] **Step 6.6 编译验证 + 冒烟测试**

```bash
cd f:/myproject/twinmonitor/TwinServer
go build ./app/aiops/...
# 预期：0 错误

# 启动 aiops 服务后通过 McpTesterPage 或 tools/list 验证 34 个工具全量返回
# 预期：tools/list 返回 34 个工具（原 24 + 新增 10），gns3 域 risk_level 正确
```

- [ ] **Step 6.7 git commit**

```bash
cd f:/myproject/twinmonitor/TwinServer
git add app/aiops/internal/biz/mcp_registry.go app/aiops/internal/biz/clients.go app/aiops/internal/biz/mcp.go app/aiops/internal/biz/mcp_call.go app/aiops/internal/data/ops_query_client.go app/aiops/configs/config.yaml app/aiops/cmd/wire_gen.go
git commit -m "$(cat <<'EOF'
feat(aiops): ops 域 3 个工具声明与后端转发（14/03/16 聚合查询）

- ops.remediation_status / collector_status / inspection_query 全只读
- 新增 OpsQueryClient 端口与 data 层实现
- 端点契约与 aranea twinops 工具一致

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.1 附录 A
EOF
)"
```

---

## Task 7：T7 aranea 侧登记 twinmonitor MCP-Server（`mcp_servers` 表 + SSE 配置）

**目标**：在 aranea 的 `mcp_servers` 表新增 `server_key="twinmonitor"` 记录，`ConfigJSON` 含 SSE URL 与调用凭据（`client_id`/`secret`），`MCPVersionHash` 按 server_key + ID + ConfigJSON 计算。

**Files:**
- Modify: `f:/myproject/aranea-agents/internal/data/ent/schema/platform_mcp_server.go`（确认 schema 无需改动，仅验证）
- Run: aranea HTTP API 或直接 SQL 插入（参照既有 `mcp_servers` 种子模式）

- [ ] **Step 7.1 确认 `mcp_servers` 表 schema 无需改动**

```bash
cd f:/myproject/aranea-agents
grep -n "server_key\|config_json\|sse" internal/data/ent/schema/platform_mcp_server.go
# 预期命中：server_key / config_json 字段已存在，无需新增列
```

- [ ] **Step 7.2 生成 twinmonitor MCP-Server 的 client_id/secret**

在 twinmonitor 13-aiops 侧通过 `POST /api/v1/aiops/mcp/clients` 创建专用调用方（`risk_cap=destructive`，工具子集不设限）：

```bash
curl -s -X POST http://localhost:8100/api/v1/aiops/mcp/clients \
  -H "Authorization: Bearer $TWIN_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "aranea-mcp-host",
    "description": "aranea 作为内部 MCP Host 消费 twinmonitor MCP-Server",
    "risk_cap": "destructive",
    "tool_scope": []
  }' | jq '{client_id, secret}'
# 预期：返回 client_id（如 mcp-a1b2c3d4e5f6）与 secret（64 字符 hex，仅此一次完整返回）
```

- [ ] **Step 7.3 aranea 侧插入 `mcp_servers` 记录**

```bash
# 通过 aranea Admin API 或 psql 直接插入（参照既有 mcp_servers 种子格式）
# ConfigJSON 结构参照 internal/mcp/config.ParseServerConfigJSON 的 SSE 配置契约
psql "$ARANEA_PG_DSN" -c "
INSERT INTO mcp_servers (id, server_key, name, description, status, enabled, sort_order, config_json, create_time, update_time)
VALUES (
  'twinmonitor-mcp-server',
  'twinmonitor',
  'TwinMonitor MCP-Server',
  '13-aiops MCP 能力面（SSE 通道），34 个内置工具（含 gns3/network/ops 新增域）',
  'active',
  true,
  100,
  '{\"url\":\"http://aiops:8100/mcp/sse\",\"headers\":{\"X-MCP-Client-ID\":\"<client_id>\",\"X-MCP-Client-Secret\":\"<secret>\"}}',
  now(),
  now()
) ON CONFLICT (server_key) DO UPDATE SET
  config_json = EXCLUDED.config_json,
  update_time = now();
"
# 预期：INSERT 0 1 或 UPDATE 1
```

> **注意**：`MCPVersionHash` 由 aranea 运行时按 `server_key + ID + ConfigJSON` 自动计算（项目既有规则），无需手动写入。

- [ ] **Step 7.4 验证 aranea MCP Host 能列出 twinmonitor 工具**

```bash
# 通过 aranea 的 MCP Host 调试接口或日志验证 SSE 连接与 tools/list
curl -s http://localhost:8000/api/v1/mcp/servers/twinmonitor/tools \
  -H "Authorization: Bearer $ARANEA_ADMIN_TOKEN" | jq '.tools | length'
# 预期：34（原 24 + 新增 10）
```

- [ ] **Step 7.5 git commit**

```bash
cd f:/myproject/aranea-agents
git add -A
git commit -m "$(cat <<'EOF'
feat(mcp): 登记 twinmonitor MCP-Server（SSE 通道）

- mcp_servers 表新增 server_key=twinmonitor 记录
- ConfigJSON 含 SSE URL 与调用凭据（client_id/secret）
- MCPVersionHash 按 server_key+ID+ConfigJSON 自动计算

Refs: docs/superpowers/specs/2026-08-19-aranea-twinmonitor-deep-fusion-design.md §3.1.2 P1
EOF
)"
```

---

## 验收清单（Sign-off）

- [ ] T1：`ai_mcp_call_history` 表新增 `plane` 字段，ent 生成物更新，编译通过。
- [ ] T2：`McpCallRecord` 与仓储层 `plane` 字段透传完整，编译通过。
- [ ] T3：`builtinMcpTools()` 新增 gns3 域 4 个工具声明，风险等级与总纲附录 A 一致。
- [ ] T4：`executeTool` 新增 gns3 域 4 个工具后端转发，`plane=gns3_sim` 写入调用历史，gns3_agent HTTP 客户端实现并 wire 装配。
- [ ] T5：network 域 3 个工具声明与转发完成（`line_probe` 为 low 非只读），LineMonitorClient 实现并 wire 装配。
- [ ] T6：ops 域 3 个工具声明与转发完成（全只读），OpsQueryClient 实现并 wire 装配。
- [ ] T7：aranea `mcp_servers` 表新增 `twinmonitor` 记录，MCP Host 能列出 34 个工具。
- [ ] 全局：`go build ./app/aiops/...`（twinmonitor）与 `go build ./cmd/... ./internal/...`（aranea）无编译错误。

---

## 发现的总纲与代码不一致之处

1. **gns3_agent 配置项缺失**：总纲 §3.1.1 提到 gns3 域后端是 gns3_agent，但 twinmonitor 13-aiops 的 `configs/config.yaml` 当前无 `gns3agent` 客户端配置项，需在 Task 4 Step 4.6 中新增（与既有 `asset`/`alarm`/`opstool` 等配置项并列）。
2. **`plane` 字段写入策略**：总纲只提到「审计记录含目标平面标记 `plane=gns3_sim`」，未明确其他域的 plane 值。计划按「gns3 域 → `gns3_sim`、只读工具 → `readonly`、其他 → `production`」三级标记实现，与总纲 GNS3 平面定位（演练/生产隔离）语义对齐。
3. **aranea 内置 twinops 工具的 gns3 端点契约**：总纲附录 A 写 gns3 域后端实现为「GNS3 控制器（gns3_agent）」，与 aranea 内置 `twinops` 工具的 gns3 端点（`GET /health`、`POST /exec`、`POST /fault/sw1-port`）完全一致，无需额外适配。
4. **network.line_probe 风险等级**：总纲附录 A 写 `network.line_probe` 为 `low` 非只读（主动探测有副作用），与 aranea 内置 `twin_line_probe` 的 `riskLevel: "medium"` 不一致。计划按总纲 `low` 实现（13 侧 `McpRiskLow`），因总纲为权威决策文档且 13 侧风险五级体系（readonly/low/medium/high/destructive）与 aranea 内置四级（low/medium/high/critical）语义不完全等价。
