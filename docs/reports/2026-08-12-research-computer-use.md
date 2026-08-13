# Computer Use 能力调研与实施方案

> 日期：2026-08-12 | 类型：research（调研 + 方案） | 状态：已评审通过
> 需求：让 Aranea Agent 能够控制 Windows / Linux / iOS 系统；**Windows 优先**；要求**快速、精确**。
> 已确认决策：① 集成进 Aranea-Agents 平台（新工具域）；② 本地控制拓扑（控制端与被控机同机）；③ iOS 以模拟器为主（macOS 宿主）；④ 混合模型架构（无障碍树优先 + 视觉模型兜底）。
> **修正（2026-08-12 二次确认）**：开源协议不作为选型约束，以系统能力最强为准 —— OmniParser V2 由"可选组件"升级为**标准视觉解析组件**（仍保持独立 HTTP 服务部署，理由是 Python/torch 技术栈隔离与 GPU 部署弹性，非许可原因）。

---

## 1. 行业格局调研

### 1.1 CUA 三级能力模型

| 级别 | 能力范围 | 代表 | 风险面 |
|------|---------|------|--------|
| L1 浏览器级 | 仅浏览器标签页 | OpenAI CUA / ChatGPT Agent、browser-use（Playwright） | 低 |
| L2 真实桌面级 | 全屏、所有原生应用 | Claude Computer Use、Agent S、UI-TARS | 宿主机直接暴露 |
| L3 沙箱 OS 级 | 完整隔离 VM | trycua（Apple VirtualizationFramework，97% 原生性能） | 宿主隔离、可快照回滚 |

本项目目标为 **L2（本地真实桌面）**，并建议在测试/高危场景用 VM 达到类 L3 隔离。

### 1.2 主要玩家与基准（2026-06 独立榜单 awesomeagents.ai）

| 方案 | OSWorld | 形态 | 启示 |
|------|---------|------|------|
| Claude Opus 4.6 Computer Use | **72.7%**（人类基线 72.4%） | 云端 API，截图→坐标 | 纯视觉天花板已超人类，但依赖顶级模型 |
| Claude Sonnet 4.6 | 72.5% | 同上 | 同上 |
| UI-TARS-2（字节） | 53.1% | 闭源模型 + 开源 desktop 应用 | 开源系最强，可自部署 |
| EvoCUA（美团） | 50.3% | 开源 | 演化式训练参考 |
| OpenCUA-72B（xlang-ai） | ~45%（100 步） | 全开源（模型+数据+工具） | 开源 grounding 基线 |
| OpenAI CUA（ChatGPT Agent） | 38% | L1 浏览器级 | WebVoyager 87%，仅浏览器 |
| 微软 UFO² / UFO³ Galaxy | — | Windows AgentOS，**UIA+OmniParser 混合检测** | 本方案最重要的架构参考 |

### 1.3 核心循环与精度瓶颈

所有 CUA 共享同一循环：`截图 → VLM 推理 → 动作(click/type/key) → 再截图`。
**纯像素坐标预测是误点的最大来源**（Anthropic 官方文档亦承认分辨率/缩放不一致是第一误点原因）。
2026 年业界共识：**无障碍树（a11y tree）优先、视觉兜底**的混合 grounding 是"快速精确"的唯一工程解：

- a11y 命中：元素级定位，**零模型调用、毫秒级、100% 命中控件中心**（甚至可用 UIA Invoke Pattern 无坐标直调）
- 视觉兜底：仅在 a11y 盲区（自绘控件、Electron、游戏画布）介入，配合 SoM 编号标注 + zoom 局部放大提升精度

---

## 2. 关键技术调研

### 2.1 三平台控制通道选型

| 平台 | 感知（a11y） | 感知（视觉） | 执行 | 前提条件 |
|------|------------|------------|------|---------|
| **Windows**（P1） | **UIA**（UI Automation，COM API；FlaUI/pywinauto 封装成熟） | PrintWindow/GraphicsCapture 截图 + OmniParser/SoM | **SendInput**（user32）、UIA Invoke/Toggle Pattern | 无，本机直接可用 |
| **Linux**（P2） | **AT-SPI2**（D-Bus，GTK/Qt 全覆盖；pyatspi/dogtail） | grim(Wayland)/import(X11) 截图 | xdotool(X11) / **ydotool**(Wayland，/dev/input 内核级注入） | 无障碍服务开启；ydotool 需 input 组权限 |
| **iOS 模拟器**（P3） | **XCUITest/WDA 元素树**（accessibility id/label） | WDA `/screenshot` 或 simctl io screenshot | **WebDriverAgent** HTTP 服务（:8100，tap/swipe/type） | macOS 宿主 + Xcode 签名 WDA；模拟器免真机签名复杂度 |

### 2.2 OmniParser V2 专题调研

**定位**：微软开源的"屏幕解析器"，把截图 tokenize 成 LLM 可理解的结构化元素列表，是**纯视觉 grounding 的事实标准组件**（UFO²、OmniTool、bytebot 等均采用）。

**架构**（双模型 + OCR 管线）：

| 组件 | 模型 | 职责 | 许可 |
|------|------|------|------|
| `icon_detect` | 微调 **YOLOv8** | 可交互元素 bbox 定位 + 置信度 + 可交互性分类 | **AGPL** ⚠️ |
| `icon_caption_florence` | 微调 **Florence-2** | 图标功能语义描述（"搜索图标"） | MIT |
| OCR | PaddleOCR / EasyOCR | 文本区域提取 | 视引擎而定 |

**输出契约**（对本方案最重要）：
```json
{ "parsed_content": [
    { "type": "text|icon", "content": "按钮文本/图标语义",
      "bbox": [x1,y1,x2,y2], "interactivity": true,
      "source": "ocr|icon_detection" } ],
  "annotated_image": "<SoM 编号标注图 base64>" }
```

**性能**（V2 比 V1 降延迟 60%）：

| 硬件 | 单帧延迟 | 显存 |
|------|---------|------|
| A100 GPU | ~0.6s | ~4GB |
| Apple Silicon (MPS) | ~1-2s | — |
| **纯 CPU** | **8-15s** ⚠️ | — |

**精度**：OmniParser+GPT-4o 在 ScreenSpot Pro（高分辨率小图标）39.6%，而 GPT-4o 裸测仅 0.8%——**证明"解析器+SoM"对 grounding 的巨大增益**；但 39.6% 的绝对值同时警示：**纯视觉在高分辨率密集 UI 下远未可靠，a11y 优先不可替代**。

**UFO² 双源融合算法**（直接可照搬）：
1. UIA 检测的控件作为主列表（全部保留）
2. OmniParser 检测的控件作为补充列表
3. 逐个计算与 UIA 控件的 IoU，> 0.1 判定重复丢弃
4. 未重复的补充进合并列表 → 最大覆盖 + 最小冗余

**OmniTool 参考部署**：`omniparserserver`（FastAPI，GPU 机）/ `omnibox`（Win11 VM docker）/ `gradio`（展示 UI）三层分离——验证"解析服务与执行环境分离"的 sidecar 模式可行。

### 2.3 其它参考实现

- **FlaUI-MCP**（shanselman）：C# MCP server，像 Playwright `browser_snapshot` 一样给 Windows 控件发元素 ref，`click ref=` 元素级操作——**证明 Windows a11y-first 工具形态可直接复用 Playwright 交互范式**
- **AutoGUI**：a11y-first clicking + SoM 兜底 + 每应用 quirks 记忆库——与本项目演化/经验记忆方向契合（后续可接 case 记忆）
- **ios-automation-mcp**：MCP 形态封装 Appium XCUITest/WDA（observe/tap/type/swipe/scroll/wait）——P3 iOS sidecar 的协议参考

---

## 3. OmniParser V2 对本方案的 7 条启发

| # | 启发 | 落地到本方案 |
|---|------|------------|
| H1 | **统一元素模型**：`{type, content, bbox, interactivity, source}` 契约通用 | 作为 Perception 层四源（UIA/AT-SPI/WDA/OmniParser）归一后的统一 `UIElement` 模型 |
| H2 | **双源 IoU 去重融合**（UFO²） | Grounding 融合器实现 a11y 主表 + 视觉补充表 + IoU>0.1 去重 |
| H3 | **interactivity 可交互性标志** | 元素模型保留该字段，过滤不可交互元素，减少 LLM 误点 |
| H4 | **延迟分层**：CPU 8-15s 不可用 | 视觉解析**绝不进关键路径**；a11y 命中零开销；OmniParser 仅异步/按需，且可指向远程 GPU 服务 |
| H5 | **部署边界**：Python/torch 技术栈与 Go 生态差异大 | OmniParser 以**独立 HTTP 服务**部署（omniparserserver 模式），进程边界隔离技术栈与依赖，可弹性调度到 GPU 机器；许可不作为选型约束（用户已决策），但服务化架构同时天然规避任何静态链接问题 |
| H6 | **ScreenSpot Pro 39.6% 的警示** | 视觉兜底必须配 zoom/crop 局部放大（借鉴 Claude zoom action），dense UI 先裁剪再判 |
| H7 | **OmniTool 三层分离** | 采用同样的 sidecar 分层：Go 核心 ↔ 设备 agent sidecar ↔ 展示层，协议标准化 |

---

## 4. 方案设计

### 4.1 总体架构

```
┌──────────────────────────── Aranea-Agents (Go) ───────────────────────────┐
│  Agent 运行时（Chat/Team）                                                  │
│     ↓ 工具调用                                                             │
│  internal/tools/computeruse/   ← 注册进工具装配（Registry+seed，场景2五步）  │
│     ↓                                                                     │
│  internal/biz/computeruse/     ← 会话/任务/预算/安全策略 + port 接口        │
│     ↓ DeviceGateway port                                                   │
│  internal/computeruse/         ← sidecar 进程管理 + CDP 客户端 +           │
│                                  Grounding 融合器 + SoM 标注器             │
└──────┬───────────────────────────────────────────┬───────────────────────┘
       │ CDP（JSON-RPC over stdio/回环HTTP）         │ HTTP（可选，GPU 机器）
┌──────▼─────────────┐                    ┌─────────▼──────────────┐
│ 设备 agent sidecar  │                    │ omniparserserver        │
│ P1: aranea-cua-win │                    │ (OmniParser V2 FastAPI, │
│  (.NET8+FlaUI)     │                    │  AGPL 隔离, 可远程GPU)   │
│ P2: aranea-cua-lnx │                    └─────────────────────────┘
│  (Python+pyatspi)  │
│ P3: aranea-cua-ios │
│  (macOS 宿主+WDA)  │
└────────────────────┘
```

**为什么是 sidecar 而非纯 Go**：UIA 最成熟绑定在 .NET（FlaUI），AT-SPI 在 Python（pyatspi），WDA 桥在 macOS 生态——各平台用其最强原生栈，Go 核心只通过标准化 **CDP 协议**驱动。这也天然满足 H5 的许可隔离与平台演进独立。

### 4.2 CDP（Computer-use Device Protocol）契约

sidecar 暴露统一 JSON-RPC 方法集（P1 全集，P2/P3 同构实现）。**P1 传输固定为 stdio JSON-RPC**（sidecar 作为子进程由 Go 核心拉起，免端口管理、天然单机隔离）；P3 iOS 因跨主机（macOS 宿主）使用回环/局域网 HTTP。

| 方法 | 语义 | 关键参数/返回 |
|------|------|--------------|
| `device.info` | 设备/屏幕/DPI 信息 | scaleFactor、物理分辨率、平台 |
| `perception.snapshot` | 全量感知 | 返回 a11y 元素树（统一 UIElement[]）+ 截图引用 |
| `perception.screenshot` | 截图（可选区域/缩放） | region、zoom → PNG bytes |
| `action.invoke` | **元素级直调**（UIA Invoke 等，无坐标） | elementRef |
| `action.click/wheel/drag` | 坐标级输入 | 物理像素坐标 |
| `action.type/key` | 文本/按键注入 | text、keycombo |
| `window.list/focus` | 窗口枚举/聚焦 | title 正则、hwnd |
| `app.launch` | 启动应用 | path/名称 |

**统一元素模型**（H1）：
```
UIElement{ ref, type, name, bbox(物理像素), interactivity, source(uia|atspi|wda|vision), appName, children? }
```

### 4.3 Grounding 决策流（快速精确的核心）

```
工具调用 computer_use.act(target: "保存按钮")
  → 1. a11y 快路径：在 snapshot 元素树按 name/type 模糊匹配（归一化+编辑距离）
       命中 → action.invoke(elementRef)      【零模型调用，P95 <300ms】
  → 2. miss → 视觉兜底：
       a. 截图 + （可选）OmniParser 解析 → IoU 融合（H2）
       b. SoM 编号标注图 + 元素列表 → VLM（Claude/Qwen-VL，走 LLM catalog 配置）选元素 ID
       c. dense UI 时先 zoom 裁剪再判（H6）
       → bbox 中心点坐标点击               【P95 <3.5s】
  → 3. 执行后感知校验：局部截图比对预期（可选），失败重试 ≤2 次后降级报用户
```

### 4.4 工具集设计（挂进工具装配）

| 工具 key | 类型 | 说明 | 确认门级别 |
|---------|------|------|-----------|
| `computer_use.observe` | 只读 | 感知快照（元素树摘要 + 可选截图） | 免确认 |
| `computer_use.screenshot` | 只读 | 截图（返回多模态图像） | 免确认 |
| `computer_use.act` | 写 | 语义动作（target 描述 → grounding → 执行） | **需授权**（复用 tool-grants 持久化） |
| `computer_use.launch` | 写 | 启动/聚焦应用 | 需授权 |
| `computer_use.session.start/stop` | 管理 | 显式会话生命周期（绑定预算与安全策略） | 免确认 |

Chat 与 Team 两种编排共用 BuildToolsets，自动生效（交叉参考场景 2）。

### 4.5 安全模型

| 机制 | 设计 |
|------|------|
| 确认门 | `computer_use.act` 默认需授权；复用现有 tool-grants 持久化授权（`grant_persisted` 短路） |
| 动作分级 | observe/screenshot 只读免确认；act/launch 需授权；**高危语义**（删除/支付/发送类按钮文本命中敏感词表）强制人工确认卡，不可持久豁免 |
| 禁区 | 进程黑名单（密码管理器/银行 U 盾窗口前置时拒绝 act）；可选屏幕区域黑名单 |
| 预算 | 每任务 `max_steps`（默认 50）+ 墙钟超时；超限自动终止并报告 |
| 干跑 | `dry_run` 模式只规划与标注，不注入输入 |
| 急停 | 全局 kill switch：热键 + API + 前端按钮，sidecar 看门狗心跳丢失即停 |
| 建议运行环境 | 高危/评测任务在 VM 内（类 L3 隔离），快照可回滚 |

### 4.6 可观测性

- **流程日志**（TraceEmitter）：新增 step_id `computeruse.session.start/act/grounding.fallback/act.done/error` 等，全部登记 `stepTitleRegistry` + 同步 52-flow-logger.design.md §5.1（红线）
- **进程日志**：loggateway 构造注入，K1-K7 覆盖（sidecar 启停/panic 必须各有一条）
- **审计**：每次 act 落库（任务、目标元素、grounding 路径、前后截图引用、耗时、确认人）——activities 事件分级走 Important
- **事件**：新增 envelope `computeruse.step`（场景 4 五步），前端实时渲染步骤流 + 截图
- **审计存储**：新 Ent Schema `computer_use_audit`（L1 自动迁移，不加索引特性则免 DDL 迁移）

### 4.7 性能预算（验收标准）

| 指标 | 目标 |
|------|------|
| a11y 路径端到端（工具调用→动作完成） | **P95 < 300ms** |
| 视觉兜底路径（含一次 VLM 调用，OmniParser 走 GPU 服务） | P95 < 3.5s |
| 元素级动作坐标精度 | 100% 控件中心（invoke 无坐标） |
| 视觉路径落点命中率（目标 bbox 内） | ≥ 95%（SoM+zoom） |
| 观察流推送 | ≥ 2 fps（差分推送，16ms 批合并复用 streaming 机制） |

### 4.8 与现有模块集成点（交叉参考已核对）

| 模块 | 集成方式 |
|------|---------|
| `internal/tools`（场景 2 五步） | Registry 注册 + builtin_tools_seed + Assemble；Chat/Team 双验证 |
| 确认门 tool-grants | act 工具走既有授权链，grant_persisted 短路 |
| `internal/event` | 新 envelope 类型（场景 4 五步）；流程日志 step 登记 |
| LLM catalog/Provider | 视觉 grounding 模型走既有 catalog 配置（支持 Claude/Qwen-VL），不另建模型栈 |
| `internal/mcp`（P3） | iOS sidecar 以 MCP server 形态被托管（健康检查/探针复用） |
| 前端 | ToolsPage 自动展示；会话步骤流复用 realtime envelope dispatcher |
| 文档 | 新建 `75-computer-use` 三件套（需求/设计/开发计划），实施时同步 |

---

## 5. 里程碑计划

### P1 — Windows（核心，本方案实施范围）

| 里程碑 | 内容 | 验收 |
|--------|------|------|
| M1.1 | CDP 协议定义 + `aranea-cua-win` sidecar（.NET8+FlaUI）：device.info / snapshot(UIA) / screenshot / invoke / click / type / key / window.focus | sidecar 单测绿；手工驱动记事本全流程 |
| M1.2 | Go 核心：`biz/computeruse`（会话/预算/安全策略+port）+ `internal/computeruse`（进程管理+CDP client+a11y 匹配器）+ 工具注册 5 件套 | 单测绿；Chat 内 Agent 完成"打开记事本→输入→保存" |
| M1.3 | 视觉兜底：SoM 标注器 + VLM 直判（catalog 配置）+ zoom 裁剪；OmniParser HTTP 客户端（可选配置） | 自绘窗口（Electron 应用）场景命中 ≥95% |
| M1.4 | 安全门（分级/禁区/预算/急停）+ 审计落库 + 流程日志 + envelope 事件 + 前端步骤流最简视图 | 确认卡四按钮全通；日志双轨齐；真机演示 |

### P2 — Linux

`aranea-cua-lnx` sidecar（Python：pyatspi 感知 + xdotool/ydotool 执行 + grim/import 截图），CDP 同构；Wayland/X11 双后端自动探测。

### P3 — iOS 模拟器

macOS 宿主 `aranea-cua-ios`（桥接 WDA :8100 + simctl），以 MCP server 形态接入 `internal/mcp` 托管；仅承诺模拟器，真机视需求另议（需 Apple 开发者签名链路）。

---

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| Python/torch 依赖链重（OmniParser 部署复杂） | 中 | 独立 HTTP 服务 + 一键部署脚本（uv 锁版本）；未就绪时自动降级 VLM 直判路径 |
| 目标机无 GPU，OmniParser CPU 8-15s | 高 | 视觉解析不进关键路径（H4）；a11y 覆盖 80-95% 场景；OmniParser 可指远程 GPU |
| UIA 盲区（Electron/自绘/游戏） | 中 | 视觉兜底 + zoom（H6）；已知应用 quirks 记忆（后续接 case 记忆层） |
| DPI 多屏缩放坐标错乱 | 中 | sidecar 声明 per-monitor DPI aware；协议统一物理像素；截图带 scaleFactor |
| 误操作造成真实损害 | 高 | 安全模型 §4.5 全量；评测/演示一律 VM 快照 |
| sidecar 崩溃/僵死 | 中 | 看门狗心跳 + 自动重启 + 任务失败回落报告 |

---

## 7. 决策记录（ADR 要点）

- **ADR-CU-01**：采用 sidecar + CDP 协议而非纯 Go 直连 UIA —— 各平台用最强原生栈；技术栈隔离；独立演进。代价：多一层进程管理。
- **ADR-CU-02**：a11y-first + 视觉兜底，而非纯视觉 —— "快速精确"的唯一工程解（§1.3、H6）。
- **ADR-CU-03**：Windows sidecar 用 .NET8+FlaUI 而非 pywinauto —— UIA 绑定最成熟、单文件发布、长期维护性（FlaUI-MCP 已验证形态）。
- **ADR-CU-04**：iOS 仅承诺模拟器 —— 真机需 macOS+Xcode 签名链路，复杂度与收益不匹配（用户已确认）。

## 8. 参考来源

- Claude Computer Use 架构解析（callsphere.ai）；Anthropic computer-use 官方文档
- CUA State of the Art June 2026（github.com/redDwarf03/mine-jepa）
- Computer Use and GUI Agents in 2026（zylos.ai）
- 微软 UFO² Hybrid Control Detection 官方文档；OmniParser V2 官方发布（microsoft.com/research）与 GitHub/HF 模型卡
- FlaUI-MCP（github.com/shanselman/FlaUI-MCP）；AutoGUI（billmongan.com）
- Linux GUI 控制 API 指南（fazm.ai）；WDA（github.com/appium/WebDriverAgent）；ios-automation-mcp（lobehub）
- Awesome Computer Use（github.com/TianyuanYang/awesome-computer-use）

## 9. 后续动作

1. 评审本方案 → 通过后新建 `docs/development/75-computer-use{,.design,.development}.md` 三件套
2. 按 M1.1-M1.4 实施（TDD），完成后 aranea-review 全栈审查
3. 真机测试：本机 Windows 实测（日志 + UI 行为验证，遵守修改前必查 R3）

---

## 附录：2026-08-13 二次对标评估与方案实施结论

> 在 M1.1-M1.5 落地后，按"对标市面上最强 CUA 能力"重新评估差距、罗列方案并实施。结论：选定**方案 A（混合架构增强）**并已实施完毕（Phase M2，全量 TDD + 真机 E2E 通过）。

### A.1 差距评估（对标 Claude Computer Use / OpenAI Operator / UFO² / UI-TARS）

| 能力维度 | 市场最佳实践 | 评估前本实现 | 差距 |
|---------|-------------|-------------|------|
| grounding 精度 | a11y+视觉混合（UFO² hybrid detection） | a11y 单路径，视觉兜底未接真实组件 | 视觉链路未通（无 OmniParser 实例、无 VLM 模型配置） |
| 失败安全 | 无匹配明确报错（Anthropic 建议拒绝乱点） | VLM 强制选择——目标不存在时乱选元素乱点 | **安全缺陷**（高危） |
| 执行可信度 | 动作后验证（settle+re-check）闭环 | 执行后无验证，no_effect 不可知 | 缺验证闭环 |
| 多步效率 | 批量动作（Claude tool batching） | 单步调用，每步一次全链路 | 多步任务延迟高 |
| 视觉模型 | 本地+云端双轨（成本/精度弹性） | 未配置任何视觉模型 | 无可用 VLM |
| 上下文工程 | 截图降采样控制 token | 全尺寸截图直发，Ollama 默认 4096 ctx 直接超限 | 大图调用必败 |

### A.2 方案罗列与评估

| 方案 | 内容 | 评估 |
|------|------|------|
| **A 混合架构增强（选定）** | 保持 a11y-first；补齐视觉链路（OmniParser 本机 GPU + 本地/云端双 VLM）、fallback 链、执行后验证、批量动作、无匹配出口 | 与市场最佳架构同构（UFO² 混合检测 + Anthropic 验证闭环）；复用既有代码，增量可控；本地模型牺牲部分精度换零成本/隐私，云端模型随时可切 |
| B 纯视觉端到端（UI-TARS 自部署） | 弃 a11y，全量视觉 grounding | OSWorld 实测精度仍低于混合方案；7B 级模型每次动作一次推理，延迟与 GPU 成本不可接受；推翻既有 a11y 投入 |
| C 云端托管 CUA API（Claude Computer Use / Operator） | grounding+执行全托管 | 数据出域（桌面截图含敏感信息，不可接受）；按 token 计费持续成本；与本平台 sidecar 安全模型（禁区/确认门/预算）无法集成 |

### A.3 实施结论（方案 A，Phase M2 全部完成）

- **视觉链路**：OmniParser V2 本机 GPU 部署（`:8101`，HF 离线权重）；VLM 双轨——本地 `ollama/qwen2.5vl-cua`（num_ctx 8192 派生模型）+ 云端 `alibaba-cn/qwen3-vl-plus`（catalog 已建行，待 API key）
- **grounding fallback 链**：a11y → SoM（OmniParser+IoU 融合+VLM 选编号）→ VLM 坐标直判（zoom 精化），逐级降级各有流程日志
- **失败安全**：VLM 无匹配出口（SoM 输出 0 / 直判输出 -1,-1 → ErrGroundingFailed），消除"目标不存在时乱点屏幕"缺陷
- **执行后验证**：settle → re-snapshot → 元素树 hash + 前台窗口检查，verify 透出供 LLM 决策
- **批量动作**：`computer_use.act` 支持 actions[] 按序 fail-fast，错误注明已完成步数防整体重试
- **运行时加固**：截图降采样 ≤1568px（视觉 token 减半、prefill 提速）；VLM 超时 60s 容忍本地冷启动
- **E2E 证据**：记事本场景 a11y/vision 双路径真实命中，批量 2 步全 OK，无匹配明确报错（详见 75-computer-use.development.md Phase M2）

**遗留**：云端 qwen3-vl-plus 启用待用户提供 dashscope API key；UI-TARS 专用 grounding 模型为后续演化方向（方案 B 的可用部分）。
