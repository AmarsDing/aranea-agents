# M59: 多模态 Agent — 设计文档

> 版本：v1.0（2026-07-28）
> 需求见 [59-multimodal-agent.md](./59-multimodal-agent.md)；进度与任务见 [59-multimodal-agent.development.md](./59-multimodal-agent.development.md)。
> 本文档记录对 phase5 原规划的**评审结论**与基于代码现状的**修订设计**。

---

## 一、评审结论（phase5 原设计 vs 代码现状）

phase5 原设计（`phase5-差异化创新/02-多模态Agent.md` §二）假设从零建设：`MultimodalAsset` 领域模型、`AssetStore`、`internal/multimodal/processor|generator|storage` 包、4 个新工具。逐项评审：

| 原设计项 | 评审结论 | 证据 |
|----------|----------|------|
| MultimodalAsset + AssetStore + Ent 持久化 | **否决，重复建设**。27-artifact 已是完整资产底座（Artifact 模型、StorageKind、PreviewKind 六类预览、上传/下载/预览 API） | [artifact.go](../../internal/biz/artifact/artifact.go) |
| ImageProcessor（压缩/格式转换） | **保留为治理组件**，但只做压缩/规格治理，不做「理解」——图片理解由 Vision 模型完成 | — |
| FileProcessor（PDF/DOCX 提取） | **否决新建**，改为复用 37-knowledge 的 `Extractor` 接口与 OOXML 实现 | [extractor.go](../../internal/knowledge/extractor.go) |
| AudioProcessor（STT） | **采纳，新建**。当前无任何 STT 能力 | — |
| ImageGenerator（DALL-E） | **否决新建**。provider/media + tools/media 已有异步生成体系（文生图/文生视频/图生视频） | [provider.go](../../internal/provider/media/provider.go)、[generate_image.go](../../internal/tools/media/generate_image.go) |
| SpeechGenerator（TTS） | **否决新建**。63-tts 模块已实现 | [63-tts.design.md](./63-tts.design.md) |
| 扩展 Chat Proto 支持多模态消息 | **已实现**。`AttachmentRef` + artifact 引用 | [chat.proto](../../api/kratos/chat/v1/chat.proto) |
| Chat Service 解析多模态输入 | **已实现**。附件 → `ContentPart{Image}` → LLM | [attachments.go](../../internal/agent/attachments.go#L33-L93) |
| 图片分析工具 | **重新定位**。图片理解走「附件直接进 Vision 模型」，不需要独立分析工具；知识库 VisionExtractor 服务知识摄入场景，不搬进 Chat | [vision_extractor.go](../../internal/knowledge/vision_extractor.go) |

**核心决策（ADR 摘要）**：

| # | 决策 | 理由 |
|---|------|------|
| D1 | 不新建资产模型与存储，27-artifact 为唯一底座 | 避免双资产体系漂移；Artifact 已支撑生成工具产物与上传附件 |
| D2 | 多模态不是新 Agent 类型，是 Chat 链路能力 | 与产品现状一致（任意 LLM Agent + 多模态模型即可） |
| D3 | 理解走「原生 ContentPart 直通」优先、「提取为文本注入」兜底 | Vision/Audio 模型能力内禀理解优于外部 OCR/STT；无能力时降级 |
| D4 | 文档理解复用 knowledge Extractor，STT 新建为独立组件挂入同一 Extractor 风格接口 | 接口已验证（`Supports(ext, mime)` + `Extract → Markdown`），STT 与文档提取同构 |
| D5 | 增量只补四块：图片治理、文档提取接入 Chat、STT、回放持久化 | 其余均为验收项，不写新代码 |

---

## 二、架构设计

### 2.1 分层与模块位置

```
web/src/components/chat/                 ← 前端：附件渲染/上传（已有，增量补齐）
        ↓ AttachmentRef[]
api/kratos/chat/v1/chat.proto            ← 已有：AttachmentRef
        ↓
internal/service/chat_attachments.go     ← 已有：refs 解析 + 能力门控
        ↓
internal/agent/attachments.go            ← 已有：BuildUserMessageFromArtifacts
        │   ├── image/* → ContentPart{Image}（已实现）
        │   ├── 文本 → blob/preview 注入（已实现）
        │   └── 【新增】文档/音频 → Extractor 提取 → 文本注入
        ↓
internal/biz/artifact/                   ← 已有：资产底座（不改动，仅消费）
internal/multimodal/                     ← 【新增，小包】仅放治理与提取胶水
  ├── imageguard/guard.go                ← 图片压缩/规格治理（D1 治理组件）
  ├── stt/transcriber.go                 ← STT 接口 + 实现（OpenAI Whisper / 兼容端点）
  └── docextract/bridge.go               ← knowledge.ExtractorRegistry 的 Chat 侧适配
internal/knowledge/                      ← 已有：Extractor/VisionExtractor（复用，不改动接口）
internal/provider/media/ + internal/tools/media/ ← 已有：媒体生成（不改动）
```

**依赖方向铁律**：`internal/multimodal` 属于运行时胶水层，只被 `internal/agent`、`internal/service` 调用；biz 层不感知其存在（附件理解在 agent/service 装配层完成，符合「框架反模式：不在 biz 引框架运行时」的边界）。

### 2.2 输入链路（现状 + 增量）

**现状链路（已实现）**：

```
上传 → artifact 保存（session 绑定）→ 前端发送 SendTurn(content, attachmentRefs)
  → ChatService.buildUserMessageFromProto
  → validateTurnAttachmentCapabilities（模型能力闸：image/pdf 等）
  → chatagent.BuildUserMessageFromArtifacts
      ├── image/* → ContentPart{Image{Data, Format, Detail:auto}}
      ├── 文本 ≤ 阈值 → 文本 part 注入
      └── 文本超阈值 → ToolResultGate 落地 blob + preview 注入
  → LLM（Vision 模型理解图片）
```

**增量 1：图片治理（imageguard）**

在 `BuildUserMessageFromArtifacts` 的 image 分支前插入：

```go
// internal/multimodal/imageguard/guard.go
type Guard struct{ MaxEdgePx, MaxBytes, JPEGQuality int }
func (g Guard) Normalize(data []byte, format string) (out []byte, outFormat string, err error)
// 规则：长边 > 2048 → 等比缩放；> 4MB 且非 jpeg → 转 jpeg q85；gif 首帧化
```

**增量 2：文档理解接入（docextract）**

非图片、非纯文本附件（pdf/docx/xlsx/pptx）当前只走 blob 治理。新增分支：

```go
// internal/multimodal/docextract/bridge.go
type Bridge struct{ Reg *knowledge.ExtractorRegistry }
func (b Bridge) ExtractToText(ctx context.Context, name, mime string, data []byte) (string, error)
// 命中 Extractor → 返回结构化 Markdown（作为文本 part 注入，标注 [附件 name 内容提取])
// 未命中/失败 → 返回空，调用方回退现有 blob 治理路径（降级语义不变）
```

**增量 3：音频理解（stt）**

```go
// internal/multimodal/stt/transcriber.go
type Transcriber interface {
    Supports(mimeType string) bool
    Transcribe(ctx context.Context, data []byte, mimeType string) (string, error)
}
// 实现 OpenAITranscriber（whisper-1 / 兼容 audio/transcriptions 端点，复用 LLM Provider 的 base_url/key 配置）
// 配置缺失时返回 ErrUnavailable → 降级提示，不阻断
```

音频在 `BuildUserMessageFromArtifacts` 新增 `audio/*` 分支：Transcriber 转写 → 文本 part 注入（`[语音转写] ...`）。Gemini 原生 audio ContentPart 直通列为后续增强（P2），不在本期。

### 2.3 输出链路（现状，验收项）

```
LLM tool_call(generate_image/generate_video/tts)
  → tools/media 或 63-tts → provider/media 异步生成
  → 产物写 artifact（MediaArtifact{ArtifactID, URL, MimeType, Width/Height/DurationMs, Thumbnail}）
  → 事件携带 artifact 引用 → 前端消息流渲染媒体卡片
```

本期不改代码，仅做端到端验收与 i18n/渲染核查。

### 2.4 回放链路（核心缺口）

**问题**：重开会话后，用户消息与工具产物的附件引用需完整恢复。

**现状线索**：`mergeUserAttachmentRefs` 已把附件 refs 合并进 turn options JSON（`MergeRefsIntoOptionsJSON`），疑似已随 turn 持久化；前端是否据此回放渲染**待验收确认**。

**设计（若验收确认缺失则实施）**：
1. 数据源：以 turn options JSON 中的附件 refs 为准（不新建表、不加列——遵循 D1 复用原则）
2. 后端：会话历史加载路径（turns → 前端消息重建）透传附件 refs
3. 前端：消息气泡按 refs 拉取 artifact 预览（复用现有 Lightbox/附件卡片组件）
4. 仅在 turn options 确实不持久化时，才走 DDL 迁移给持久化记录加 `attachments_json` 列（备选，幂等迁移）

### 2.5 前端组件设计

| 组件 | 现状 | 增量 |
|------|------|------|
| 附件选择器（输入框） | 已有 | 类型白名单 + 数量/体积前置校验提示 |
| ChatMessageAttachments | 已有 | 按 PreviewKind 分派：image→缩略图+Lightbox、audio→播放条、video→poster+播放、pdf/file→下载卡 |
| Lightbox 预览 | 已有（observe-media-preview 注入模式） | 复用，核查 chat 场景注入一致 |
| i18n | — | 新增 `multimodal.*` 键（zh-CN/en-US）：类型名、错误提示、降级提示 |

---

## 三、接口与数据模型

### 3.1 复用接口（不改动）

| 接口 | 位置 | 用途 |
|------|------|------|
| `artifact.Usecase`（Load/ResolveAttachmentRefs/Preview） | [internal/biz/artifact](../../internal/biz/artifact/) | 资产读取与引用解析 |
| `knowledge.Extractor` / `ExtractorRegistry` | [extractor.go](../../internal/knowledge/extractor.go#L13-L28) | 文档结构化提取 |
| `media.MediaProvider` | [provider.go](../../internal/provider/media/provider.go) | 媒体生成 |
| `provider.ValidateAttachmentCapabilities` | internal/provider | 模型能力门控 |

### 3.2 新增接口（小包、窄接口）

| 接口 | 方法数 | 说明 |
|------|--------|------|
| `imageguard.Guard` | 1（Normalize） | 图片规格治理 |
| `stt.Transcriber` | 2（Supports/Transcribe） | STT 抽象，Stability:evolving |
| `docextract.Bridge` | 1（ExtractToText） | knowledge → chat 适配 |

### 3.3 数据模型

**不新增 Ent Schema、不新增表**。全部资产数据在 27-artifact 既有表。回放若需补丁，仅一条幂等 DDL 迁移（备选，见 §2.4）。

### 3.4 配置

```yaml
multimodal:
  image_max_edge_px: 2048
  image_max_bytes: 4194304
  stt:
    provider: openai-compatible   # 复用 LLM provider base_url/key
    model: whisper-1
    timeout_seconds: 30
```

配置走既有 Kratos config 体系；STT 未配置时 Transcriber 返回 ErrUnavailable，链路降级。

---

## 四、错误处理

| 场景 | 错误 | 行为 |
|------|------|------|
| 附件类型不在白名单 | `BadRequest("CHAT_INPUT", ...)`（复用现有域） | 闸前拒绝 |
| 图片超限 | `BadRequest("CHAT_INPUT", ...)` | 闸前拒绝 |
| 模型不支持图片/PDF | `TurnErrAttachmentUnsupported`（已有） | 闸前拒绝 + 前端提示 |
| 文档提取失败 | 降级：回退 blob 治理路径 | 不阻断 |
| STT 未配置/失败 | 降级：注入 `[语音无法转写]` 提示 part | 不阻断 |
| 媒体生成失败 | 工具返回结构化 error | Agent 转述 |
| 回放 artifact 已删除 | 前端渲染占位卡（「附件已删除」） | 不报错 |

---

## 五、与 phase5 原任务清单的差异说明

原 T1–T16 中：T1/T2/T12（资产模型/存储/Repo）**取消**（D1）；T3 保留为治理；T4 改为复用；T5 保留（STT）；T6/T7 **取消**（已有）；T8 取消 3/4（只无新增工具）；T9/T10 **已完成**；T11 取消；T14/T15 保留为前端增量；T16 保留。修订后的任务清单见开发计划 §4。
