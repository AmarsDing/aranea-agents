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
| `internal/event/contract/monitor_event.go` MonitorEventType 常量块 | 新增 `computeruse.step`（ADR-03 后不再新增 Envelope 类型） |
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

### Phase M1.4 — 安全/审计/观测/前端 ✅

| # | 任务 | 验收 | 状态 |
|---|------|------|------|
| 1 | Ent Schema computer_use_audit + AuditStore repo | `go generate` + 迁移绿 | ✅ |
| 2 | computeruse.step MonitorEvent + 流程日志 step 登记 + 双文档同步 | 事件链路全做（contract 类型/Publisher 适配器/MonitorBus 装配/WS pump 直达/前端订阅在任务 4）；52 号文档 §5.1 已同步；TraceDomainComputerUse + domainForStepID 已注册 | ✅ |
| 3 | 安全门全链路（确认卡 danger 标记/禁区/预算/急停 API） | 验收 A5-A8：danger 强制逐次确认（授权链短路，tool_confirm_gate_computeruse_test.go 绿）；确认卡 Danger=true 仅「允许本次/拒绝」两按钮（ConfirmBlock.spec.ts 绿）；禁区拒绝含原因（ErrBlockedProcess+进程名）；预算超限自动终止+流程日志；急停 API+前端按钮 | ✅ |
| 4 | 前端 CuStepStream 最简视图 + 急停按钮 + ToolsPage 展示 | `pnpm test`（1735 绿）+ `pnpm build` 绿；TurnContainer 运行中 turn 内嵌步骤流（cuSessionIdFromSteps）；ToolsPage 分类筛选含 computeruse；i18n keys 已注册 zh/en（pnpm lint 本模块文件干净，全局 lint 基线漂移归并行会话） | ✅ |

### Phase M1.5 — 三路并行审查修复（后端 Go / 前端 / C# sidecar）✅

3 个并行 review sub-agent 按 SKILL 维度审查 M75 全量变更集，结论与修复如下（全部 TDD：先失败测试再修复）。

#### 后端阻断项（biz/computeruse）

| # | 问题 | 修复 | 回归测试 |
|---|------|------|---------|
| B1 | Session 字段锁外读写（数据竞态） | 所有 Session 字段访问收敛到 `u.mu` 锁内；新增 `statusOf()` 锁内读状态；`chargeBudget` 锁内原子占用并返回已用步数 | usecase_test.go B1 回归 |
| B2 | Failed 会话永久阻塞 Agent（终态未解除 activeByAgent 映射） | `transitionLocked` 进终态（done/failed/cancelled）时删除 `activeByAgent[AgentKey]`，下一次 Act 自动重建会话 | `TestAct_FailedSessionDoesNotBlockAgent` |
| B3 | 状态直赋绕过 Transition | 全部状态变更走 `transitionLocked`；状态机补 Idle→EvFail→Failed、各态→EvFinish→Done 合法转换 | usecase_test.go B3 回归 |

#### C# sidecar 阻断项（aranea-cua-win）

| # | 问题 | 修复 | 回归测试 |
|---|------|------|---------|
| F1 | `zoom` 参数被静默忽略（截图未缩放，dense UI 局部放大失效） | `CaptureService.Screenshot` 实现 `ScaleBitmap`（HighQualityBicubic，≥1x1）；`JsonRpc.Screenshot` 解析 zoom；`Gateway.Screenshot` 传递 zoom | `TestGatewayScreenshotWithZoom` + C# 侧单测 |
| F2 | `includeScreenshot` 参数被静默忽略（snapshot 不返回内联截图） | `SnapshotResultDto.Screenshot` 可空属性；`JsonRpc.Snapshot` 解析 includeScreenshot 并内联主屏截图；`Gateway.Snapshot` 解码进 `Snapshot.Screenshot`（biz 模型新增该字段） | `TestGatewaySnapshotWithScreenshot` |
| F3 | 看门狗假僵死：sidecar 单线程顺序执行，长动作在途时无法应答 ping，误判僵死误杀合法长动作 | `Client.InFlight()` 暴露未完成请求数；看门狗在 InFlight>0 时跳过本轮 ping 且不计 miss（在途请求自身 10s/30s 超时兜底） | `TestManagerWatchdogSkipsWhileInFlight` |

#### 前端建议项

| # | 问题 | 修复 | 回归测试 |
|---|------|------|---------|
| S1 | CuStepStream 使用场景未约束，存在滥用风险 | TurnContainer.vue 加容器白名单注释：仅允许聊天气泡尾部内嵌，监控页等场景复用前须过 UX 评审 | 注释约束 |
| S2 | 步骤流仅消费 WS 实时事件，页面刷新后不回放 | useCuStepStream.ts 加 TECH-DEBT 标注：REST 回补未实现，回补时须复用 `cuStepFromMonitorEvent` 字段映射口径（含 danger/confirmed_by） | 标注约束 |
| S3 | REST 重载路径丢 danger 标记（页面刷新后 confirm 卡高危徽标丢失） | `session.proto` StepV2 加 `danger=23`；`bizStepToProto` 映射；前端 `v2Api.ts` StepV2Dto/mapStep 映射（WS 路径此前已带） | `TestSessionV2Service_ListSteps_DangerMapping` + v2Api.spec.ts |

### Phase M2 — 对标市场最佳的能力增强（2026-08-13）✅

> 背景：按 `docs/reports/2026-08-12-research-computer-use.md` 二次对标评估选定「方案 A：混合架构增强」——保留 a11y 快路径，补齐视觉链路（本地+云端 VLM、OmniParser 本机 GPU）、执行后验证闭环与批量动作。评估结论已回写该报告附录。

| # | 任务 | 验收 | 状态 |
|---|------|------|------|
| 1 | 工具面放开：`computer_use_*` 入 `registryOptInOnlyKeys` + spirit profile `group:computeruse`；存量库 reseed 迁移 `20261208 builtin_platform_tools_cua_reseed`（种子幂等） | spirit 生效工具集含 computeruse 组 | ✅ |
| 2 | VLM Grounder 模型配置：本地 `ollama/qwen2.5vl-cua`（qwen2.5vl:7b 派生 num_ctx=8192，修复 Ollama 默认 4096 上下文超限）+ 云端 `alibaba-cn/qwen3-vl-plus` catalog 建行 | catalog 视觉模型启用 | ✅（云端待用户提供 dashscope API key 后启用） |
| 3 | OmniParser V2 本机 GPU 部署（HF 离线权重 + venv + `:8101`，8100 被占改端口；懒加载 PaddleOCR/关 reload 控显存） | `start_omniparser.ps1` 一键起服，`Available()` 通过 | ✅ |
| 4 | grounding fallback 链补全：SoM 失败降级 VLM 坐标直判（归一化千分位 + 480x360@2x zoom 精化） | TDD 单测绿（PathVLMDirect） | ✅ |
| 5 | 执行后验证闭环：settle + re-snapshot + 元素树 hash + 前台窗口检查，verify 透出 | TDD 单测绿 | ✅ |
| 6 | `computer_use.act` 批量动作 `actions[]`：按序 fail-fast、错误注明已完成步数、逐步审计 | TDD 单测绿 | ✅ |
| 7 | 运行期加固（E2E 驱动）：VLM 超时 30s→60s（冷启动容忍）；发送图降采样 ≤1568px（prompt bbox 同比例缩放）；**无匹配出口**（SoM 输出 0 / 直判输出 -1,-1 → 明确报错，禁止强制乱点） | TDD 单测绿 | ✅ |
| 8 | 记事本 E2E 运行时验证（`test/cua-reseed`，真实 sidecar+OmniParser+Ollama） | launch/observe/act-type(a11y)/som-dryrun(vision 命中"红叉→关闭")/act-batch(2步)/grounding-miss(双无匹配出口) 全 PASS | ✅ |

E2E 关键证据（2026-08-13 运行）：`act-type path=a11y verify.Changed=true`；`som-dryrun path=vision ResolvedName=关闭 @2863,571`（语义目标"窗口右上角的红叉"经 OmniParser+SoM+qwen2.5vl-cua 正确解析）；`grounding-miss` 经 SoM「VLM 判定无匹配元素」降级直判「无匹配」后明确报错。

**E2E 排障沉淀**：①多次失败运行泄漏的 notepad.exe 会造成全屏 a11y 树同名元素歧义（matcher top1/top2 分差 <0.2 拒绝命中）——测试前须清理残留实例；②RTX 2080 Ti 11GB 同时承载 OmniParser(Florence2/YOLO) 与 qwen2.5vl-cua(5.9GB, 100% GPU) 可行，但首次 VLM 调用含模型加载，须预热或容忍 60s 超时；③Ollama OpenAI 兼容端点不透传 num_ctx，派生 Modelfile 是唯一稳妥的上下文扩容手段（对生产 catalog 路径同样生效）。

### Phase M2.5 — M2 审查修复（2026-08-13）✅

按 `aranea-review` SKILL 全维度复审 M2 变更集：无阻断项；4 条建议（F1-F4）全部 TDD 修复，5 条提示逐条处置。

| # | 发现 | 修复 | 回归测试 |
|---|------|------|---------|
| F1 | vlm_direct 粗判坐标在 DPI≠100% 显示器误点（sidecar PerMonitorV2 截图已是物理像素，却又除 ScaleFactor 二次换算） | `directGround` 删除二次换算；设计文档 §3.3 固化坐标语义 | `TestAct_VLMDirectScaledDisplay` |
| F2 | grounding 降级（设计内回退，K3）误用 error 级流程日志 | `biz.FlowLogWriter` 扩展 `LogFlowWarn`；adapter + 全部 fakes 同步；降级点切换 warn 级 | `TestAct_GroundingFallbackLogsWarn` |
| F3 | 并发 Act 双计费后一者 transit 失败 → 预算泄漏 | `beginStep` 同锁原子完成「忙/终态检查 + StepsUsed++ + 状态转换」（替代 chargeBudget 两步式）；提取 `rejectBudgetStep` 顺带使 actOne ≤80 行 | `TestBeginStep_ConcurrentNoDoubleCharge` |
| F4 | SoM 编号正则只取数字，"-1" 无匹配哨兵被提取为候选 1 误选 | `vlmNumberPattern` 允许负号 + `parseVLMNumber` 判负明确失败 | `TestVLMGrounderPick_NegativeSentinel` / `TestParseVLMNumber_Negative` |

提示处置：①actOne 行数超标 → F3 顺带解决；②预算耗尽时被拒审计步 Index 撞号 → F3 顺带解决（Index=stepsUsed+1 单调不撞号，`TestAct_BudgetExceededRejectedStepIndex`）；③verifyAfterAction settle 不感知急停取消 → 接受（400ms 有界延迟，急停最多多等一拍，设计文档 §3.3 已注明）；④resolveVisionLLM 每次 List + 首项优先 → 接受（仅 a11y 未命中的视觉兜底路径触发、低频，优先级由 catalog 排序控制，设计文档 §3.2 已注明）；⑤迁移 20261208 对已存在行不更新 schema → 接受（实测部署库 `computer_use_act` 已含 actions[]；结构性注意已录入设计文档 §3.4——未来 computer_use_* schema 变更走 catalog-patch 迁移）。

改动文件追加：`internal/biz/computeruse/usecase.go`（beginStep/rejectBudgetStep）、`internal/computeruse/vlm.go`（正则+判负）、`internal/biz/memory_worker.go`（FlowLogWriter.LogFlowWarn）、`internal/service/event_adapter.go`（LogFlowWarn 实现）、`internal/cronrunner/jobs/{memory_l1_archive_test,memory_canary_test}.go` + `internal/tools/clientbridge/bridge_test.go`（fakes 补 LogFlowWarn）。

### Phase P2 — Linux sidecar（后续迭代）
### Phase P3 — iOS 模拟器（macOS 宿主 WDA + MCP 托管，后续迭代）

## 5. 总验收标准

按需求文档 §3 A1-A10 全量；另加：全量 `make api && make wire && make build && make test && make lint` 绿；前端三件套绿；真机演示 A1 通过（日志+UI 双重验证，遵守 R3）。

## 6. 改动文件清单

见 §2.2 新增锚点；修改锚点：`internal/tools/toolset.go`（Registry 追加）、`internal/tools/toolset_assemble.go`（装配）、`internal/data/builtin_tools_seed.go`（种子）、`internal/event/contract/monitor_event.go`（新 MonitorEvent 类型）、`internal/event/trace_context.go`（TraceDomainComputerUse）、`internal/event/flow_log.go`（step 登记）、`internal/service/event_adapter.go`（domainForStepID 前缀映射）、`internal/computeruse/step_events.go`（MonitorBus 适配器）、`internal/agent/tool_confirm_gate.go`（danger 短路）、`internal/biz/step.go` + `internal/biz/activity.go` + `internal/agent/v2/projector.go`（Step.Danger 传递）、`internal/service/`（proto 实现）、`web/src/components/chat/ConfirmBlock.vue`（高危徽标+两按钮）、`web/src/components/chat/v2/TurnContainer.vue`（内嵌步骤流 + S1 白名单注释）、`web/src/features/computeruse/`（CuStepStream/useCuStepStream + S2 TECH-DEBT 标注）、`web/src/i18n/locales/{zh-CN,en-US}.ts`（computeruse + chat.confirm.danger keys）、`web/src/realtime/monitorEvent.ts`（事件类型）、`web/src/services/index.ts`（ComputerUseService）、`web/src/components/tools/toolUi.ts`（分类筛选）、`docs/development/52-flow-logger.design.md`（§5.1 同步）、`AGENTS.md`（如需登记 bin/cua 产物约定）。

M1.5 审查修复追加：`internal/biz/computeruse/usecase.go`（B1 锁收敛/B2 终态解映射/B3 状态机）、`internal/biz/computeruse/session_state_machine.go`（转换表补全）、`internal/biz/computeruse/models.go`（Snapshot.Screenshot 字段）、`internal/computeruse/client.go`（InFlight）、`internal/computeruse/process.go`（看门狗跳过在途）、`internal/computeruse/gateway.go`（zoom/includeScreenshot 映射）、`internal/computeruse/sidecar/aranea-cua-win/{CaptureService,JsonRpc,Models}.cs`（F1/F2）、`api/kratos/session/v1/session.proto`（StepV2.danger=23）、`internal/service/session_v2.go`（bizStepToProto 映射）、`web/src/features/session/v2Api.ts`（danger 映射）。

## 7. 风险与对策

见设计文档 §5。实施纪律：TDD（先失败测试）；每 Phase 完成跑分级验证；不顺带改无关模块；发现问题回退设计。
