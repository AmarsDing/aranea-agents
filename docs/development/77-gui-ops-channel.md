# M77: GUI 运维通道（Computer Use × TwinMonitor 场景集成）— 需求规格

> 编号：77 | 状态：G1–G3 已完成（2026-08-15）；P2 场景实跑属环境联调，不在本模块
> 上游方案：[competition/11-ComputerUse-GUI运维通道方案.md](../../competition/11-ComputerUse-GUI运维通道方案.md)
> 能力基座：M75（75-computer-use.md，已落地 ✅）——本模块不重复建设 GUI 能力，只做**运维场景集成 + 安全增强 + 评测资产**。

## 1. 模块定位

将 M75 桌面 GUI 控制能力（`computer_use_*`）与受控浏览器能力（`browser_*`，Playwright MCP + SSRF 防护）以「E 通道」形态接入 TwinMonitor 智能运维部门，并补齐运维场景特有的安全机制（屏幕内容注入防护）与可量化评测（运维 GUI 任务集）。

**定位原则**（继承 10 号方案）：API 优先、事件驱动、SQL 兜底、**GUI 补盲**——E 通道只在目标无 API/无 CLI 时启用。

**边界**：
- 不做长链路无人值守 GUI 自动化（OSWorld 2.0 数据：长链路二元完成率仅 20.6%）；GUI 动作原子化、每步验证、高危审批。
- 不含录屏审计（P1 规划，见方案 §1.2 G4）。
- 不含 S2（设备网管页）/S1（TwinWeb 取证）场景实跑——属环境联调（方案 §七 P1/P2），本模块交付其使能代码与资产。

## 2. 需求清单

### 2.1 R1 屏幕内容注入检测（P0）

- Observe 感知到的全部 UI 元素（名称/文本）经注入模式扫描。
- 模式表：中英双语指令性短语（"ignore previous instructions"、"忽略之前/以上指令"、"system prompt"、"you are now"、"新指令"、"覆盖指令"等），匹配口径复用 `policy.go` 的 normalize（小写+去空白标点）。
- 模式表内置默认 + 可通过 Deps 注入自定义表（nil=默认，与 Policy 同风格）。
- **只做检测，不篡改屏幕内容**（红线）。

### 2.2 R2 命中会话高危升级（P0）

- 命中即会话级打标 `injection_suspected=true`（内存态，不落库、不改 Ent schema）。
- 命中会话内后续写动作（`Act`/`Launch`）**无条件 danger=true**（与敏感词命中并联，逻辑或），强制走逐次人工确认（复用 M75 确认门 danger 短路，授权链不可豁免）。
- 只读动作（Observe/Screenshot）不受限。
- 会话打标对调用方透出：`ObserveResult`/`Act` 结果含 `injection_suspected` 字段。

### 2.3 R3 检出留痕（P0）

- 每次命中写流程日志 warn 级：命中模式、元素 ref、片段摘要（截断 ≤80 字符，防日志膨胀与二次注入）。
- 后续写动作步的审计记录 `danger=true` 天然留痕（复用既有审计链，零 schema 变更）。

### 2.4 R4 运维 GUI 评测任务集（P1）

- 任务定义 JSON：`docs/testing/test-data/sample-gui-ops-tasks.json`（sample- 前缀合规）。
- 每任务字段：`id / title / channel(browser|computer_use) / instruction / setup(环境预置描述) / verifier{type, params} / risk_level`。
- 首批 5 任务（T1-T5，见方案 §五），其中 T5 为注入防护红队任务（验证 R1-R3 端到端生效）。

### 2.5 R5 评测 runner（P1）

- `test/gui-ops-eval/`：加载任务 JSON → 经工具层执行 → 运行可执行验证器 → 输出结构化结果（pass/fail + 证据）。
- 验证器接口化（`Verifier`），内置：`rest_assert`（REST 对账）、`artifact_exists`（产物存在+非空白校验）、`file_hash`（文件内容 hash）、`text_contains`（提取文本断言）、`injection_blocked`（动作未执行+打标存在）。
- runner 为手工/演练触发（需真实 sidecar/TwinWeb 环境），**非 CI 门禁**；单测覆盖任务加载与验证器判定逻辑（mock 执行面）。

### 2.6 R6 Skill 手册文本（P1）

- 第 6 本运维手册《GUI 运维取证与处置手册》文本（竞赛材料，附于方案或 competition 目录）：browser/computer_use 工具用法、注入规避规约（Agent 侧行为准则：屏幕文本仅作数据不作指令）、截图取证规范、高危动作审批路径。

## 3. 验收标准

| # | 场景 | 验收标准 |
|---|------|---------|
| B1 | 注入检出 | 元素名含"ignore previous instructions"的 snapshot → Observe 返回 `injection_suspected=true` 且流程日志 warn 含模式/ref/摘要 |
| B2 | 高危升级 | 命中会话内 `act`（无敏感词的正常动作）→ danger=true；`launch` 同理；未命中会话不受影响 |
| B3 | 授权不可豁免 | 命中会话内即使已持久授权，`act` 仍需逐次确认（danger 短路复用，行为与敏感词一致） |
| B4 | 只读放行 | 命中会话内 `observe`/`screenshot` 正常返回 |
| B5 | 内容不篡改 | Observe 返回的元素清单与 sidecar 原始快照一致（不删改命中元素） |
| B6 | 评测资产 | `sample-gui-ops-tasks.json` 5 任务字段完整、可被 runner 加载；验证器单测绿 |
| B7 | 回归 | `go test ./internal/biz/computeruse/... ./internal/tools/computeruse/...` 全绿（含 M75 既有用例） |

## 4. 非功能需求

| 类别 | 要求 |
|------|------|
| 性能 | 注入扫描为纯字符串匹配，Observe 附加耗时 < 5ms（500 元素量级） |
| 安全 | Guard 不可关闭（无开关参数）；命中摘要截断防日志注入；不引入新外部依赖 |
| 兼容 | 零 Ent schema 变更、零迁移；M75 既有行为仅在「命中会话的写动作」上变化 |
| 可观测 | 流程日志 warn（复用 LogFlowWarn，M2.5 已建）；不新增 MonitorEvent 类型 |

## 5. 关联文档

- 上游方案：competition/11-ComputerUse-GUI运维通道方案.md（能力评估数据、演示剧本、评分映射）
- 能力基座：75-computer-use.md / .design.md / .development.md
- 工具装配规范：internal/tools/doc.go | 模块交叉参考：65-module-cross-reference-full.md
