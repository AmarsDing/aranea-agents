# Computer Use 二次调研与下一步实施方案（M3）

> 日期：2026-08-15 | 类型：research（调研 + 方案） | 状态：已实施（M3.0–M3.4；M3.5 后置）
> 前置：[`2026-08-12-research-computer-use.md`](./2026-08-12-research-computer-use.md)（选型 + M1/M2 方案，已落地）
> 代码基座：M75 Windows sidecar + 混合 grounding（✅）；M77 注入防护 + 评测 runner + 手册（✅）
> 本文目标：对照 2026-08 开源实现、前沿论文与厂商文档，判定 **M2 之后真正该做的下一跳**，而不是再造一轮 GUI 执行层。

---

## 0. 结论先行

Aranea Computer Use 已经是一套可用的 **L2 本机桌面执行层**（a11y-first + 视觉兜底 + 安全门 + 审计），Windows 短任务（记事本级）已 E2E 验证。下一步的瓶颈 **不在「会不会点按钮」**，而在三件事：

1. **动作面未齐**：sidecar 已实现 `wheel/drag`，biz/工具层未暴露；缺 `wait`/`scroll`/`zoom` 一等公民动作（Claude Computer Use 2025-11 版已标配）。
2. **长程智能不足**：OSWorld 2.0（2026-06，108 任务，人类中位 1.6h）上最强闭源也只有 **20.6% 二元完成率**。失败主因是丢约束、错过中途信息、该问不问、跳过验证——不是 grounding。Aranea 当前把规划完全交给通用 LLM，会话无约束账本、无强制复检预算。
3. **桌面共存差**：默认 `SendInput` 会抢前台；trycua cua-driver 已把 background-first（UIA Invoke / PostMessage / 不抬窗）做成跨平台契约。运维场景可以抢焦点，日常本机助手不行。

**产品定位不变（继承方案 11 号）**：API / CLI / 代码优先，GUI 只做补盲。禁止把 Aranea 做成「无人值守长链路 CUA」。禁止用纯视觉端到端（UI-TARS）推翻 a11y 快路径。

推荐路线：**先收口 M77 评测债 → 补齐 Windows 动作面与 grounding 插件 → 把 OSWorld 2.0 的失败模式做成会话级工程机制 → 背景输入作为可选能力。Linux/iOS、bBoN 多轨迹、UFO³ 跨设备一律后置。**

---

## 1. Aranea 现状（代码实测，非文档宣称）

### 1.1 已落地能力（M75 M1–M2 + M77 G2）

| 层 | 锚点 | 真实能力 |
|----|------|---------|
| 工具 | `internal/tools/computeruse` | 5 件套：`observe` / `screenshot` / `act` / `launch` / `session`；act 支持 `actions[]` fail-fast 批量 |
| 编排 | `internal/biz/computeruse` | 会话状态机、预算 50 步/30 分钟、敏感词高危、禁区进程、干跑、急停、注入打标 |
| Grounding | usecase `groundAndExecute` | a11y 模糊匹配 → OmniParser+SoM+VLM 选编号 → VLM 坐标直判+480×360@2x zoom；无匹配明确失败（不乱点） |
| 执行后 | `verifyAfterAction` | settle 400ms → 元素树 hash + 前台窗口；`verify` 透出给 LLM |
| Sidecar | `aranea-cua-win`（.NET+FlaUI） | CDP 全集含 `action.wheel` / `action.drag`；UIA invoke；PerMonitorV2 DPI |
| 视觉 | OmniParser HTTP `:8101` + catalog 多模态 | 本地 `qwen2.5vl-cua`（num_ctx=8192）；云端 qwen3-vl-plus 待 API key |
| 安全 | 确认门 + `InjectionGuard` | 屏幕文本模式扫描；命中后写动作强制 danger（授权链不可豁免） |
| 观测 | audit 表 + `computeruse.step` | Chat 内 CuStepStream；刷新后不回放（TECH-DEBT S2） |

### 1.2 明确缺口（代码注释 / 未建路径）

| 缺口 | 证据 | 影响 |
|------|------|------|
| `wheel`/`drag` 未上工具面 | usecase.go：「wheel/drag 不在 P1 暴露（biz port 未定义，CDP 能力保留）」；`DeviceActor` 仅 Invoke/Click/Type/Key | 无法滚列表、拖滑块、拖文件；长页面任务直接卡死 |
| 无 `wait` 动作 | Claude / OSWorld 轨迹中 wait 占动作预算 ~14% | 弹窗/加载只能靠固定 400ms settle，时序失败无法由模型主动等待 |
| zoom 非一等动作 | 仅 `vlm_direct` 内部精化；Agent 不能主动 `screenshot(region, zoom)` 作为观察策略（工具有 zoom 参数但规划器不知道何时用） | 密集 UI（CAD/网管表）命中率上限被锁在通用 VLM |
| 专用 grounding 模型未接 | M2 遗留「UI-TARS 为后续演化」；catalog 仍用通用 VL | ScreenSpot-Pro 上通用 VL 远低于 UI-Venus-1.5 / Jedi |
| 会话纯内存 | `sessions`/`suspectedByAgent` map | 进程重启丢失急停对象、注入打标、预算 |
| 步骤流不回补 | `useCuStepStream.ts` TECH-DEBT | 刷新后 Chat 内步骤卡空白，审计只能走 REST |
| 评测 runner 未建 | `sample-gui-ops-tasks.json` 已有；`test/gui-ops-eval/` 不存在 | M77 G3 未完成，无法量化回归 |
| 手册未写 | `competition/12-GUI运维取证与处置手册.md` 不存在 | 竞赛/运维交付缺行为规约 |
| Linux / iOS sidecar | 开发计划 Phase P2/P3 空 | 跨 OS 为零 |
| 抢前台 | sidecar `SendInput` | 本机助手会打断用户正在做的事 |
| 应用 quirks 记忆 | 原调研 H 条「后续接 case 记忆」从未开工 | Electron/自绘应用每次从零 grounding |
| 约束账本 / 强制复检 / 该问则问 | 规划在 Agent LLM，CUA 会话无任务级状态 | 对标 OSWorld 2.0 的主失败模式，零覆盖 |

### 1.3 能力分层（便于对标）

```
L0 感知      ████████░░  Windows UIA 快照 + 截图；无差分 2fps 观察流（需求有、未做）
L1 定位      ███████░░░  a11y 快路径成熟；视觉链通路但模型非 SOTA
L2 动作      █████░░░░░  invoke/click/type/key；wheel/drag 沉在 sidecar
L3 安全      █████████░  业界少见的确认门+注入+禁区+预算组合
L4 观测      ██████░░░░  步级审计有；无视频、无刷新回放
L5 规划      ███░░░░░░░  完全外包给 Chat/Team LLM；无 CUA 专用记忆
L6 评测      ███░░░░░░░  记事本 E2E；运维 5 任务 JSON 无 runner
L7 平台      ██░░░░░░░░  仅 Windows 本机；无沙箱 VM、无背景输入
```

---

## 2. 开源软件调研（2026-08 截面）

### 2.1 执行层 / Driver（Aranea sidecar 的同类）

| 项目 | 形态 | 对 Aranea 的可借鉴点 | 不直接抄的原因 |
|------|------|---------------------|----------------|
| **trycua/cua**（cua-driver） | MIT，跨 OS MCP/CLI driver | **背景输入契约**：默认 `delivery_mode=background`（UIA Invoke → PostMessage → 触控/笔注入），失败才 `foreground`；Windows 交互会话 daemon 解决 Session 0 无桌面；像素点击先 UIA hit-test 再决定 invoke vs SendInput | 完整 PostMessage 栈工程量大、应用 quirks 多；应作为 M3.3 可选能力，不是推翻 sidecar |
| **FlaUI-MCP** | C# MCP，元素 ref 点击 | 已是 ADR-CU-03 的形态验证；Aranea CDP 已吸收 | 无 grounding/安全/审计 |
| **微软 UFO² / UFO³** | UFO²=Windows UIA+OmniParser 混合检测；UFO³=跨设备 TaskConstellation DAG | 双源 IoU 融合已落地（fusion.go）。UFO³ 的「子任务图 + 设备分配」对 Team 编排有启发，但对单机运维补盲过重 | NebulaBench 是 5 机 10 类跨设备，超出本模块边界 |
| **OmniParser V2** | 屏幕 tokenizer | 已是标准视觉组件；2026-07 增加 YOLOv9-E 检测器可选 | CPU 8–15s 约束仍在，继续不进 a11y 快路径 |

### 2.2 Agent 框架（规划层，不是 driver）

| 项目 | 关键机制 | OSWorld 量级 | 对 Aranea |
|------|----------|--------------|-----------|
| **Agent S3**（Simular, arXiv:2510.02250） | 去掉 manager-worker 层级；**原生 coding agent**（能改文件就不要点 GUI）；**bBoN**（多条完整轨迹 + 行为叙事裁判选优） | 单 agent 62.6% → bBoN 69.9%（OSWorld 1.x，接近人类 72%） | **代码优先**应接到已有 M76 Coding Agent Bridge：规划器先判「能否 API/脚本」，GUI 是 fallback。bBoN 成本高，生产默认不做 |
| **UI-TARS-1.5/2**（字节） | 端到端像素→动作模型；Desktop 应用 | OSWorld 100 步 42.5%（1.5）；2 为闭源 all-in-one | 可作 **专用 grounding 模型候选**，不可替换 a11y 快路径（M2 已否决方案 B） |
| **OpenCUA-72B**（xlang） | 开源模型+数据+工具 | ~45%（100 步，当时开源 SOTA） | 同 UI-TARS：grounding 插件，不换架构 |
| **Jedi**（xlang, NeurIPS 2025） | 4M grounding 数据；Jedi-7B | ScreenSpot-Pro 39.5%；OSWorld-G 54.1%；把通用模型 OSWorld 从 5% 拉到 27%/51% | **最便宜的精度提升**：catalog 增加 Jedi/UI-Venus 作为 VisionGrounder，不改协议 |
| **UI-Venus-1.5**（inclusionAI, 2026-02） | screenshot-only + RFT；ZoomIn | ScreenSpot-Pro **69.6%**（30B-A3B），+ZoomIn **74.8%** | 当前开源 grounding SOTA；8B+ZoomIn 已 73.9%，适合本机 GPU 作为第 4 条 grounding 路径 |
| **browser-use / Stagehand / Skyvern** | L1 浏览器 | WebVoyager 量级 | Aranea 已有 Playwright MCP；保持「浏览器走 browser_*，桌面走 computer_use_*」 |

### 2.3 对标矩阵（执行层，不是榜单分数）

| 维度 | Aranea M2 | Claude Computer Use | cua-driver | UFO² | Agent S3 |
|------|-----------|---------------------|------------|------|----------|
| 感知 | UIA + 截图 | 纯截图 | UIA/MSAA + 窗口像素 | UIA + OmniParser | 截图（可加 a11y） |
| Grounding | a11y → SoM → 坐标 | 模型内坐标 + zoom | hit-test → invoke | 双源融合 | 外置 grounder |
| 动作 | 4 种暴露 / 6 种 CDP | screenshot/click/type/key/scroll/drag/wait/hold/zoom | click/type/scroll/drag + background | Windows 控件模式 | GUI + **code** |
| 安全 | 确认门/注入/禁区/预算/急停 | 模型侧谨慎 + 宿主沙箱 | 不抢焦为默认安全 | 企业策略 | 无产品级 HITL |
| 长程 | 无 | 模型内记忆 + batched tools | 无（harness 侧） | DAG 跨设备 | 扁平 worker + bBoN |
| 部署 | 本机 sidecar | 云端模型 + 本地/容器执行 | 本机 daemon | Windows AgentOS | 研究框架 |

**一句话**：Aranea 的安全与 a11y 快路径是差异化；动作面、背景输入、专用 grounding、长程状态是短板；规划智能应复用 Chat/Team/M76，而不是在 CUA 模块里再造一个 Agent S3。

---

## 3. 前沿论文与技术文档（对工程的可执行启示）

### 3.1 OSWorld 2.0（arXiv:2606.29537，2026-06；任务包 2026-08-08 更新）

- 108 个长程工作流，人类中位 **1.6 小时**，约 48× OSWorld 1.0。
- Claude Opus 4.8 max thinking + batched tools：**二元完成 20.6%**，部分分 54.8%；GPT-5.5 ~13% 但 token 效率高。
- 失败不是不会点：轨迹分析五类——信息跟踪失败、感知-动作时序、领域工作流、**验证/反思缺失**（自修复预算 <7%）、长程状态漂移。
- 动态环境：任务中途插入邮件/聊天会改约束；agent 当背景噪声。
- 该问不问：缺证据时直接提交，而不是 ASK_USER。
- 流式 UI：弹窗在推理期间移动，截图坐标过期。
- 动作预算结构：GUI click 27%、终端 25%、热键 14%、**wait 14%**。

**工程映射（必须做，且正好落在 Aranea 已有钩子上）**：

| 论文失败模式 | Aranea 现状 | M3 措施 |
|--------------|-------------|---------|
| 丢失初始约束 | 无会话级约束存储 | **约束账本**：session.start / 首次 observe 从用户指令抽取 MUST/MUST-NOT，每步 observe 结果顶部回灌 |
| 中途信息当噪声 | 每步重新 snapshot，但 LLM 无「变更频道」 | observe 返回 `generation` + 前台窗口变化；工具 desc 要求先读再规划 |
| 该问不问 | grounding 失败只报错 | 连续 2 次 `ErrGroundingFailed` 或 verify 无变化 → 工具结果带 `ask_user=true`，确认门可复用 |
| 跳过验证 | verify 已透出，LLM 可忽略 | `verify.changed=false` 时 **强制** 下一步只能 observe/screenshot（会话标志 `must_reobserve`） |
| 坐标过期 | ref 同代校验已有 | 视觉路径执行前 **短 settle + 再 snapshot**；提供 `wait` 动作 |
| 自修复预算 <7% | 无 | 预算内预留 10% 步数给 verify/retry；超限明确告诉模型「进入收尾」 |

**明确不做**：自建 OSWorld 2.0 评测集群。用运维 5 任务 + 记事本 E2E 作为回归；grounding 精度用 ScreenSpot-Pro 子集离线测，不进 CI。

### 3.2 Agent S3 / bBoN（arXiv:2510.02250）

- 层级规划在 CUA 上是负优化（延迟+误差传播）；扁平 worker 更好。
- **能写代码就不要点 GUI**：OSWorld 上 coding agent 贡献了大幅增益。
- bBoN：多条完整轨迹 + 行为叙事裁判。成本 ≈ N 倍 rollout。

**工程映射**：Team/Chat 规划器增加路由规则——文件/配置/日志/API 任务优先 `coding_*` / `browser_*` / TwinMonitor API；仅当目标无可编程面时启用 `computer_use_*`。bBoN 仅作为离线评测选项，默认关闭。

### 3.3 Jedi / OSWorld-G（arXiv:2505.13227）与 UI-Venus-1.5（arXiv:2602.09082）

- 通用 VL 在 ScreenSpot-Pro（高分屏专业软件）仍然弱；专用 grounding + ZoomIn 把 Pro 从 ~28%（Qwen2.5-VL-7B）拉到 70%+。
- ZoomIn 是可叠加增益（Venus-1.5-8B 68.4% → 73.9%）。

**工程映射**：`VisionGrounder` 已是端口。新增 catalog 模型类型或独立 HTTP grounder（Jedi-7B / UI-Venus-1.5-8B），插入 SoM 与 `vlm_direct` 之间：a11y → SoM → **专用坐标模型** → 通用 VLM 直判。Zoom 提升为 `computer_use_screenshot` 的推荐策略（工具 desc + Skill 手册）。

### 3.4 Anthropic Computer Use 文档（`computer_20251124`）

动作全集：screenshot, left/right/middle click, double/triple, drag, scroll(方向+量), type, key, hold_key, wait, mouse_move, **zoom(region)**。

DPI/分辨率不一致仍是官方第一误点原因（Aranea 已用物理像素+PerMonitorV2 处理）。

**工程映射**：P1 补齐 scroll/drag/wait；zoom 作为感知动作而非只在直判内部。hold_key / mouse_down/up 可 P2。

### 3.5 cua-driver 技术文档（Windows background）

- 默认不抢焦；`background_unavailable` 时由调用方显式 `foreground`。
- 像素点击：UIA hit-test → 有 Invoke 就走无障碍 → canvas 才 Win32。
- Session 0 服务进程必须经交互会话 daemon。

**工程映射**：Aranea 的 `action.invoke` 已经是「不经 SendInput」的快路径。M3.3 把 Click/Type 的默认策略改为：能 invoke 绝不 click；click 增加 `delivery=background|foreground`（默认 foreground 以保持运维演示兼容，本机助手 profile 改 background）。

---

## 4. 方案选择

| 方案 | 内容 | 评估 |
|------|------|------|
| **A 执行层补齐 + 会话智能（选定）** | 动作面/grounding 插件/约束账本/强制复检/wait；背景输入可选；收口 M77 runner | 不推翻 ADR-CU-01/02；增量全部落在已有 port；对标论文失败模式有一一对应 |
| B 换端到端视觉模型 | 弃 a11y，上 UI-TARS-2 / Venus 导航模型 | M2 已否决；短任务精度仍低于混合；延迟与 GPU 成本高 |
| C 托管 CUA API | Claude/Operator 全托管 | 截图出域；无法接确认门/禁区/注入 |
| D 上 bBoN / UFO³ 跨设备 | 多轨迹或跨机 DAG | 成本与复杂度不匹配「GUI 补盲」定位 |
| E 先做 Linux/iOS | P2/P3 sidecar | 当前唯一生产桌面是 Windows；先把 Windows 动作面做完再同构 |

选定 **A**。下面按可评审的里程碑拆开。

---

## 5. 实施方案（Phase M3）

### 5.0 原则

- GUI 补盲：规划层（Chat/Team Skill）写明「有 API 禁止 GUI」。
- 短原子动作：继续 50 步硬顶；长任务靠人工拆步或 Team 图，不在 CUA 内做小时级无人值守。
- TDD；只改 computeruse 相关文件；文档同步 75/77 三件套状态。
- 新 ADR 必须写入 75-computer-use.design.md §0。

### 5.1 Phase M3.0 — 收口 M77 评测债（P0，约 2–3 天）

未完成项会让「能跑」无法证明「能回归」。

| # | 任务 | 验收 |
|---|------|------|
| 1 | 建 `test/gui-ops-eval/`：加载 `sample-gui-ops-tasks.json`、Verifier 接口、5 个判定器单测、`--dry-run` 列任务 | `go test ./test/gui-ops-eval/...` 绿 |
| 2 | 写 `competition/12-GUI运维取证与处置手册.md`（工具用法、屏幕文本不作指令、取证、审批） | G3 文档勾完 |
| 3 | 回写 77 开发计划状态；方案 11 号 P0 状态 | DOC-SYNC-5 |

**不做**：S1/S2 环境联调、录屏 G4。

### 5.2 Phase M3.1 — 动作面齐套（P0，约 1 周）

把 sidecar 已有能力接到 biz/工具，并补 wait。

| # | 任务 | 设计要点 | 验收 |
|---|------|----------|------|
| 1 | `DeviceActor` 增 `Wheel`/`Drag`/`Wait`（窄接口仍 ≤5？Wait 可放 DeviceController 或新 `DevicePointer`） | 超 5 方法继续拆窄接口，Gateway 组合 | 单测 mock sidecar |
| 2 | `computer_use_act` schema：`action` 增加 `wheel`/`drag`/`wait`；wheel 需 x,y,delta；drag 需 from/to；wait 需 `ms`（上限 10s） | 高危词仍走 danger | tools 单测 + usecase 单测 |
| 3 | 工具 desc 与 Skill 片段：何时滚、何时等、禁止用 wait 空转耗尽预算 | | desc 含 wait 预算警告 |
| 4 | 记事本/文件管理器 E2E：滚轮 + 拖拽各 1 条（可 dry_run+真机可选） | | 真机或 sidecar 单测 |

**ADR-CU-07**：动作面与 Claude `computer_20250124` 对齐到 scroll/drag/wait；hold_key/mouse_down 暂缓。

### 5.3 Phase M3.2 — Grounding 插件化（P1，约 1 周）

| # | 任务 | 设计要点 | 验收 |
|---|------|----------|------|
| 1 | 新端口实现 `SpecialistGrounder`（或复用 `PickCoordinate`）：HTTP/OpenAI 兼容，输入图+target，输出点或 ref | catalog 增加 `vision_grounding=true` 标记的模型优先于通用 VL | 单测 httptest |
| 2 | fallback 链改为：a11y → SoM(OmniParser+通用 VL 选号) → **专用坐标模型** → 通用 `vlm_direct` | 每级 warn 日志已有 | Path 枚举加 `grounder` |
| 3 | 推荐权重：本机 GPU 用 UI-Venus-1.5-8B 或 Jedi-7B（二选一，先接 OpenAI 兼容端点，不绑定具体权重仓库） | 无权重时跳过该级 | Available() 降级 |
| 4 | 工具/手册：密集 UI 先 `screenshot(region, zoom=2)` 再 act | zoom 已在 screenshot 工具 | desc 更新 |

**ADR-CU-08**：专用 grounding 模型是第 4 级可选路径，不是端到端策略模型。

### 5.4 Phase M3.3 — 会话智能（P0，约 1–1.5 周）——对标 OSWorld 2.0 的主杠杆

全部做在 `biz/computeruse` 会话内存态，**不改 Ent**（与 M77 注入打标同风格）；需要持久化时再单独立项。

| # | 机制 | 行为 | 验收 |
|---|------|------|------|
| 1 | 约束账本 | `StartSession` / 首次 Act 可带 `goal`；Usecase 用已注入的 LLM 抽 3–8 条约束（失败则把原始 goal 整段当作约束）。`Observe`/`Act` 返回 `constraints[]` | 单测：返回含注入的约束；LLM nil 时回退原文 |
| 2 | must_reobserve | `verify.changed==false` 且动作非 wait → 会话置位；下一次只允许 observe/screenshot/wait，Act 写动作返回明确错误 | 单测 |
| 3 | ask_user | 同会话连续 2 次 grounding 失败 → 结果 `ask_user=true` + 建议问题 | 单测 |
| 4 | 路由提示 | observe 工具 desc 增加「若任务可用 API/文件/CLI，应改用其它工具」 | desc 单测或快照 |
| 5 | wait 与 settle | 模型可 wait；系统 settle 仍 400ms；wait 计入预算 | 预算单测 |

**ADR-CU-09**：长程可靠性用会话级约束与强制复检，不用多轨迹 bBoN。

约束抽取若不想在 CUA 内调 LLM：第一期只存用户原始 `goal` 字符串并每步回灌，抽取放到 M3.3b。

### 5.5 Phase M3.4 — 观测补齐（P1，约 3–4 天）

| # | 任务 | 验收 |
|---|------|------|
| 1 | CuStepStream 挂载时 `ListComputerUseSteps` 回补；复用 `cuStepFromMonitorEvent` 口径（含 danger/confirmed_by） | 前端单测：REST+WS 去重 |
| 2 | 步骤卡可选缩略图（audit.screenshot_ref 若空则跳过） | 无 ref 不崩 |
| 3 | 会话重启：文档标明内存会话限制；可选 `computer_use_session` 表 **不做**（避免范围膨胀） | 手册写明 |

G4 录屏：仍按方案 11 号 P1 规划，本轮不做。

### 5.6 Phase M3.5 — 背景输入（P2，约 1–2 周，可裁）

仅当「本机助手不打断用户」成为明确产品需求时开工。

| # | 任务 | 说明 |
|---|------|------|
| 1 | invoke 路径保持默认（已不抢焦） | 无工作量 |
| 2 | click/type 增加 `delivery=foreground\|background` | background：UIA hit-test + Invoke；失败返回 `background_unavailable`，禁止默默 SendInput |
| 3 | 运维 spirit 默认 foreground；个人助手 profile 默认 background | 配置，不改协议必选字段 |

参考 cua-driver，**不要**第一期做 PostMessage/触控注入全集。

### 5.7 明确后置（本轮禁止开工）

| 项 | 原因 |
|----|------|
| Linux AT-SPI / iOS WDA sidecar | 无生产桌面；协议已预留，等 Windows M3.1 稳定再同构 |
| bBoN 多轨迹 | 成本 ×N；与 50 步补盲定位冲突 |
| UFO³ 跨设备 DAG | 属 Team 编排，不是 computeruse 模块 |
| 端到端 UI-TARS 替换 a11y | ADR-CU-02 |
| 云端托管 CUA | 截图出域 |
| 完整 OSWorld 2.0 评测 | 环境 31 站自托管，超出仓库职责 |
| 录屏审计 G4 | 方案已标 P1，不阻塞 M3.0–M3.3 |
| 应用 quirks 记忆库 | 依赖真实失败样本；先靠专用 grounder |

---

## 6. 建议实施顺序与依赖

```
M3.0 评测债 ─────────────────────────────┐
M3.1 动作面（wheel/drag/wait） ──────────┼─→ M3.3 会话智能（wait 被约束账本使用）
M3.2 grounding 插件（可与 3.1 并行） ───┤
M3.4 步骤流回补（可与 3.3 并行） ────────┘
M3.5 背景输入（独立，可砍）
```

**最小可交付（评审若砍范围）**：只做 M3.0 + M3.1 + M3.3 的 must_reobserve/ask_user（不含 LLM 约束抽取）。这三项直接打中「不能滚」「不能等」「失败后乱点/不验证」。

### 6.1 改动文件预估（M3.0–M3.3）

- `internal/biz/computeruse/`：ports、models（Path/ActionType/Session 标志）、usecase、测试
- `internal/computeruse/gateway.go`：Wheel/Drag/Wait CDP
- `internal/tools/computeruse/tools.go` + 测试 + 种子 catalog-patch（设计文档已警告 schema 不自动演进）
- `internal/computeruse/vlm.go` 或新 `grounder_http.go`
- `web/src/features/computeruse/useCuStepStream.ts`（仅 M3.4）
- `test/gui-ops-eval/`、`competition/12-*.md`
- 文档：75 三件套补 M3；77 勾完 G3；本报告评审后改状态

---

## 7. 风险

| 风险 | 等级 | 缓解 |
|------|------|------|
| 专用 grounding 模型显存与 OmniParser+7B VL 争 GPU | 高 | 插件默认关闭；与 OmniParser 互斥或远程 |
| wait 被模型用来空转耗尽预算 | 中 | 单次 ≤10s；计入步数；desc 警告 |
| 约束抽取幻觉 | 中 | 第一期只回灌原文 goal |
| background 输入在 Electron 静默失败 | 高 | 失败必须显式 `background_unavailable`，禁止假成功 |
| 范围膨胀到「做 OSWorld」 | 高 | §5.7 禁止清单；评审卡范围 |

---

## 8. 决策记录（拟新增，评审通过后写入 design）

- **ADR-CU-07**：工具动作面与 Claude 20250124 对齐到 wheel/drag/wait。
- **ADR-CU-08**：专用 GUI grounding 模型作为 fallback 链中的可选一级，不替换 a11y。
- **ADR-CU-09**：用会话约束账本 + 强制复检 + ask_user 对抗长程失败；不用 bBoN。
- **ADR-CU-10**（可选）：click/type 支持 background delivery，默认仍 foreground。

原 ADR-CU-01～06 保持有效。

---

## 9. 参考来源

- OSWorld 2.0：arXiv:2606.29537；https://osworld-v2.xlang.ai/ ；任务包 `osworld-v2-2026.08.08`
- Agent S3 / bBoN：arXiv:2510.02250 ；https://www.simular.ai/articles/agent-s3
- Jedi / OSWorld-G：arXiv:2505.13227 ；https://osworld-grounding.github.io/
- UI-Venus-1.5：arXiv:2602.09082 ；https://github.com/inclusionAI/UI-Venus
- UFO³：arXiv:2511.11332 ；https://github.com/microsoft/UFO/
- Anthropic Computer Use：https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool
- trycua cua-driver：https://github.com/trycua/cua ；https://cua.ai/blog/inside-windows-computer-use
- OmniParser：https://github.com/microsoft/OmniParser （2026-07 YOLOv9-E）
- UI-TARS：https://github.com/bytedance/UI-TARS
- 本仓库：75 三件套、77 三件套、`competition/11-ComputerUse-GUI运维通道方案.md`

---

## 10. 评审后动作

1. 通过 → 在 `75-computer-use.development.md` 增补 Phase M3（状态 📋），design §0 写入 ADR-CU-07～09。
2. 按 M3.0 → M3.1 → M3.3 实施（TDD），M3.2/M3.4 可并行。
3. 真机回归：记事本 A1 + 滚轮/等待各一条；注入 T5 逻辑单测保持绿。

**实施记录（2026-08-15）**：M3.0–M3.4 已落地（见 75 开发计划 Phase M3）；M3.5 背景输入按方案后置。
