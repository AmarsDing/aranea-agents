# Aranea-Agents 内置工具与知识图谱/记忆可视化建设指南

> 基于 2026-05-31 竞品深度调研，对标 OpenClaw 55 个内置 Skills + Hermes ~70 个内置 Tools。
> 为 Aranea-Agents 的内置工具建设和知识图谱/记忆可视化提供代码级实施指导。

---

## 一、竞品内置工具全景

### 1.1 OpenClaw 内置工具体系

OpenClaw 的工具体系分为两层：**Tool（能力器官）** + **Skill（操作指南）**。

#### Tool 层（6 个核心工具，始终可用）

| Tool | 功能 | 对标 Aranea |
|------|------|------------|
| `browser` | 真实 Chromium 浏览器自动化（25+ action） | ⚠️ Aranea 仅 MCP 配置 |
| `exec_command` | Shell 命令执行（前台/后台/PTY/超时/环境注入） | ⚠️ Aranea 有 `hostexec` 但 Factory 为 nil |
| `read_document` | 读取 PDF/DOCX/纯文本上传文件 | ❌ Aranea 无此工具 |
| `read_spreadsheet` | 读取 XLSX/CSV 上传文件 | ❌ Aranea 无此工具 |
| `write_stdin` | 向运行中进程写入 stdin | ❌ Aranea 无此工具 |
| `kill_session` | 终止运行中的 exec_command 会话 | ❌ Aranea 无此工具 |

#### Skill 层（55 个内置 Skills，按需加载）

按功能域分类：

| 域 | Skills | 数量 |
|----|--------|------|
| **通信** | discord, slack, bluebubbles(iMessage), himalaya(邮件), wacli(WhatsApp), imsg(iMessage), voice-call | 7 |
| **效率** | gog(Google Workspace), notion, obsidian, bear-notes, apple-notes, apple-reminders, things-mac, trello | 8 |
| **开发** | github, gh-issues, coding-agent(Codex/Claude Code), clawhub, mcporter(MCP), tmux | 6 |
| **搜索/内容** | http_get, summarize, blogwatcher, session-logs | 4 |
| **多媒体** | openai-image-gen, nano-banana-pro(图片生成), nano-pdf, video-frames, gifgrep, songsee, camsnap | 7 |
| **语音** | sag(TTS/ElevenLabs), openai-whisper(STT本地), openai-whisper-api(STT云端), sherpa-onnx-tts(本地TTS) | 4 |
| **智能家居** | openhue(Philips Hue), eightctl(Eight Sleep), blucli(BluOS), sonoscli(Sonos) | 4 |
| **生活** | weather, goplaces, ordercli(外卖), spotify-player | 4 |
| **社交** | xurl(X/Twitter) | 1 |
| **安全/系统** | healthcheck, envdump, oracle | 3 |
| **创意** | canvas(HTML展示), skill-creator | 2 |
| **AI** | gemini(Gemini CLI) | 1 |
| **密码管理** | 1password | 1 |
| **macOS 专属** | peekaboo(macOS UI自动化) | 1 |
| **模型用量** | model-usage(CodexBar) | 1 |

**Skill Eligibility 五维检查**（OpenClaw 核心差异化）：

1. **OS 检查**：`evaluateOpenClawOS(meta.OS)` —— macOS 专属 Skill 只在 darwin 上启用
2. **二进制依赖**：`evaluateRequiredBins(meta.Requires.Bins)` —— `exec.LookPath` 搜索
3. **任一二进制**：`evaluateRequiredAnyBins` —— 满足任一即可（如 `spogo` OR `spotify_player`）
4. **环境变量**：`evaluateRequiredEnv` —— 含 `SkillConfig.APIKey` 回退
5. **配置键**：`evaluateRequiredConfig` —— 前缀匹配

### 1.2 Hermes 内置工具体系

Hermes 的工具体系按 toolset 组织，共 **~70 个工具**：

| Toolset | 工具数 | 代表工具 | 对标 Aranea |
|---------|--------|---------|------------|
| `browser` | 10+2 | navigate/click/type/scroll/snapshot/screenshot/vision/back/press/console + CDP(dialog/cdp) | ❌ Aranea 无浏览器工具 |
| `file` | 4 | read/write/search/edit | ✅ Aranea 有 `file` 工具 |
| `terminal` | 2 | execute/process | ⚠️ Aranea 有 `hostexec` 但未实现 |
| `web` | 2 | search/extract | ⚠️ Aranea 有 `duckduckgo`/`httpfetch` |
| `memory` | 1 | memory（跨会话持久记忆） | ✅ Aranea 有记忆工具 |
| `session_search` | 1 | 搜索过去会话 | ❌ Aranea 无此工具 |
| `clarify` | 1 | 向用户提问澄清 | ⚠️ Aranea 有 `await_user_reply` |
| `code_execution` | 1 | execute_code（Python 沙箱） | ⚠️ Aranea 有 Docker 沙箱 |
| `delegation` | 1 | delegate_task（子任务委托） | ✅ Aranea 有 subagent |
| `cronjob` | 1 | 定时任务管理 | ✅ Aranea 有 CronRunner |
| `messaging` | 1 | 跨平台消息发送 | ⚠️ Aranea 有 `message` 但 Factory nil |
| `todo` | 1 | 任务计划跟踪 | ✅ Aranea 有 `todo` |
| `vision` | 1 | vision_analyze（图像分析） | ❌ Aranea 无此工具 |
| `image_gen` | 1 | image_generate（AI 图像生成） | ❌ Aranea 无此工具 |
| `tts` | 1 | text_to_speech | ❌ Aranea 无此工具 |
| `homeassistant` | 4 | 智能家居控制 | ❌ Aranea 无此工具 |
| `spotify` | 7 | Spotify 播放控制 | ❌ Aranea 无此工具 |
| `feishu` | 5 | 飞书集成 | ❌ Aranea 无此工具 |
| `yuanbao` | 5 | 腾讯元宝集成 | ❌ Aranea 无此工具 |
| `kanban` | 7 | 看板任务管理 | ✅ Aranea 有 kanban 工具 |
| `discord` | 2 | Discord 操作 | ❌ Aranea 无此工具 |
| `rl` | 10 | 强化学习工具 | ❌ Aranea 无此工具 |
| `moa` | 1 | mixture_of_agents | ❌ Aranea 无此工具 |
| `skill_view/manage/list` | 3 | 技能浏览/管理 | ✅ Aranea 有 Skill 系统 |

---

## 二、Aranea 现有工具差距分析

### 2.1 当前注册工具清单（25 个注册 + 7+ CustomTool）

| # | 名称 | 类别 | 风险 | 默认启用 | Factory | 差距 |
|---|------|------|------|----------|---------|------|
| 1 | `file` | filesystem | low | ✅ | ✅ | — |
| 2 | `hostexec` | execution | critical | ❌ | **nil** | 🔴 需实现 |
| 3 | `httpfetch` | web | medium | ❌ | ✅ | — |
| 4 | `claudefetch` | web | medium | ❌ | **nil** | 🟡 stub |
| 5 | `geminifetch` | web | medium | ❌ | **nil** | 🟡 需配置 |
| 6 | `duckduckgo` | search | medium | ❌ | ✅ | — |
| 7 | `google_search` | search | medium | ❌ | **nil** | 🟡 需配置 |
| 8 | `arxiv_search` | search | low | ❌ | ✅ | — |
| 9 | `wikipedia` | search | low | ❌ | ✅ | — |
| 10 | `email` | communication | high | ❌ | ✅ | — |
| 11 | `message` | communication | high | ❌ | **nil** | 🔴 需实现 |
| 12 | `todo` | productivity | low | ❌ | ✅ | — |
| 13 | `await_user_reply` | interaction | low | ❌ | ✅ | — |
| 14 | `claudecode` | coding | critical | ❌ | ✅ | — |
| 15 | `workspace_exec` | execution | critical | ❌ | **nil** | 🔴 需实现 |
| 16 | `openapi` | integration | medium | ❌ | ✅(err) | 🟡 需配置 |
| 17 | `browser` | browser | critical | ❌ | **nil** | 🔴 需实现 |
| 18 | `mcp` | integration | medium | ❌ | 动态 | — |
| 19 | `mcpbroker` | integration | medium | ❌ | 动态 | — |
| 20 | `subagents_*` | composition | medium | ❌ | 动态 | — |
| Custom | `knowledge_search/reflect` | knowledge | low | — | ✅ | — |
| Custom | `web_research` | web | medium | — | ✅ | — |
| Custom | `call_agent` | a2a | medium | — | ✅ | — |
| Custom | `kanban` | productivity | medium | — | ✅ | — |
| Custom | `memory.*` | memory | low | — | ✅ | — |

### 2.2 与竞品的核心差距

| 差距 | 严重度 | OpenClaw | Hermes | Aranea 现状 |
|------|--------|----------|--------|------------|
| **浏览器自动化** | 🔴 P0 | 25+ action + 双驱动 | 10+ action + CDP | 仅 MCP 配置 |
| **Shell 执行** | 🔴 P0 | exec_command + write_stdin + kill_session | terminal + process | hostexec Factory nil |
| **文档读取** | 🔴 P0 | read_document + read_spreadsheet | file.read | 无专用文档工具 |
| **消息发送** | 🟠 P1 | message(多渠道) | messaging(多平台) | message Factory nil |
| **图像生成** | 🟠 P1 | openai-image-gen + nano-banana-pro | image_generate | 无 |
| **语音 TTS/STT** | 🟠 P1 | sag + whisper + sherpa-onnx-tts | text_to_speech | 无 |
| **视觉分析** | 🟠 P1 | browser_vision | vision_analyze | 无 |
| **会话搜索** | 🟡 P2 | session-logs | session_search | 无 |
| **Skill 生态** | 🟡 P2 | 55 内置 + ClawHub 5700+ | 40 内置 + agentskills.io | 无内置 Skills |

---

## 三、Aranea 内置工具建设方案

### 3.1 设计原则

基于 Aranea 的产品定位（一人开发公司的统一应用），内置工具建设遵循：

1. **框架优先**：优先使用 `pkg/trpc-agent-go/` 已有能力，不重复造轮子
2. **渐进式**：按 P0/P1/P2 优先级逐步实现，每个工具独立可用
3. **Eligibility 驱动**：移植 OpenClaw 的五维检查，自动评估工具可用性
4. **安全分级**：critical 工具需确认，medium 工具需配置，low 工具默认启用

### 3.2 P0 — 核心工具（对标竞品必备能力）

#### P0-1：Shell 执行工具组（exec_command + write_stdin + kill_session）

**对标**：OpenClaw `openclaw/internal/octool/`

**实现方案**：移植 OpenClaw 的 octool 三件套

| 工具 | 功能 | 实现方式 |
|------|------|---------|
| `exec_command` | Shell 命令执行（前台/后台/PTY/超时/环境注入） | 移植 `octool/exec_command`，适配 Kratos kerrors + safego |
| `write_stdin` | 向运行中进程写入 stdin | 移植 `octool/write_stdin` |
| `kill_session` | 终止运行中 exec_command 会话 | 移植 `octool/kill_session` |

**安全策略**（必须同步移植）：

| 策略 | 来源 | 说明 |
|------|------|------|
| `CommandPolicy` | `octool/policy.go` | 阻止访问 `.ssh/`、`.aws/credentials`、`.kube/config` 等敏感路径 |
| `OutputRedactor` | `octool/redaction.go` | 从输出中移除敏感值 |
| 环境变量注入 | `octool/tools.go` | 注入 `OPENCLAW_SESSION_UPLOADS_DIR` 等 16 个环境变量 |

**涉及文件**：

```
internal/tools/hostexec/
  ├── service.go          ← 移植 octool/manager.go + session.go
  ├── exec_command.go     ← 移植 octool NewExecCommandTool
  ├── write_stdin.go      ← 移植 octool NewWriteStdinTool
  ├── kill_session.go     ← 移植 octool NewKillSessionTool
  ├── policy.go           ← 移植 octool/policy.go
  └── redaction.go        ← 移植 octool/redaction.go
```

**验收标准**：
- [ ] 可执行 Shell 命令（前台/后台模式）
- [ ] 可向运行中进程写入 stdin
- [ ] 可终止运行中会话
- [ ] 敏感路径访问被阻止
- [ ] 输出中敏感值被脱敏
- [ ] 上传文件路径自动注入环境变量

---

#### P0-2：文档读取工具组（read_document + read_spreadsheet）

**对标**：OpenClaw `octool/read_document` + `octool/read_spreadsheet`

**实现方案**：基于框架已有的 `knowledge/document/reader/pdf` 扩展

| 工具 | 功能 | 实现方式 |
|------|------|---------|
| `read_document` | 读取 PDF/DOCX/纯文本上传文件 | 使用框架 `pdf` reader + 扩展 DOCX/TXT |
| `read_spreadsheet` | 读取 XLSX/CSV 上传文件 | 新建，使用 `excelize` 库 |

**涉及文件**：

```
internal/tools/document/
  ├── service.go          ← 文档读取服务
  ├── read_document.go    ← PDF/DOCX/TXT 读取工具
  └── read_spreadsheet.go ← XLSX/CSV 读取工具
```

**验收标准**：
- [ ] 可读取 PDF 文件（含分页）
- [ ] 可读取 DOCX 文件
- [ ] 可读取 XLSX/CSV 文件（含按行/范围读取）
- [ ] 上传文件自动关联到当前会话

---

#### P0-3：浏览器工具

**对标**：OpenClaw `openclaw/internal/browser/`（25+ action）

**实现方案**：分两阶段

**阶段 1**（MVP）：集成框架 MCP toolset + Playwright MCP

```
internal/tools/browser/
  ├── config.go           ← 已有，PlaywrightMCPConfig
  └── mcp_driver.go       ← MCP Profile Driver 实现
```

**阶段 2**（完整）：移植 OpenClaw 浏览器工具

```
internal/tools/browser/
  ├── tool.go             ← 移植 openclaw/browser/tool.go
  ├── driver.go           ← 移植 openclaw/browser/driver.go
  ├── server_driver.go    ← Browser Server Driver
  ├── navigation_guard.go ← 导航安全策略
  └── profiles.go         ← Profile 管理
```

**验收标准**：
- [ ] MVP：通过 MCP 连接 Playwright，支持 navigate/click/type/screenshot
- [ ] 完整：25+ action，双驱动，导航安全策略

---

### 3.3 P1 — 增强工具

#### P1-1：消息发送工具（message）

**对标**：OpenClaw `message` 工具（多渠道）+ Hermes `messaging` 工具

**实现方案**：基于已有 `internal/outbound/` 扩展

| 功能 | 实现方式 |
|------|---------|
| 文本消息 | 复用 `outbound.TextSender` |
| 文件消息 | 新增 `outbound.MessageSender` 的文件发送 |
| 语音消息 | 移植 OpenClaw `as_voice` 参数 |
| 多渠道路由 | 复用 `outbound.Router` |

**涉及文件**：

```
internal/tools/message/
  └── service.go          ← 消息发送工具，桥接 outbound.Router
```

---

#### P1-2：图像生成工具

**对标**：OpenClaw `openai-image-gen` + `nano-banana-pro` | Hermes `image_generate`

**实现方案**：基于 Provider 层的模型调用

| 功能 | 实现方式 |
|------|---------|
| OpenAI DALL-E | 通过 `internal/provider` 调用 OpenAI Images API |
| Gemini 图片生成 | 通过 `internal/provider` 调用 Gemini 图片生成 API |

**涉及文件**：

```
internal/tools/imagegen/
  └── service.go          ← 图像生成工具
```

---

#### P1-3：语音工具（TTS + STT）

**对标**：OpenClaw `sag`/`whisper`/`sherpa-onnx-tts` | Hermes `text_to_speech`

**实现方案**：基于 Provider 层 + 本地模型

| 功能 | 实现方式 |
|------|---------|
| TTS（云端） | 通过 `internal/provider` 调用 OpenAI/Edge TTS API |
| STT（云端） | 通过 `internal/provider` 调用 OpenAI Whisper API |
| TTS（本地） | 集成 `sherpa-onnx` 离线 TTS |

**涉及文件**：

```
internal/tools/speech/
  ├── tts.go              ← TTS 工具
  └── stt.go              ← STT 工具
```

---

#### P1-4：视觉分析工具

**对标**：Hermes `vision_analyze` + OpenClaw `browser_vision`

**实现方案**：基于 Provider 层的 Vision 模型

| 功能 | 实现方式 |
|------|---------|
| 图片分析 | 通过 `internal/provider` 调用 Vision 模型（GPT-4o/Gemini） |
| OCR | 集成 `internal/knowledge/ocr.go` 的真实实现（替换 stub） |

**涉及文件**：

```
internal/tools/vision/
  └── service.go          ← 视觉分析工具
```

---

### 3.4 P2 — 生态工具

#### P2-1：会话搜索工具

**对标**：OpenClaw `session-logs` | Hermes `session_search`

**实现方案**：基于已有 `internal/biz` 的 Session/Message 查询接口

```
internal/tools/sessionsearch/
  └── service.go          ← 会话搜索工具
```

---

#### P2-2：内置 Skills 库

**对标**：OpenClaw 55 个内置 Skills | Hermes 40+ 内置 Skills

**实现方案**：分三步

1. **移植 OpenClaw Skills**：从 `pkg/trpc-agent-go/openclaw/skills/` 移植通用 Skills
2. **实现 Eligibility**：移植五维检查
3. **新增 Aranea 专属 Skills**：基于 Aranea 独有能力（Graph/Team/A2A）编写

**推荐首批移植的 Skills**（通用性强、依赖少）：

| # | Skill | 来源 | 依赖 | 理由 |
|---|-------|------|------|------|
| 1 | `weather` | OpenClaw | curl | 零配置，高频使用 |
| 2 | `summarize` | OpenClaw | summarize CLI | 高频使用 |
| 3 | `http_get` | OpenClaw | bash+curl | 基础能力 |
| 4 | `github` | OpenClaw | gh | 开发者必备 |
| 5 | `coding-agent` | OpenClaw | claude/codex | 开发者必备 |
| 6 | `notion` | OpenClaw | API Key | 效率工具 |
| 7 | `gog` | OpenClaw | gog CLI | Google Workspace |
| 8 | `himalaya` | OpenClaw | himalaya CLI | 邮件管理 |
| 9 | `tmux` | OpenClaw | tmux | 终端管理 |
| 10 | `skill-creator` | OpenClaw | 无 | Skill 自创建基础 |
| 11 | `healthcheck` | OpenClaw | 无 | 安全检查 |
| 12 | `envdump` | OpenClaw | bash | 环境信息 |
| 13 | `nano-pdf` | OpenClaw | nano-pdf CLI | PDF 编辑 |
| 14 | `video-frames` | OpenClaw | ffmpeg | 视频处理 |
| 15 | `openai-whisper-api` | OpenClaw | curl+API Key | 语音转文字 |

**Aranea 专属 Skills**（基于独有能力）：

| # | Skill | 功能 | 理由 |
|---|-------|------|------|
| A1 | `graph-workflow` | 创建/编辑/运行 Graph 工作流 | Aranea 独有 |
| A2 | `team-orchestrate` | 创建/编排 Team 协作 | Aranea 独有 |
| A3 | `a2a-discover` | 发现和调用远程 A2A Agent | Aranea 独有 |
| A4 | `knowledge-rag` | 知识库检索与注入 | Aranea 独有 |
| A5 | `memory-recall` | 跨层记忆检索 | Aranea 独有 |

---

## 四、知识图谱与记忆可视化建设指南

### 4.1 竞品知识图谱/记忆可视化实现

#### OpenClaw 的做法

**记忆可视化**：基于 MEMORY.md 文件

- 用户可直接查看和编辑 `MEMORY.md` 文件
- 三段式模板：Long-term facts / Preferences / Repeated working style
- 文件型存储，用户可见、可编辑、可版本控制
- Gateway 层自动注入 memory 内容到 Agent prompt

**知识图谱**：无独立知识图谱可视化

- OpenClaw 没有知识图谱概念
- 记忆仅通过 MEMORY.md 文件和 FTS5 全文搜索管理
- 依赖 LLM 自行从 MEMORY.md 中检索和推理

#### Hermes 的做法

**记忆可视化**：基于 FTS5 + LLM 摘要

- `memory` 工具：跨会话持久记忆存储
- `session_search` 工具：搜索过去会话
- 记忆内容通过 `nudge` 机制自动沉淀
- 用户可通过 CLI 命令查看/编辑记忆

**知识图谱**：无独立知识图谱可视化

- Hermes 没有结构化知识图谱
- 记忆以文本形式存储，通过 FTS5 全文搜索检索
- 依赖 LLM 自行从记忆中推理关系

#### 竞品对比结论

| 维度 | OpenClaw | Hermes | Aranea |
|------|----------|--------|--------|
| 记忆存储 | MEMORY.md 文件 | FTS5 + LLM 摘要 | **L0-L4 五层 + pgvector** |
| 用户可见性 | ✅ 直接编辑文件 | ✅ CLI 查看/编辑 | ❌ 仅 DB 存储 |
| 知识图谱 | ❌ 无 | ❌ 无 | ✅ **L4 实体/关系图** |
| 向量检索 | ❌ 无 | ❌ 无 | ✅ **pgvector 混合检索** |
| 图谱可视化 | ❌ 无 | ❌ 无 | ⚠️ **基础 SVG** |

**关键发现**：OpenClaw 和 Hermes **都没有知识图谱**。Aranea 的 L4 知识图谱是**独有能力**，但可视化体验不足。记忆的用户可见性是竞品标配，Aranea 缺失。

### 4.2 Aranea 知识图谱可视化现状

**后端**（已完整实现）：

| 组件 | 状态 | 说明 |
|------|------|------|
| L4EntityWrite/Read | ✅ | 实体 CRUD |
| L4RelationWrite | ✅ | 关系 CRUD |
| L4DecayWriter | ✅ | 置信度衰减 + 强化信号 |
| Cascade Saga | ✅ | 名称冲突级联更新 |
| Neighborhood BFS | ✅ | 图邻域查询 |
| 自动提取 | ✅ | 人名/偏好正则提取 |

**前端**（基础阶段）：

| 组件 | 状态 | 问题 |
|------|------|------|
| MemoryGraphExplorer | ⚠️ 基础版 | SVG 圆形布局，无力导向/缩放/拖拽 |
| MemoryEvolutionPanel | ⚠️ 基础版 | 仅展示摘要文本，无交互式图谱 |
| ChatEntitySidebar | ⚠️ 轻量级 | 仅列表展示，非图谱 |

**核心问题**：

1. **布局算法差**：简单圆形等分角度排列，节点多了重叠
2. **无交互**：无缩放/平移/拖拽/节点详情弹窗
3. **无编辑**：不能在图谱上直接编辑实体/关系
4. **无筛选**：不能按实体类型/置信度/重要性筛选
5. **无时间维度**：不能查看图谱的演化历史

### 4.3 知识图谱可视化建设方案

#### 技术选型

| 方案 | 优点 | 缺点 | 推荐 |
|------|------|------|------|
| **D3.js force-directed** | 生态最丰富、力导向布局成熟 | 学习曲线陡、需手动管理 DOM | ✅ 推荐 |
| **Cytoscape.js** | 图分析专用、布局算法多 | 包体积大、与 Quasar 集成需适配 | 备选 |
| **Vue Flow**（已有） | 项目已集成、DAG 编辑成熟 | 非知识图谱场景设计，力导向支持弱 | ❌ 不适合 |
| **Sigma.js / Graphology** | 大规模图渲染性能好 | 功能偏少、社区小 | 备选 |
| **当前 SVG 手写** | 零依赖 | 无力导向/交互/缩放 | ❌ 需替换 |

**推荐方案**：**D3.js force-directed** + Vue 3 Composition API 封装

理由：
1. D3 是知识图谱可视化的业界标准
2. 力导向布局（force simulation）天然适合实体-关系图
3. 与 Vue 3 的响应式系统可良好集成
4. 项目前端已有 D3 依赖（通过其他库间接引入）

#### 组件架构

```
components/memory/
  ├── KnowledgeGraphCanvas.vue      ← 主画布组件（D3 force-directed）
  ├── KnowledgeGraphNode.vue        ← 节点渲染（按 EntityType 着色/图标）
  ├── KnowledgeGraphEdge.vue        ← 边渲染（按 RelationType 着色/标签）
  ├── KnowledgeGraphToolbar.vue     ← 工具栏（缩放/筛选/布局切换/导出）
  ├── KnowledgeGraphDetailDrawer.vue ← 节点/边详情抽屉（编辑/删除/强化）
  ├── KnowledgeGraphTimeline.vue    ← 时间轴滑块（图谱演化历史）
  └── KnowledgeGraphMinimap.vue     ← 小地图导航
```

#### 核心功能清单

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | 力导向布局 | P0 | D3 forceSimulation，节点自动排列 |
| 2 | 缩放/平移 | P0 | d3.zoom，鼠标滚轮缩放 + 拖拽平移 |
| 3 | 节点拖拽 | P0 | d3.drag，拖拽节点重新定位 |
| 4 | 节点类型着色 | P0 | user_profile=蓝 / person=绿 / preference=橙 / event=红 / concept=紫 / place=青 |
| 5 | 关系类型标签 | P0 | knows_as / prefers 等关系标签 |
| 6 | 节点详情弹窗 | P0 | 点击节点显示实体详情 + 编辑/删除/强化 |
| 7 | 置信度视觉映射 | P1 | 节点大小/透明度映射 confidence |
| 8 | 重要性视觉映射 | P1 | 节点边框粗细映射 importance |
| 9 | 实体类型筛选 | P1 | 按 EntityType 过滤显示 |
| 10 | 置信度筛选 | P1 | 滑块过滤低置信度实体 |
| 11 | 邻域扩展 | P1 | 双击节点扩展 N-hop 邻域 |
| 12 | 搜索定位 | P1 | 搜索实体名称，高亮定位 |
| 13 | 图谱导出 | P2 | 导出为 PNG/SVG/JSON |
| 14 | 时间轴演化 | P2 | 滑块查看图谱在不同时间点的状态 |
| 15 | 小地图导航 | P2 | 大图谱时提供全局视图 |
| 16 | 批量编辑 | P2 | 多选节点批量修改属性 |
| 17 | 关系创建 | P2 | 拖拽连线创建新关系 |
| 18 | Cascade 可视化 | P2 | 名称冲突的级联影响可视化 |

#### 数据流设计

```
stores/memory/index.ts
  ├── loadNeighborhood(centerID, { hops, minConfidence })  ← 已有
  ├── loadEntities(query)                                   ← 已有
  ├── loadEvolutionForAgent(agentID)                        ← 已有
  ├── upsertEntity(params)                                  ← 新增
  ├── upsertRelation(params)                                ← 新增
  ├── deleteEntity(entityID)                                ← 新增
  ├── deleteRelation(relationID)                            ← 新增
  └── recordReinforcement(entityID, signal, source)         ← 新增
```

#### 后端 API 扩展

当前 `memory.v1` Proto 已有：
- `ListMemoryEntities` — 列出实体
- `GetMemoryNeighborhood` — 获取邻域

需要新增：

| API | 说明 | Proto 变更 |
|-----|------|-----------|
| `UpsertMemoryEntity` | 创建/更新实体 | 新增 RPC |
| `DeleteMemoryEntity` | 删除实体 | 新增 RPC |
| `UpsertMemoryRelation` | 创建/更新关系 | 新增 RPC |
| `DeleteMemoryRelation` | 删除关系 | 新增 RPC |
| `RecordEntityReinforcement` | 记录强化信号 | 新增 RPC |
| `GetMemoryGraphStats` | 图谱统计（节点数/边数/类型分布） | 新增 RPC |
| `GetMemoryEntityTimeline` | 实体时间线（用于演化视图） | 新增 RPC |

### 4.4 记忆用户可见性建设方案

**对标**：OpenClaw MEMORY.md + Hermes FTS5 可搜索

#### 方案：MEMORY.md 导出/编辑 + 记忆仪表盘

**核心思路**：Aranea 的记忆存储在 DB 中（比竞品的文件存储更强大），但需要增加用户可见性。

| 功能 | 实现方式 | 优先级 |
|------|---------|--------|
| **MEMORY.md 导出** | 将 L1 事实 + L2 情景 + L3 知识导出为 Markdown | P0 |
| **MEMORY.md 编辑** | 用户编辑 Markdown 后回写 DB | P1 |
| **记忆仪表盘** | 可视化展示各层记忆的统计和内容 | P0 |
| **GDPR 删除** | 按用户清除所有记忆数据 | P1 |
| **记忆搜索** | 全文搜索所有层级的记忆内容 | P0 |

**MEMORY.md 导出格式**：

```markdown
# Memory: {AgentName}

## Profile (L1)
- Name: {user_name}
- Language: {preferred_language}
- Timezone: {timezone}

## Preferences (L1)
- {preference_1}
- {preference_2}

## Episodes (L2)
### {date} — {summary}
{episode_detail}

## Knowledge (L3)
### {topic}
- {fact_1} (confidence: 0.85)
- {fact_2} (confidence: 0.72)

## Knowledge Graph (L4)
### Entities
- {entity_name} ({entity_type}, confidence: {confidence})

### Relations
- {source} —[{relation_type}]→ {target}
```

**涉及文件**：

```
internal/memory/
  ├── export.go           ← MEMORY.md 导出
  ├── import.go           ← MEMORY.md 编辑回写
  └── purge.go            ← GDPR 删除

web/src/components/memory/
  ├── MemoryExportDialog.vue     ← 导出对话框
  ├── MemoryEditorPanel.vue      ← Markdown 编辑器
  └── MemoryDashboardPanel.vue   ← 记忆仪表盘
```

---

## 五、实施路线图

### Sprint 1（P0 核心工具）

| 任务 | 工作量 | 依赖 |
|------|--------|------|
| Shell 执行工具组（exec_command + write_stdin + kill_session） | 大 | 无 |
| 文档读取工具组（read_document + read_spreadsheet） | 中 | 无 |
| 浏览器工具 MVP（MCP + Playwright） | 中 | 无 |

### Sprint 2（P0 可视化 + P1 工具）

| 任务 | 工作量 | 依赖 |
|------|--------|------|
| 知识图谱可视化（D3 force-directed） | 大 | 无 |
| 记忆 MEMORY.md 导出 | 中 | 无 |
| 消息发送工具 | 中 | outbound.Router |
| 图像生成工具 | 中 | provider 层 |

### Sprint 3（P1 工具 + P2 生态）

| 任务 | 工作量 | 依赖 |
|------|--------|------|
| 语音工具（TTS + STT） | 中 | provider 层 |
| 视觉分析工具 | 中 | provider 层 |
| 内置 Skills 库（15 个首批） | 中 | Eligibility 系统 |
| 浏览器工具完整版（25+ action） | 大 | Sprint 1 MCP 版 |

### Sprint 4（P2 生态 + 高级可视化）

| 任务 | 工作量 | 依赖 |
|------|--------|------|
| 会话搜索工具 | 小 | 无 |
| 记忆 MEMORY.md 编辑 | 中 | Sprint 2 导出 |
| 知识图谱高级功能（时间轴/小地图/Cascade 可视化） | 大 | Sprint 2 基础版 |
| Aranea 专属 Skills（5 个） | 中 | Skills 库 |
| GDPR 删除 | 小 | 无 |

---

## 六、工具注册规范

新增工具必须遵循以下规范：

### 6.1 注册流程

1. 在 `internal/tools/trpc/toolsets.go` 的 `Registry()` 中注册 `ToolRegistration`
2. 在 `internal/tools/trpc/builtin_tools_seed.go` 中添加种子数据
3. 在 `internal/tools/trpc/toolsets.go` 的 `Assemble()` 中处理特殊逻辑
4. 编写单元测试 `internal/tools/trpc/*_test.go`

### 6.2 ToolRegistration 字段规范

```go
ToolRegistration{
    Name:            "tool_name",       // 小写下划线
    Category:        "category",        // filesystem/execution/web/search/communication/productivity/coding/integration/browser/knowledge/memory/vision/speech
    RiskLevel:       "low|medium|high|critical",
    EnabledByDefault: false,            // 仅 file/todo 等低风险工具默认启用
    RequiresConfirmation: false,        // critical 工具必须 true
    Description:     "简短描述",
    Factory:         nil,               // 或 Factory 函数
    ToolSetFactory:  nil,               // 或 ToolSetFactory 函数
}
```

### 6.3 工具构建规范

- 使用 `function.NewFunctionTool[I, O]` 构建工具（红线 A5）
- 禁止手动实现 `CallableTool` 接口
- 错误返回使用 `kerrors`，禁止 `fmt.Errorf`
- goroutine 使用 `pkg/safego.Go`
- 日志使用 `pkg/loggateway.Logger`
