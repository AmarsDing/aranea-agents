# M75: Computer Use（桌面 GUI 自动化控制）— 设计文档

> 编号：75 | 状态：已评审（2026-08-12） | 需求：75-computer-use.md
> 上游调研：[2026-08-12-research-computer-use.md](../reports/2026-08-12-research-computer-use.md)

## 0. 架构决策摘要（ADR）

| # | 决策 | 理由 | 代价 |
|---|------|------|------|
| ADR-CU-01 | sidecar + CDP 协议，非纯 Go 直连 UIA | 各平台用最强原生栈（UIA→.NET/FlaUI、AT-SPI→Python、WDA→macOS）；技术栈隔离；独立演进 | 多一层进程管理 |
| ADR-CU-02 | a11y-first + 视觉兜底混合 grounding | "快速精确"唯一工程解：a11y 零模型调用毫秒级 100% 精确；视觉仅补盲区 | 双路径维护 |
| ADR-CU-03 | Windows sidecar 用 .NET + FlaUI | UIA 绑定最成熟（FlaUI-MCP 已验证形态）；单文件发布 | 引入 C# 工具链 |
| ADR-CU-04 | iOS 仅承诺模拟器（P3） | 真机需 macOS+Xcode 签名链路，复杂度与收益不匹配（用户确认） | 真机不支持 |
| ADR-CU-05 | OmniParser 独立 HTTP 服务（标准组件） | Python/torch 栈隔离、GPU 弹性调度；许可不作选型约束（用户决策） | 部署多一个服务 |
| ADR-CU-06 | CDP 传输：P1 stdio JSON-RPC；P3 回环/局域网 HTTP | sidecar 子进程免端口管理、天然单机隔离；iOS 跨主机必须 HTTP | 两种传输实现 |

## 1. 总体架构

```
┌──────────────────────────── Aranea-Agents (Go) ────────────────────────────┐
│ Agent 运行时（Chat/Team）                                                    │
│   ↓ 工具调用                                                                │
│ internal/tools/computeruse/    5 个工具（Registry + seed + Assemble）        │
│   ↓ 依赖 biz Usecase                                                        │
│ internal/biz/computeruse/      会话/任务/预算/安全策略/grounding 编排 + port  │
│   ↓ port: DeviceGateway / VisionParser / AuditStore                         │
│ internal/computeruse/          sidecar 进程管理 + CDP stdio client +         │
│                                a11y 匹配器 + IoU 融合器 + SoM 标注器         │
│ internal/data/                 AuditStore 实现（Ent）                        │
└──────┬──────────────────────────────────────────────┬─────────────────────┘
       │ CDP（stdio JSON-RPC，P1）                     │ HTTP（可选，GPU 机）
┌──────▼──────────────┐                     ┌─────────▼───────────────┐
│ sidecar/aranea-cua-win│                     │ omniparserserver         │
│ .NET + FlaUI          │                     │ OmniParser V2 FastAPI    │
│ (P2: python / P3: wda)│                     └─────────────────────────┘
└───────────────────────┘
```

**依赖方向**：tools → biz → (port) ← internal/computeruse / internal/data。biz 不依赖 sidecar 具体实现。

## 2. CDP（Computer-use Device Protocol）契约

### 2.1 传输与帧

- P1：stdio JSON-RPC 2.0（sidecar 为 Go 核心拉起的子进程，stdin/stdout 通信，stderr 进进程日志）
- 每行一个 JSON 对象：`{"jsonrpc":"2.0","id":N,"method":"...","params":{...}}` / 响应 `{"id":N,"result":{...}}` 或 `{"id":N,"error":{"code":N,"message":"..."}}`
- 截图等二进制以 base64 内联于 result（P1 本机数据量可接受）
- 心跳：Go 核心每 5s 发 `device.ping`，连续 3 次超时判定僵死 → 看门狗终止重启

### 2.2 方法集

| 方法 | 参数 | 返回 | 说明 |
|------|------|------|------|
| `device.ping` | — | `{ok:true}` | 心跳 |
| `device.info` | — | `{platform, screen:{width,height,scaleFactor}, displays[]}` | 物理像素与 DPI |
| `perception.snapshot` | `{windowTitle?, includeScreenshot?, maxElements?}` | `{elements:UIElement[], screenshotRef?, generation:N}` | a11y 元素树（扁平化+ref） |
| `perception.screenshot` | `{region?:{x,y,w,h}, zoom?:float}` | `{pngBase64, width, height, scaleFactor}` | 物理像素 PNG |
| `action.invoke` | `{ref}` | `{ok, via:"invoke"}` | 元素级直调（无坐标） |
| `action.click` | `{x,y,button:"left|right|middle",clickCount}` | `{ok}` | 坐标级（物理像素） |
| `action.type` | `{text, intervalMs?}` | `{ok}` | 文本注入（SendInput Unicode） |
| `action.key` | `{combo:"ctrl+s"}` | `{ok}` | 组合键 |
| `action.wheel` | `{x,y,delta}` | `{ok}` | 滚轮 |
| `action.drag` | `{from:{x,y},to:{x,y},durationMs?}` | `{ok}` | 拖拽 |
| `window.list` | — | `{windows:[{hwnd,title,processName,isForeground,bounds}]}` | |
| `window.focus` | `{titleRegex 或 hwnd}` | `{ok, hwnd}` | |
| `app.launch` | `{target, args?, workDir?}` | `{ok, pid}` | target=路径或注册应用名 |

### 2.3 统一元素模型（UIElement）

```json
{ "ref": "g12.e42", "type": "button|edit|menuitem|text|icon|...",
  "name": "保存(S)", "bbox": {"x":100,"y":200,"w":80,"h":28},
  "interactivity": true, "source": "uia|atspi|wda|vision",
  "appName": "notepad.exe", "enabled": true }
```

- `ref` = `g{generation}.e{index}`：仅在同一 snapshot generation 内有效；grounding 与 invoke 必须携带同代 ref，跨代自动重新感知（防错位点击）
- `bbox` 一律物理像素

### 2.4 错误码

| code | 含义 |
|------|------|
| -32001 | 元素未找到 / ref 过期 |
| -32002 | 目标窗口失焦/元素不可交互 |
| -32003 | OS 级注入被拒绝（权限/安全桌面） |
| -32004 | sidecar 内部错误 |

## 3. 后端分层设计

### 3.1 biz 层（`internal/biz/computeruse/`）

**领域模型**：`Session{ID, AgentKey, Status, Budget{MaxSteps, Deadline}, StepsUsed}`、`Step{ID, SessionID, Target, Path(a11y|vision), Action, Result, DurationMs}`、审计记录 `AuditEntry`。

**状态机（AS-FSM-01）**：`session_state_machine.go`
```
idle → observing → grounding → acting → done
  ↘ awaiting_confirm ↗（确认门）   ↘ failed / cancelled(急停/超预算)
```
补充转换（M1.5 审查 B3 后补全）：`EvFinish`（用户中途结束）从任意非终态直达 `done`；`EvFail` 从 `idle` 可达（chargeBudget 预算预检失败）；终态不可再转换，会话复用由 Usecase 重建 idle 处理。

**Port 接口**（窄接口 + Stability 标注）：

```go
// Stability:evolving
type DeviceGateway interface {          // 与 sidecar 通信
    Info(ctx) (DeviceInfo, error)
    Snapshot(ctx, SnapshotOpts) (Snapshot, error)
    Screenshot(ctx, region, zoom) (Image, error)
    Invoke(ctx, ref) error
    Click(ctx, p Point, btn string, n int) error
    TypeText(ctx, text string) error
    Key(ctx, combo string) error
    FocusWindow(ctx, titleRegex string) error
    Launch(ctx, target string) (int, error)
} // 注：超 5 方法，按 DB-N3 拆分为 DevicePerceiver / DeviceActor 两个窄接口，DeviceGateway 仅作组合

// Stability:evolving
type VisionParser interface {           // OmniParser / VLM 直判统一抽象
    Available(ctx) bool
    Parse(ctx, img Image) ([]UIElement, error)   // OmniParser
}
// Stability:evolving
type VisionGrounder interface {         // VLM 语义定位
    Pick(ctx, img Image, candidates []UIElement, target string) (ref string, err error)
}
// Stability:evolving
type AuditStore interface {             // 审计落库（data 层实现）
    RecordStep(ctx, AuditEntry) error
    ListSteps(ctx, sessionID string) ([]AuditEntry, error)
}
```

**Usecase**：`ComputerUseUsecase`
- `Observe(ctx, agentKey, opts)` → 快照（走 DeviceGateway，免确认）
- `Act(ctx, req{AgentKey, Target, Action, Args, DryRun})` → grounding 编排（§3.3）→ 执行 → 审计 → 发事件
- `Launch / FocusWindow`（需确认）
- `StartSession / StopSession / KillSwitch(ctx, sessionID)`
- 安全策略执行点：敏感词表、进程禁区、步数预算、干跑——全部在 Usecase 层强制

### 3.2 基础设施层（`internal/computeruse/`）

| 文件 | 职责 |
|------|------|
| `process.go` | sidecar 子进程拉起/看门狗/自动重启（心跳 5s×3）；K7 进程日志 |
| `client.go` | CDP stdio JSON-RPC client（id 复用、并发 mux、调用超时 10s/动作 30s） |
| `gateway.go` | 实现 biz.DeviceGateway（组合 client） |
| `matcher.go` | a11y 模糊匹配器：归一化（小写/去空白/去标点/全半角）→ 精确 → 包含 → 编辑距离 ≤2；候选打分，top1 与 top2 分差 ≥0.2 才判命中 |
| `fusion.go` | IoU 融合器：a11y 主表 + vision 补充表，IoU>0.1 去重（UFO² 算法 H2） |
| `som.go` | SoM 标注器：截图上为候选元素绘制编号框（Go image/draw，ASCII 编号）；`DownscalePNG` 截图降采样（最长边 ≤1568，x/image ApproxBiLinear） |
| `omniparser.go` | VisionParser HTTP 客户端：POST {base64} → parsed_content[] → UIElement[]（source=vision）；`Available()` 健康检查 + 失败降级标记 |
| `vlm.go` | VisionGrounder：经 LLM catalog 取多模态模型，SoM 图 + 候选列表 + target → 返回编号；发送图降采样 ≤1568px（prompt bbox 同比例缩放）；单次调用超时 60s（容忍本地模型冷启动）；**无匹配出口**：SoM 选编号输出 0 / 坐标直判输出 "-1, -1" → 明确 ErrGroundingFailed，禁止强制乱选/乱点 |

### 3.3 Grounding 决策流（Act 编排）

```
Act(target)
 1. snapshot := gateway.Snapshot()            （含 generation）
 2. hit := matcher.Match(snapshot.elements, target)
    命中 → gateway.Invoke(hit.ref)            【a11y 快路径，path=a11y】
 3. miss → SoM 视觉兜底（OmniParser 可用时，path=vision）：
    a. img := gateway.Screenshot()
    b. visionEls := omniparser.Available() ? omniparser.Parse(img) : []
    c. merged := fusion.Merge(snapshot.elements, visionEls)   （IoU>0.1）
    d. som := som.Annotate(img, merged) → 降采样 ≤1568px
    e. ref := vlm.Pick(som图, merged, target)  （VLM 输出 0 = 无匹配 → 继续降级）
 4. SoM 失败 → VLM 坐标直判（path=vlm_direct，免 OmniParser 最低精度路径）：
    全屏粗判（归一化千分位坐标）→ 以粗判点为中心 480x360 区域 2x zoom 精判
    （VLM 输出 "-1, -1" = 无匹配 → ErrGroundingFailed 明确报错，不乱点）
 5. 命中 → 执行（invoke/click/type/key）；dry_run 只 grounding + 返回计划
 6. 执行后验证：settle → re-snapshot → 元素树 hash 比对 + 前台窗口检查，
    结果透出 verify{changed, foreground_before/after}（no_effect 供 LLM 决策）
 7. 批量：actions[] 按序 fail-fast，任一步失败即停且错误注明已完成步数
 8. 全程：step 审计落库 + envelope 事件 + 流程日志
```

### 3.4 工具集（`internal/tools/computeruse/`）

| 工具 key | 名称 | RequiresConfirmation | 说明 |
|---------|------|---------------------|------|
| `computer_use.observe` | 桌面感知 | false | 元素树摘要（LLM 可读文本）+ 可选截图 |
| `computer_use.screenshot` | 桌面截图 | false | 返回多模态图像内容 |
| `computer_use.act` | 桌面动作 | **true** | target 语义描述 + action 类型 + args；高危词强制人工 |
| `computer_use.launch` | 启动应用 | true | target=应用名/路径 |
| `computer_use.session` | 会话管理 | false | action=start\|stop\|status，绑定预算 |

- 注册：`toolset.go` Registry() 追加 + `internal/data/builtin_tools_seed.go` 种子（分类 `computeruse`，Tags `["desktop","gui","automation"]`，RiskLevel=high for act/launch）
- 工厂经 `AssemblyConfig` 新增 `ComputerUse *bizcomputeruse.ComputerUseUsecase` 字段装配
- 确认门：复用现有 tool-grants；敏感词命中（`tool_confirm_gate.go` `computerUseDangerHit`，仅 act/launch 两工具检查入参）时决策链短路为 `policy_danger`——**强制逐次确认，持久/会话授权不生效**；确认卡 Step.Danger=true 渲染「高危」徽标，且前端仅显示「允许本次/拒绝」两按钮（需求 §5.3）

### 3.5 安全模型实现点

| 机制 | 实现位置 |
|------|---------|
| 确认门 | 工具 RequiresConfirmation + 确认门 danger 短路（`computerUseDangerHit` 命中敏感词时绕过授权链）+ Usecase 内 `Policy.IsDanger` 标记审计/事件 |
| 敏感词表 | `internal/biz/computeruse/policy.go` 内置默认表（删除/支付/转账/发送/确认支付/格式化/永久删除…），配置可覆盖 |
| 进程禁区 | sidecar `window.list` 前台进程名 ∈ 黑名单（keepass/1password/银行 U 盾…）→ act 拒绝 |
| 预算 | Session.Budget，Act 原子自增 StepsUsed，超限返回预算错误 |
| 干跑 | req.DryRun=true → 只执行 grounding + SoM 标注返回计划，不注入 |
| 急停 | `KillSwitch`：context cancel + sidecar 动作队列清空；前端按钮 → API |

### 3.6 数据模型

**`computer_use_audit` 表**（Ent Schema `ComputerUseAudit`，L1 自动迁移，`entsql.Annotation{Table:"computer_use_audit"}`）：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 | 主键 |
| session_id | string | 会话 |
| agent_key | string | Agent |
| step_index | int | 步序号 |
| target | text | 目标描述 |
| path | enum(a11y/vision/vlm_direct) | grounding 路径 |
| action | string | invoke/click/type/key/launch… |
| params | json | 动作参数 |
| result | enum(ok/retry/failed/cancelled) | |
| error | text, 可空 | |
| duration_ms | int64 | |
| confirmed_by | string, 可空 | 确认人（确认门） |
| danger | bool | 高危标记 |
| screenshot_ref | string, 可空 | 截图存储引用（文件路径） |
| created_at | time | |

### 3.7 事件与日志

- **实时事件**：`computeruse.step` MonitorEvent（`internal/event/contract/monitor_event.go`，ADR-03 双总线迁移后不再新增 Envelope 类型）。payload=Step 摘要（Metadata：step_index/target/path/action/result/duration_ms/danger/confirmed_by/error）；发布端口 `bizcu.StepEventPublisher`，适配器 `internal/computeruse/step_events.go` 走 MonitorBus → WS monitor pump。可靠性 Informational——持久化以 `computer_use_audit` 表为准（每步同步落库）
- **流程日志 step_id**（登记 stepTitleRegistry + 52 号文档 §5.1；domain=computeruse 已注册 TraceDomain 并接入 `domainForStepID`）：
  `computeruse.session.start` / `computeruse.session.done` / `computeruse.act` / `computeruse.grounding.fallback`（降级 warn）/ `computeruse.act.done` / `computeruse.act.error` / `computeruse.budget.exceeded` / `computeruse.killswitch`
- **进程日志**：loggateway 构造注入；K7：sidecar 启停/panic/看门狗各一条；K2 错误含 `loggateway.Err`

### 3.8 前端设计（P1 最简视图）

- `web/src/features/computeruse/`：步骤流组件 `CuStepStream.vue`（订阅 WS monitor 通道的 `computeruse.step` MonitorEvent）
- 聊天气泡内嵌：`TurnContainer.vue` 在 **turn.Status=running** 且 steps 中含 computeruse 会话（`cuSessionIdFromSteps` 提取 ToolResult.session_id）时，气泡尾部渲染 CuStepStream——步骤卡片（目标/路径徽标/耗时/结果/失败原因）；头部急停按钮 → `POST /v1/computeruse/sessions/{id}/kill`。历史 turn 不渲染（急停对死会话无意义，审计回放走监控页 steps API）
- **容器白名单（M1.5 审查 S1）**：CuStepStream 仅允许嵌入 TurnContainer（聊天气泡尾部）；监控页等其他场景复用前须过 UX 评审（实时事件流+急停按钮的展示语境与气泡内不同）
- **TECH-DEBT（M1.5 审查 S2）**：步骤流仅消费 WS 实时事件，页面刷新后不回放（审计数据仍可经 ListComputerUseSteps 查询）；后续补齐 REST 回补时须复用 `cuStepFromMonitorEvent` 字段映射口径（含 danger/confirmed_by），避免双份映射漂移
- ToolsPage 自动展示新工具（种子分类 computeruse，分类筛选项含 computeruse）

### 3.9 API（service 层）

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/computeruse/sessions/{id}/kill` | POST | 急停 |
| `/v1/computeruse/sessions/{id}/steps` | GET | 审计步骤查询（监控页） |
| `/v1/computeruse/status` | GET | sidecar/OmniParser 健康状态 |

Proto：`api/kratos/computeruse/v1/computeruse.proto`（`make api` 生成；服务实现走 service 层调 biz Usecase）。

## 4. 技术选型

| 组件 | 选型 | 备选（未选原因） |
|------|------|----------------|
| Windows sidecar | .NET + FlaUI 4.x（目标 net8.0-windows；SDK 不可用时 net7.0-windows） | pywinauto（绑定质量/部署）；Go+go-ole COM（工作量/风险） |
| CDP 传输 | stdio JSON-RPC 2.0 | gRPC（子进程场景过重） |
| a11y 匹配 | 自研归一化+编辑距离 | 外部模糊库（需求简单） |
| 视觉解析 | OmniParser V2（omniparserserver FastAPI，独立部署；本机 GPU `:8101`——8100 被本机常驻进程占用；HF 离线权重预置 `bin/cua/omniparser/`，启动脚本 `start_omniparser.ps1`） | 纯 VLM 直判（精度不足，作为降级路径保留） |
| VLM | 复用 LLM catalog 多模态模型：本地 `ollama/qwen2.5vl-cua`（qwen2.5vl:7b 派生，Modelfile 固化 num_ctx=8192——Ollama 默认 4096 放不下全屏 SoM 请求）+ 云端 `alibaba-cn/qwen3-vl-plus`（catalog 已建行，待 API key 启用） | 自部署 UI-TARS（后续演化方向） |
| SoM 标注 | Go image/draw 自绘编号框 | sidecar GDI（保持 sidecar 精简） |
| 审计存储 | Ent Schema `computer_use_audit`（L1） | DDL 迁移（无索引特性需求，免） |

## 5. 风险与对策

| 风险 | 等级 | 对策 |
|------|------|------|
| 无 GPU 机器 OmniParser 8-15s/帧 | 高 | 不进关键路径；默认 VLM 直判路径；OmniParser 指远程 GPU |
| UIA 盲区（Electron/自绘） | 中 | 视觉兜底 + zoom 裁剪；后续 quirks 记忆 |
| DPI/多屏坐标错乱 | 中 | sidecar manifest per-monitor V2 aware；协议统一物理像素 |
| sidecar 崩溃僵死 | 中 | 心跳看门狗 + 自动重启 + 任务失败回落 |
| 误操作真实损害 | 高 | 确认门/敏感词/禁区/预算/干跑/急停全量；评测用 VM 快照 |
| ref 跨代错位点击 | 中 | generation 校验，跨代强制重新感知 |

## 6. 与其他模块的关系

| 模块 | 关系 |
|------|------|
| tools 装配（场景 2） | Registry+seed+Assemble 五步；Chat/Team 双生效 |
| 确认门 tool-grants | act/launch 走既有授权链，grant_persisted 短路 |
| event/monitor | 新增 computeruse.step MonitorEvent 类型（contract/monitor_event.go）+ StepEventPublisher 适配器 |
| LLM catalog | VLM 模型配置复用，不另建模型栈 |
| MCP 管理（P3） | iOS sidecar 以 MCP server 托管 |
| 语音伴侣（M74） | 语音任务可调用 computer_use 工具控制桌面（自然延伸，无代码耦合） |
