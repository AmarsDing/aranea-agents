# M75: Computer Use（桌面 GUI 自动化控制）— 开发计划

> 编号：75 | 状态：实施中 | 需求：75-computer-use.md | 设计：75-computer-use.design.md

## 1. 模块定位

为 Agent 提供本机桌面 GUI 感知与操作能力（P0=Windows），以 sidecar+CDP 架构集成进工具装配，a11y 优先混合 grounding 保证"快速精确"。

## 2. 代码锚点

### 2.1 现有复用锚点（已验证存在）

| 锚点 | 用途 |
|------|------|
| `internal/tools/toolset.go` Registry() | 工具注册入口 |
| `internal/tools/tool.go` ToolRegistration | 注册结构（RequiresConfirmation/Factory） |
| `internal/data/builtin_tools_seed.go` | 种子数据 |
| `internal/event/envelope.go` EnvelopeType 常量块 | 新增 `computeruse.step` |
| `internal/event/flow_log.go` stepTitleRegistry | 流程日志步骤登记 |
| `pkg/loggateway` Logger | 进程日志（构造注入） |
| 确认门 tool-grants | act/launch 授权链复用 |
| LLM catalog / Provider 体系 | VLM 模型配置复用 |
| `internal/data/ent/schema/` | 新增 computer_use_audit |

### 2.2 新增锚点（本模块）

| 路径 | 说明 |
|------|------|
| `internal/biz/computeruse/` | 领域模型 + Usecase + port + 状态机 + 安全策略 |
| `internal/computeruse/` | sidecar 进程管理、CDP client、gateway、matcher、fusion、som、omniparser、vlm |
| `internal/computeruse/sidecar/aranea-cua-win/` | C# sidecar 源码（.NET + FlaUI） |
| `internal/tools/computeruse/` | 5 个工具注册 |
| `internal/data/computeruse_audit_repo.go` | AuditStore 实现 |
| `internal/data/ent/schema/computer_use_audit.go` | Ent Schema |
| `api/kratos/computeruse/v1/computeruse.proto` | API 契约 |
| `internal/service/computeruse*.go` | service 层 |
| `web/src/features/computeruse/` | 前端步骤流（P1 最简） |
| `bin/cua/` | sidecar 编译产物输出（gitignore，不入库） |

## 3. 现状评估与差距

从零新建模块。平台已有：工具装配五步流程、确认门、事件总线、双轨日志、LLM catalog——全部复用。差距=全部本模块代码（sidecar/Go 核心/工具/API/前端）。

## 4. Phase 划分与任务清单

### Phase M1.1 — Windows sidecar（CDP + FlaUI）✅

| # | 任务 | 验收 |
|---|------|------|
| 1 | 建 C# 工程（internal/computeruse/sidecar/aranea-cua-win/），FlaUI 引用，单文件发布到 bin/cua/ | dotnet build 绿 |
| 2 | stdio JSON-RPC 帧循环 + device.ping/device.info | 手工 echo 测试通 |
| 3 | perception.snapshot（UIA 遍历→UIElement[]，ref 生成，DPI 物理像素）+ perception.screenshot | 对记事本 snapshot 含"文件"菜单等元素 |
| 4 | action.invoke/click/type/key/wheel/drag + window.list/focus + app.launch | 驱动记事本：打开→输入→Ctrl+S |
| 5 | sidecar manifest per-monitor DPI aware；stderr 诊断日志 | 125% 缩放下坐标一致 |

### Phase M1.2 — Go 核心 + 工具注册 ✅

| # | 任务 | 验收 |
|---|------|------|
| 1 | biz/computeruse：模型/状态机/port/Usecase（Observe/Act/Launch/Session/KillSwitch）+ 安全策略（敏感词/禁区/预算/干跑） | TDD 单测绿 |
| 2 | internal/computeruse：process/client/gateway/matcher | TDD 单测绿（mock sidecar stdio） |
| 3 | tools/computeruse 5 工具 + Registry + seed + AssemblyConfig 装配 | `go test ./internal/tools/...` 绿；种子含 5 条目 |
| 4 | service 层 + proto（kill/steps/status） | `make api && make wire && make build` 绿 |

### Phase M1.3 — 视觉兜底 ✅

| # | 任务 | 验收 |
|---|------|------|
| 1 | fusion.go IoU 去重 + som.go 标注器 | 单测绿 |
| 2 | omniparser.go HTTP 客户端 + Available 健康检查 + 降级标记 | 单测绿（httptest） |
| 3 | vlm.go VisionGrounder（catalog 多模态调用）+ Act 编排接入视觉路径 | 单测绿（mock VLM）；自绘窗口场景命中率验证 |

### Phase M1.4 — 安全/审计/观测/前端 ⏳

| # | 任务 | 验收 |
|---|------|------|
| 1 | Ent Schema computer_use_audit + AuditStore repo | `go generate` + 迁移绿 |
| 2 | envelope computeruse.step + 流程日志 step 登记 + 双文档同步 | 事件五步全做；52 号文档 §5.1 同步 |
| 3 | 安全门全链路（确认卡 danger 标记/禁区/预算/急停 API） | 验收 A5-A8 |
| 4 | 前端 CuStepStream 最简视图 + 急停按钮 + ToolsPage 展示 | `pnpm lint && pnpm test && pnpm build` 绿 |

### Phase P2 — Linux sidecar（后续迭代）
### Phase P3 — iOS 模拟器（macOS 宿主 WDA + MCP 托管，后续迭代）

## 5. 总验收标准

按需求文档 §3 A1-A10 全量；另加：全量 `make api && make wire && make build && make test && make lint` 绿；前端三件套绿；真机演示 A1 通过（日志+UI 双重验证，遵守 R3）。

## 6. 改动文件清单（预估）

见 §2.2 新增锚点；修改锚点：`internal/tools/toolset.go`（Registry 追加）、`internal/tools/toolset_assemble.go`（装配）、`internal/data/builtin_tools_seed.go`（种子）、`internal/event/envelope.go`（新类型）、`internal/event/flow_log.go`（step 登记）、`internal/service/`（proto 实现）、`docs/development/52-flow-logger.design.md`（§5.1 同步）、`AGENTS.md`（如需登记 bin/cua 产物约定）。

## 7. 风险与对策

见设计文档 §5。实施纪律：TDD（先失败测试）；每 Phase 完成跑分级验证；不顺带改无关模块；发现问题回退设计。
