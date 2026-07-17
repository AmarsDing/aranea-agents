# 媒体生成 + 成员节点观测视图设计

> **设计稿** · 2026-07-18 · 状态：可实施
>
> **范围**：媒体生成能力（文生图/文生视频/图生视频）+ 聊天 UI 内 ComfyUI 风格的成员节点实时观测视图
> **方式**：新建为主（独立 MediaProvider 系统 + 独立观测画布），少量扩展现有组件
> **前置调研**：ComfyUI 前端架构（[Comfy-Org/ComfyUI_frontend](https://github.com/Comfy-Org/ComfyUI_frontend)）、项目现有 Graph/Provider/Activity 系统
>
> **核心定位**：不复用现有规划图编辑器（GraphEditorCanvas），新建专用观测画布；不混入 LLM Provider 系统，新建独立 MediaProvider。

---

## 一、背景与问题陈述

### 1.1 用户需求

1. 扩展文生图、文生视频、图生视频三种媒体生成能力
2. 创建专门制作视频的团队，下达任务后每位成员成果实时展示
3. 类似 ComfyUI，能实时查看每个成员节点的工作状态和输出结果
4. 聊天 UI 保持现有逻辑实时输出聊天内容
5. 在状态栏（SpiritStatusBar）右侧添加面板切换按钮，点击全屏切换到观测视图
6. Composer（聊天输入框）可开启和关闭

### 1.2 ComfyUI 调研结论

ComfyUI 前端基于 LiteGraph.js（Canvas2D 渲染），正在向 Nodes 2.0（Vue 组件渲染）迁移。关键特性：
- 节点内实时预览（`executed` 事件携带 output，直接在节点内渲染图片/视频缩略图）
- 二进制预览帧（WebSocket 推送采样中间结果）
- 节点边框变色（executing 黄、executed 绿、error 红）
- 3 个专用 Store：`ExecutionStore`（执行状态）、`NodeOutputStore`（节点产出管理）、`ExecutionErrorStore`（错误可视化）

### 1.3 项目现状差距

| 能力 | ComfyUI | 项目现状 | 差距等级 |
|------|---------|---------|---------|
| 节点图引擎 | LiteGraph (Canvas2D) | Vue Flow (SVG/DOM) | ✅ 项目更现代 |
| 节点内媒体预览 | ✅ | ❌ 仅文本 outputPreview | 🔴 核心 |
| 媒体生成工具 | ✅ 内置节点 | ❌ 无 | 🔴 核心 |
| NodeOutputStore 等价物 | ✅ | ❌ Artifact 未对接节点 | 🔴 核心 |
| 嵌入聊天 UI | ❌ 独立应用 | ❌ 独立路由 | 🔴 共同差距 |
| 节点级细粒度进度 | ✅ progress value/max | ❌ 仅节点级状态 | 🟡 可选 |
| 实时执行状态 | ✅ ExecutionStore | ✅ exec-node-states | ✅ 无差距 |

### 1.4 关键架构决策（基于代码实读）

**决策 1：MediaProvider 独立于 LLM Provider 系统**

现有 Provider 系统（[catalog.go](../../../internal/provider/catalog.go)）为 LLM 文本生成设计：
- 接口模式：同步/流式文本生成（Generate/Stream）
- Capabilities：Text/Vision/Audio/File/ToolCall/Cache/Thinking/TextOnly（无媒体生成能力声明）
- 注册方式：`trpcprovider.Register(name, constructor)`（[register_extra.go:23](../../../internal/provider/register_extra.go#L23)）

媒体生成是异步长任务（提交→轮询→取结果），协议完全不同。强行混入会污染 LLM 抽象。**新建独立 MediaProvider 系统**，参考现有注册模式但不混用。

Agent 的 `provider`/`model` 字段（[agent.go:37-38](../../../internal/data/ent/schema/agent.go#L37-L38)）继续承载 LLM（思考决策），媒体生成由 Agent 调用工具，工具内部调用 MediaProvider。

**决策 2：观测视图不复用 GraphEditorCanvas**

项目有两套独立的 Graph 数据：

| 维度 | 规划图（GraphDef） | 观测图（GraphStage） |
|------|-------------------|---------------------|
| 数据来源 | [builder.go](../../../internal/graph/trpc/builder.go) Team 编译而来 | activityV2Store 实时事件流 |
| 职责 | 指明团队成员如何工作（编排） | 观测每个成员实时工作状态/输出 |
| 渲染组件 | [GraphEditorCanvas](../../../web/src/components/graph/GraphEditorCanvas.vue)（Vue Flow） | [GraphStageBlock](../../../web/src/components/chat/v2/GraphStageBlock.vue)（SVG） |
| 所在页面 | GraphEditorPage / GraphRunPage | 聊天流内嵌 |

chat 中的 ComfyUI 观测视图职责是"实时观测每个成员节点的工作状态和输出结果"，对应**观测图**。**新建专用观测画布 ObservationCanvas**，数据源是 activityV2Store，不复用 GraphEditorCanvas。

**决策 3：Composer 可开关（双独立状态）**

- `viewMode: 'chat' | 'observe'`（视图模式）
- `composerVisible: boolean`（Composer 显隐，默认 true）

两个状态独立，两种模式都可控制 Composer。

---

## 二、设计目标

### 2.1 功能性目标

1. **媒体生成**：Agent 能调用工具生成图片/视频，结果存为 Artifact 并在 Activity 中引用
2. **节点内媒体预览**：观测画布的每个成员节点实时显示产出的图片/视频缩略图
3. **实时观测**：节点状态实时更新（pending/running/completed/failed），running 时显示进度条
4. **全屏切换**：状态栏右侧按钮全屏切换 chat ↔ observe 视图
5. **Composer 可控**：两种模式下 Composer 都可独立开关
6. **视频团队**：通过现有 Agent + Team 系统配置视频制作团队模板

### 2.2 非功能性目标

1. **实时性**：节点状态/媒体产出通过现有 WebSocket 事件流推送，延迟 < 500ms
2. **可观测性**：每个成员节点的状态/进度/产出/最新活动一目了然
3. **零迁移**：Phase 1-2 用 Activity.Meta 约定字段，不改 Ent Schema
4. **不破坏现有**：聊天流逻辑保持不变，观测视图是增量

---

## 三、整体架构

```
┌─────────────────────────────────────────────────────────┐
│                    ChatPage（聊天页）                     │
│  ┌──────────┐  ┌────────────────────┐  ┌──────────────┐ │
│  │ Entity   │  │  ChatMessagePanel  │  │ Session      │ │
│  │ Sidebar  │  │  ┌──────────────┐  │  │ Sidebar      │ │
│  │          │  │  │ viewMode=    │  │  │              │ │
│  │          │  │  │  chat/observe│  │  │              │ │
│  │          │  │  ├──────────────┤  │  │              │ │
│  │          │  │  │ ChatMessageL │  │  │              │ │
│  │          │  │  │ ist（chat）  │  │  │              │ │
│  │          │  │  │   或         │  │  │              │ │
│  │          │  │  │ Observation  │  │  │              │ │
│  │          │  │  │ Panel(observe)│  │  │              │ │
│  │          │  │  ├──────────────┤  │  │              │ │
│  │          │  │  │ ChatComposer │  │  │              │ │
│  │          │  │  │ (可开关)     │  │  │              │ │
│  │          │  │  ├──────────────┤  │  │              │ │
│  │          │  │  │SpiritStatus │  │  │              │ │
│  │          │  │  │Bar+切换按钮 │  │  │              │ │
│  │          │  │  └──────────────┘  │  │              │ │
│  └──────────┘  └────────────────────┘  └──────────────┘ │
└─────────────────────────────────────────────────────────┘

数据流：
  后端 Activity 事件 → WebSocket → activityV2Store
    ├→ 聊天流：ActionBlock → MediaToolDetail（媒体卡片）
    └→ 观测画布：useObserveGraph → ObservationCanvas → ObserveNode
                                                       ├→ 状态/进度
                                                       └→ nodeOutputStore（媒体预览）
```

---

## 四、Phase 1：MediaProvider 系统 + 媒体生成工具（后端）

### 4.1 MediaProvider 接口

```go
// internal/provider/media/provider.go
package media

// MediaProvider 媒体生成 Provider 接口。
// 独立于 LLM Provider（internal/provider/），因为媒体生成是异步长任务
// （提交→轮询→取结果），协议与 LLM 同步/流式文本生成完全不同。
type MediaProvider interface {
    Name() string
    GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
    GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error)
    ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error)
}

type ImageRequest struct {
    Prompt  string
    Size    string  // "1024x1024" / "1792x1024" / "1024x1792"
    Style   string  // "realistic" / "anime" / "oil_painting" / ""
    Count   int     // 1-4
    Seed    int64   // 0 = random
}

type ImageResult struct {
    Artifacts []MediaArtifact
}

type VideoRequest struct {
    Prompt     string
    DurationMs int64
    FPS        int
    Resolution string  // "720p" / "1080p"
    Seed       int64
}

type ImageToVideoRequest struct {
    ImageArtifactID string  // 输入图片的 Artifact ID
    Prompt          string
    DurationMs      int64
    FPS             int
}

type VideoResult struct {
    Artifacts []MediaArtifact
}

type MediaArtifact struct {
    ArtifactID string  // Artifact 系统 ID
    URL        string  // 签名访问 URL
    MimeType   string  // "image/png" / "video/mp4"
    Width      int
    Height     int
    DurationMs int64   // 视频
    Thumbnail  string  // 缩略图 URL（视频 poster）
}
```

### 4.2 Provider 注册（参考 register_extra.go 模式）

```go
// internal/provider/media/registry.go
package media

var providers = map[string]MediaProviderConstructor{}

type MediaProviderConstructor func(cfg ProviderConfig) (MediaProvider, error)

func Register(name string, c MediaProviderConstructor) {
    providers[name] = c
}

func Get(name string, cfg ProviderConfig) (MediaProvider, error) {
    c, ok := providers[name]
    if !ok {
        return nil, fmt.Errorf("media provider %q not registered", name)
    }
    return c(cfg)
}

type ProviderConfig struct {
    Name      string
    ProviderType string  // "comfyui_local" / "qwen" / "kling"
    BaseURL   string
    APIKey    string
    Extra     map[string]any
}
```

### 4.3 Provider 实现

**ComfyUILocalProvider**（`internal/provider/media/comfyui_local.go`）：
- 调用 ComfyUI HTTP API：`POST /prompt` 提交工作流 + `GET /history/{prompt_id}` 取结果
- WebSocket `ws://<host>/ws` 监听执行进度（`executing`/`progress`/`executed`）
- 工作流模板：内置文生图/文生视频/图生视频三个 ComfyUI workflow JSON

**QwenProvider**（`internal/provider/media/qwen.go`）：
- 通义万相 API（图像）：`POST /services/aigc/text2image/image-synthesis`
- 通义万相视频 API：`POST /services/aigc/video-generation/video-synthesis`
- 异步任务模式：提交→轮询 `GET /tasks/{task_id}`→取结果

**KlingProvider**（`internal/provider/media/kling.go`）：
- 可灵视频 API（文生视频/图生视频）
- 异步任务模式

### 4.4 配置存储

新建 `media_providers` 表（参考 agent_catalog 模式，复用 DDL Migration Registry）：

```sql
-- sql/migrations/20260718_media_providers.sql
CREATE TABLE IF NOT EXISTS media_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL,  -- comfyui_local / qwen / kling
    base_url TEXT DEFAULT '',
    api_key TEXT DEFAULT '',       -- 加密存储
    config_json TEXT DEFAULT '{}', -- 额外配置
    capabilities TEXT DEFAULT '[]', -- ["image","video","image_to_video"]
    status TEXT DEFAULT 'active',
    created_at TEXT DEFAULT '',
    updated_at TEXT DEFAULT ''
);
```

**Ent Schema**：`internal/data/ent/schema/media_provider.go`，标记 `.Sensitive()` 保护 api_key（遵守 DB-N8）。

### 4.5 媒体生成工具

```go
// internal/tools/media/generate_image.go
func NewGenerateImageTool(mp media.MediaProvider, artifactRepo artifact.Repo) tool.Tool {
    return trpcfunction.NewFunctionTool(
        "generate_image",
        "根据文本描述生成图片",
        func(ctx context.Context, in GenerateImageInput) (GenerateImageOutput, error) {
            // 1. 调用 MediaProvider 生成
            result, err := mp.GenerateImage(ctx, media.ImageRequest{
                Prompt: in.Prompt, Size: in.Size, Style: in.Style, Count: in.Count,
            })
            if err != nil {
                return GenerateImageOutput{}, err
            }
            // 2. 存为 Artifact
            var artifacts []MediaArtifact
            for _, a := range result.Artifacts {
                art, err := artifactRepo.Create(ctx, artifact.CreateRequest{
                    MimeType: a.MimeType, Data: a.Data,
                })
                if err != nil {
                    return GenerateImageOutput{}, err
                }
                artifacts = append(artifacts, MediaArtifact{
                    ArtifactID: art.ID, URL: art.SignedURL,
                    MimeType: a.MimeType, Width: a.Width, Height: a.Height,
                })
            }
            // 3. 返回工具结果（前端通过 ToolResult 获取 media_artifacts）
            return GenerateImageOutput{Artifacts: artifacts}, nil
        },
    )
}
```

三个工具：`generate_image` / `generate_video` / `image_to_video`，结构类似。

### 4.6 Activity.Meta 约定（零迁移）

工具执行后，将 MediaArtifact 列表写入 `Activity.Meta["media_artifacts"]`，前端通过 Meta 读取：

```json
{
  "media_artifacts": [
    {
      "artifact_id": "art_xxx",
      "url": "https://...",
      "mime_type": "image/png",
      "width": 1024,
      "height": 1024
    }
  ],
  "progress": { "value": 100, "max": 100, "label": "生成完成" }
}
```

**工具进度推送**：长任务（视频生成）通过 ActivityEvent updated 事件推送 `meta.progress`：

```go
// 工具执行中定期推送进度
ctx = WithActivityMeta(ctx, map[string]any{
    "progress": map[string]any{"value": 30, "max": 100, "label": "采样中 30%"},
})
```

### 4.7 ToolCategory 扩展

```go
// internal/biz/activity.go
const (
    ToolCategoryShell     ToolCategory = "shell"
    // ... 现有类别
    ToolCategoryMedia     ToolCategory = "