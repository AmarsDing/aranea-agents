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
3. **Activity 数据零迁移**：媒体产出引用通过 `Activity.Meta["media_artifacts"]` 约定字段传递，不改动 `activities` 表 Schema。Phase 1 新增 `media_providers` 配置表属于新表（非改动现有表），通过 DDL Migration Registry 管理
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
    ToolCategoryMedia     ToolCategory = "media"  // 新增
)
```

### 4.8 Wire 注入

```go
// cmd/admin/wire.go 新增 binding
func NewMediaProviderSet(...) wire.ProviderSet {
    return wire.NewSet(
        media.NewComfyUILocalProvider,
        media.NewQwenProvider,
        media.NewKlingProvider,
        media.NewRegistry,
        mediatools.NewGenerateImageTool,
        mediatools.NewGenerateVideoTool,
        mediatools.NewImageToVideoTool,
    )
}
```

### 4.9 Phase 1 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/provider/media/provider.go` | 新建：接口 + 类型定义 |
| `internal/provider/media/registry.go` | 新建：注册中心 |
| `internal/provider/media/comfyui_local.go` | 新建：ComfyUI 本地实现 |
| `internal/provider/media/qwen.go` | 新建：通义万相实现 |
| `internal/provider/media/kling.go` | 新建：可灵实现 |
| `internal/tools/media/generate_image.go` | 新建：文生图工具 |
| `internal/tools/media/generate_video.go` | 新建：文生视频工具 |
| `internal/tools/media/image_to_video.go` | 新建：图生视频工具 |
| `internal/tools/media/toolset.go` | 新建：工具注册 |
| `internal/data/ent/schema/media_provider.go` | 新建：Schema |
| `sql/migrations/20260718_media_providers.sql` | 新建：DDL 迁移 |
| `internal/data/ddl_migration_registry.go` | 注册新迁移 |
| `internal/biz/activity.go` | ToolCategory 新增 media |
| `internal/tools/toolset_assemble.go` | 注册媒体工具 |
| `cmd/admin/wire.go` | 注入 MediaProvider |

---

## 五、Phase 2：成员节点观测画布（前端）

### 5.1 核心组件层级

```
ObservationPanel（容器）
├── ObservationCanvas（Vue Flow 画布）
│   ├── ObserveNode（成员节点，自定义 Vue Flow Node）
│   │   ├── 头部：Avatar + 名称 + StatusBadge
│   │   ├── 进度条（running 时显示）
│   │   ├── NodeMediaPreview（媒体缩略图网格）
│   │   └── 最新活动摘要
│   └── 边：依赖关系（来自 GraphNode.DependsOn）
├── ObserveNodeDetail（选中节点的详情侧栏）
│   ├── MemberSessionPanel（复用现有组件）
│   └── 媒体产出列表
└── 工具栏：刷新 + Composer toggle
```

### 5.2 ObservationCanvas（观测画布）

```vue
<!-- web/src/components/chat/observe/ObservationCanvas.vue -->
<template>
  <VueFlow :nodes="nodes" :edges="edges" :fit-view-on-init="true" :node-types="nodeTypes">
    <Background />
    <Controls />
    <MiniMap />
  </VueFlow>
</template>

<script setup lang="ts">
import { VueFlow, useVueFlow } from '@vue-flow/core';
import { computed, markRaw } from 'vue';
import ObserveNode from './ObserveNode.vue';
import type { GraphStage, GraphNode } from '../../../features/chat/v2Types';
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';

const props = defineProps<{
  graphStage: GraphStage;
  nodes: GraphNode[];
}>();

const nodeOutputStore = useNodeOutputStore();
const nodeTypes = { observe: markRaw(ObserveNode) };

// GraphNode → VueFlow Node 转换
const nodes = computed(() =>
  props.nodes.map(n => ({
    id: n.ID,
    type: 'observe',
    position: positionFor(n),
    data: {
      label: n.Label,
      agentKey: n.AgentKey,
      status: n.Status,
      dependsOn: n.DependsOn,
      mediaOutput: nodeOutputStore.getNodeOutput(n.ID),
    },
  }))
);

const edges = computed(() =>
  props.nodes.flatMap(n =>
    (n.DependsOn || []).map(depId => ({
      id: `${depId}->${n.ID}`,
      source: depId,
      target: n.ID,
      animated: true,
    }))
  )
);
</script>
```

**布局算法**：复用现有 [usePlanDAGLayout.ts](../../../web/src/features/chat/composables/usePlanDAGLayout.ts) 的最长路径分层算法，适配为 VueFlow position 格式。

### 5.3 ObserveNode（ComfyUI 风格节点）

```vue
<!-- web/src/components/chat/observe/ObserveNode.vue -->
<template>
  <div :class="['observe-node', `observe-node--${data.status}`]">
    <header class="observe-node__header">
      <EntityAvatar :agent-key="data.agentKey" size="24px" />
      <span class="observe-node__name">{{ data.label }}</span>
      <ObserveStatusBadge :status="data.status" />
    </header>

    <!-- 进度条（running 时显示，对标 ComfyUI progress） -->
    <div v-if="data.status === 'running' && progress" class="observe-node__progress">
      <q-linear-progress :value="progress.value / progress.max" color="primary" size="4px" />
      <span class="observe-node__progress-label">{{ progress.label }}</span>
    </div>

    <!-- 媒体预览区（对标 ComfyUI 节点内图片预览） -->
    <NodeMediaPreview
      v-if="data.mediaOutput?.length"
      :artifacts="data.mediaOutput"
      @preview="$emit('preview', $event)"
    />

    <!-- 最新活动摘要 -->
    <div class="observe-node__activity">
      <q-icon :name="latestActivityIcon" size="12px" />
      <span>{{ latestActivitySummary }}</span>
    </div>
  </div>
</template>
```

**节点状态 CSS**（对标 ComfyUI 边框变色）：

```scss
.observe-node {
  border: 2px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-surface);
  min-width: 180px;
  &--running {
    border-color: var(--color-warning);
    animation: pulse 1.5s infinite;
  }
  &--completed { border-color: var(--color-positive); }
  &--failed { border-color: var(--color-negative); }
}
@keyframes pulse {
  0%, 100% { box-shadow: 0 0 0 0 rgba(var(--color-warning-rgb), 0.4); }
  50% { box-shadow: 0 0 0 6px rgba(var(--color-warning-rgb), 0); }
}
```

### 5.4 NodeMediaPreview（节点内媒体预览）

```vue
<!-- web/src/components/chat/observe/NodeMediaPreview.vue -->
<template>
  <div class="node-media-preview">
    <div
      v-for="art in displayArtifacts"
      :key="art.artifact_id"
      class="node-media-preview__item"
      @click="$emit('preview', art)"
    >
      <video
        v-if="art.mime_type.startsWith('video/')"
        :src="art.url"
        :poster="art.thumbnail"
        muted
        preload="metadata"
        class="node-media-preview__media"
      />
      <img
        v-else
        :src="art.url"
        loading="lazy"
        class="node-media-preview__media"
      />
    </div>
    <span v-if="artifacts.length > 3" class="node-media-preview__more">
      +{{ artifacts.length - 3 }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

const props = defineProps<{ artifacts: MediaArtifact[] }>();
defineEmits<{ preview: [art: MediaArtifact] }>();

const displayArtifacts = computed(() => props.artifacts.slice(0, 3));
</script>
```

### 5.5 useNodeOutputStore（对标 ComfyUI NodeOutputStore）

```typescript
// web/src/stores/chat/nodeOutputStore.ts
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { MediaArtifact } from '../../features/chat/mediaTypes';

export const useNodeOutputStore = defineStore('nodeOutput', () => {
  // nodeId -> MediaArtifact[]
  const outputsByNode = reactive<Map<string, MediaArtifact[]>>(new Map());

  function setNodeOutput(nodeId: string, artifacts: MediaArtifact[]): void {
    outputsByNode.set(nodeId, artifacts);
  }

  function appendNodeOutput(nodeId: string, artifact: MediaArtifact): void {
    const existing = outputsByNode.get(nodeId) || [];
    existing.push(artifact);
    outputsByNode.set(nodeId, existing);
  }

  function getNodeOutput(nodeId: string): MediaArtifact[] {
    return outputsByNode.get(nodeId) || [];
  }

  function clearSession(): void {
    outputsByNode.clear();
  }

  return { outputsByNode, setNodeOutput, appendNodeOutput, getNodeOutput, clearSession };
});
```

### 5.6 activityV2Store → nodeOutputStore 同步

在 activityV2Store 处理 `action` 活动时，若 `meta.media_artifacts` 非空，同步写入 nodeOutputStore：

```typescript
// web/src/stores/chat/activityV2Store.ts（已有文件，新增逻辑）
function handleActionActivity(activity: Activity) {
  // ... 现有逻辑
  const mediaArtifacts = activity.Meta?.media_artifacts as MediaArtifact[] | undefined;
  if (mediaArtifacts?.length) {
    // nodeId = author agent key（成员节点 ID）
    const nodeId = resolveNodeIdFromAuthor(activity.Author);
    nodeOutputStore.setNodeOutput(nodeId, mediaArtifacts);
  }
}
```

### 5.7 媒体类型定义

```typescript
// web/src/features/chat/mediaTypes.ts
export interface MediaArtifact {
  artifact_id: string;
  url: string;
  mime_type: string;  // "image/png" / "video/mp4"
  width?: number;
  height?: number;
  duration_ms?: number;
  thumbnail?: string;  // 视频 poster
}

export interface MediaProgress {
  value: number;
  max: number;
  label?: string;
}
```

### 5.8 useObserveGraph（数据转换 composable）

```typescript
// web/src/features/chat/composables/useObserveGraph.ts
import { computed } from 'vue';
import { useActivityQueries } from './useActivityQueries';
import { usePlanDAGLayout } from './usePlanDAGLayout';
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';

export function useObserveGraph(spiritSessionId: Ref<string>) {
  const { getGraphStage, getGraphStageNodes } = useActivityQueries();
  const nodeOutputStore = useNodeOutputStore();

  const graphStage = computed(() => getGraphStage(spiritSessionId.value));
  const nodes = computed(() => getGraphStageNodes(graphStage.value?.ID || ''));

  // 复用现有 DAG 布局算法
  const { positions, computedWidth } = usePlanDAGLayout(nodes);

  return { graphStage, nodes, positions, computedWidth };
}
```

### 5.9 Phase 2 改动文件清单

| 文件 | 改动 |
|------|------|
| `web/src/components/chat/observe/ObservationPanel.vue` | 新建：观测视图容器 |
| `web/src/components/chat/observe/ObservationCanvas.vue` | 新建：Vue Flow 画布 |
| `web/src/components/chat/observe/ObserveNode.vue` | 新建：ComfyUI 风格节点 |
| `web/src/components/chat/observe/ObserveStatusBadge.vue` | 新建：节点状态徽章 |
| `web/src/components/chat/observe/NodeMediaPreview.vue` | 新建：节点内媒体预览 |
| `web/src/components/chat/observe/MediaLightbox.vue` | 新建：全屏预览 |
| `web/src/components/chat/tools/MediaToolDetail.vue` | 新建：聊天流媒体渲染 |
| `web/src/components/chat/tools/classifyTool.ts` | 新增 media 分类 |
| `web/src/components/chat/ActionBlock.vue` | switch 新增 media 分支 |
| `web/src/stores/chat/nodeOutputStore.ts` | 新建：节点产出 Store |
| `web/src/stores/chat/activityV2Store.ts` | 新增 media_artifacts 同步逻辑 |
| `web/src/features/chat/mediaTypes.ts` | 新建：媒体类型定义 |
| `web/src/features/chat/composables/useObserveGraph.ts` | 新建：数据转换 |

---

## 六、Phase 3：状态栏切换 + 视图集成（前端）

### 6.1 状态栏切换按钮

在 [SpiritStatusBar.vue](../../../web/src/components/spirit/SpiritStatusBar.vue) 的 `__inner` div 末尾新增：

```vue
<template>
  <div class="row items-center no-wrap q-gutter-sm spirit-status-bar__inner">
    <span class="spirit-status-bar__dot" />
    <!-- 现有 items：complexity / running teams / progress / context usage -->
    <div v-if="hasContextInfo" class="spirit-status-bar__item ...">
      <q-icon name="data_usage" ... />
      <span>{{ contextPercent }}%</span>
    </div>

    <!-- 新增：视图切换按钮（你选中的 div 的右侧） -->
    <div
      class="spirit-status-bar__item spirit-status-bar__item--clickable spirit-status-bar__view-toggle"
      @click="emit('toggle-view')"
    >
      <q-icon
        :name="viewMode === 'observe' ? 'chat' : 'visibility'"
        size="14px"
        :color="viewMode === 'observe' ? 'primary' : 'text-tertiary'"
      />
      <span>{{ viewMode === 'observe' ? t('spirit.backToChat') : t('spirit.observeView') }}</span>
    </div>

    <!-- 新增：Composer 显隐按钮（仅 observe 模式显示） -->
    <div
      v-if="viewMode === 'observe'"
      class="spirit-status-bar__item spirit-status-bar__item--clickable"
      @click="emit('toggle-composer')"
    >
      <q-icon
        :name="composerVisible ? 'keyboard' : 'keyboard_hide'"
        size="14px"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  viewMode: 'chat' | 'observe';
  composerVisible: boolean;
  // ... 现有 props
}>();
defineEmits<{
  'toggle-view': [];
  'toggle-composer': [];
  // ... 现有 emits
}>();
</script>
```

### 6.2 spiritStore 双状态管理

```typescript
// web/src/stores/spirit/index.ts（已有文件，新增）
const viewMode = ref<'chat' | 'observe'>('chat');
const composerVisible = ref(true);

function toggleViewMode(): void {
  viewMode.value = viewMode.value === 'chat' ? 'observe' : 'chat';
}

function toggleComposer(): void {
  composerVisible.value = !composerVisible.value;
}

function setViewMode(mode: 'chat' | 'observe'): void {
  viewMode.value = mode;
}

return {
  // ... 现有
  viewMode,
  composerVisible,
  toggleViewMode,
  toggleComposer,
  setViewMode,
};
```

### 6.3 ChatMessagePanel 条件渲染

[ChatMessagePanel.vue](../../../web/src/components/chat/ChatMessagePanel.vue) 消息区改造：

```vue
<template>
  <div class="col column no-wrap chat-message-panel">
    <!-- 消息区/观测区切换 -->
    <div class="col row no-wrap chat-messages-area" style="min-height: 0">
      <!-- 聊天流模式 -->
      <div v-if="viewMode !== 'observe'" class="col column no-wrap chat-messages-main">
        <TodoKanbanBoard v-if="showTodoBoard" :board-state="todoBoardState" />
        <ChatMessageList ... />
      </div>
      <!-- 观测画布模式 -->
      <ObservationPanel
        v-else
        :session-id="sessionId"
        :spirit-session-id="spiritSessionId"
        :is-dark="isDark"
        @select-node="onSelectObserveNode"
        @preview-media="onPreviewMedia"
      />
      <ChatReasoningDrawer ... />
    </div>

    <!-- Composer 受 composerVisible 控制 -->
    <ChatComposer v-if="composerVisible" ... />

    <!-- SpiritStatusBar（透传 viewMode + composerVisible） -->
    <SpiritStatusBar
      v-if="showStatusBar"
      :view-mode="viewMode"
      :composer-visible="composerVisible"
      @toggle-view="onToggleView"
      @toggle-composer="onToggleComposer"
      ... 现有 props
    />
  </div>
</template>
```

### 6.4 ObservationPanel（观测视图容器）

```vue
<!-- web/src/components/chat/observe/ObservationPanel.vue -->
<template>
  <div class="observation-panel">
    <div class="observation-panel__toolbar">
      <q-btn flat dense icon="refresh" @click="refresh" :loading="loading">
        <q-tooltip>{{ t('observe.refresh') }}</q-tooltip>
      </q-btn>
      <q-space />
      <q-badge v-if="liveConnected" rounded color="positive" :label="t('observe.live')" />
    </div>

    <div class="observation-panel__canvas">
      <ObservationCanvas
        v-if="graphStage"
        :graph-stage="graphStage"
        :nodes="nodes"
        @select-node="onSelectNode"
        @preview="onPreview"
      />
      <div v-else class="observation-panel__empty">
        <q-icon name="visibility_off" size="48px" />
        <p>{{ t('observe.noActiveGraph') }}</p>
      </div>
    </div>

    <!-- 节点详情侧栏 -->
    <ObserveNodeDetail
      v-if="selectedNode"
      :node="selectedNode"
      :member-session-id="selectedNodeMemberSessionId"
      @close="selectedNode = null"
      @preview="onPreview"
    />

    <!-- 全屏媒体预览 -->
    <MediaLightbox v-if="previewArtifact" :artifact="previewArtifact" @close="previewArtifact = null" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useSpiritStore } from '../../../stores/spirit';
import { useObserveGraph } from '../../../features/chat/composables/useObserveGraph';

const props = defineProps<{
  sessionId: string;
  spiritSessionId: string;
  isDark: boolean;
}>();

const spiritStore = useSpiritStore();
const { graphStage, nodes } = useObserveGraph(computed(() => props.spiritSessionId));

const selectedNode = ref<GraphNode | null>(null);
const previewArtifact = ref<MediaArtifact | null>(null);
const liveConnected = computed(() => /* 从 wsStore 获取 */ true);

function onSelectNode(node: GraphNode) {
  selectedNode.value = node;
}

function onPreview(art: MediaArtifact) {
  previewArtifact.value = art;
}
</script>
```

### 6.5 ObserveNodeDetail（节点详情侧栏）

复用现有 [MemberSessionPanel.vue](../../../web/src/components/chat/v2/MemberSessionPanel.vue) 展示该成员的完整活动流，外加媒体产出列表：

```vue
<!-- web/src/components/chat/observe/ObserveNodeDetail.vue -->
<template>
  <div class="observe-node-detail">
    <header class="observe-node-detail__header">
      <EntityAvatar :agent-key="node.AgentKey" size="32px" />
      <div>
        <h3>{{ node.Label }}</h3>
        <ObserveStatusBadge :status="node.Status" />
      </div>
      <q-btn flat dense icon="close" @click="$emit('close')" />
    </header>

    <!-- 媒体产出列表 -->
    <section v-if="mediaOutputs.length" class="observe-node-detail__media">
      <h4>{{ t('observe.mediaOutputs') }}</h4>
      <div class="observe-node-detail__media-grid">
        <NodeMediaPreview :artifacts="mediaOutputs" @preview="$emit('preview', $event)" />
      </div>
    </section>

    <!-- 成员活动流（复用现有组件） -->
    <section class="observe-node-detail__activity">
      <h4>{{ t('observe.activityStream') }}</h4>
      <MemberSessionPanel :member-session-id="memberSessionId" />
    </section>
  </div>
</template>
```

### 6.6 i18n 新增

```json
// web/src/i18n/zh-CN/spirit.json
{
  "spirit": {
    "observeView": "观测视图",
    "backToChat": "返回聊天"
  },
  "observe": {
    "refresh": "刷新",
    "live": "实时",
    "noActiveGraph": "当前无活跃的团队执行",
    "mediaOutputs": "媒体产出",
    "activityStream": "活动流"
  }
}
```

### 6.7 Phase 3 改动文件清单

| 文件 | 改动 |
|------|------|
| `web/src/components/spirit/SpiritStatusBar.vue` | 末尾新增切换按钮 + composer toggle |
| `web/src/stores/spirit/index.ts` | 新增 viewMode + composerVisible |
| `web/src/components/chat/ChatMessagePanel.vue` | 条件渲染 + 透传 props |
| `web/src/components/chat/observe/ObservationPanel.vue` | 新建（Phase 2 已列） |
| `web/src/components/chat/observe/ObserveNodeDetail.vue` | 新建：节点详情侧栏 |
| `web/src/i18n/zh-CN/*.json` | 新增 observe 相关键 |

---

## 七、Phase 4：节点级进度 + 视频团队模板

### 7.1 节点级进度推送（对标 ComfyUI progress 事件）

工具执行中通过 ActivityEvent updated 事件推送 `meta.progress`：

```go
// internal/tools/media/generate_video.go
func executeGenerateVideo(ctx context.Context, mp media.MediaProvider, ...) error {
    // 提交异步任务
    taskID, err := mp.SubmitVideoJob(ctx, req)
    if err != nil {
        return err
    }

    // 轮询进度，推送 Activity 更新
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            status, err := mp.GetJobStatus(ctx, taskID)
            if err != nil {
                continue
            }
            // 推送进度到 Activity.Meta
            PublishActivityProgress(ctx, ActivityProgress{
                Value: status.Progress, Max: 100,
                Label: fmt.Sprintf("视频生成 %d%%", status.Progress),
            })
            if status.Status == "completed" {
                return nil  // 完成，工具返回结果
            }
        }
    }
}
```

前端 ObserveNode 在 status=running 时读取 `activity.Meta.progress` 显示进度条。

### 7.2 视频制作团队模板

通过现有 Agent + Team 系统配置，无需新代码：

| 角色 | LLM Provider（思考） | 媒体工具 | DAG 依赖 |
|------|---------------------|---------|---------|
| 导演（Director） | OpenAI/Qwen（plan_and_execute） | 无 | 入口 |
| 编剧（Scriptwriter） | OpenAI/Qwen（纯 LLM） | 无 | ← 导演 |
| 图像师（ImageGenerator） | OpenAI/Qwen | generate_image | ← 编剧 |
| 视频师（VideoGenerator） | OpenAI/Qwen | generate_video, image_to_video | ← 图像师 |
| 剪辑师（Editor） | OpenAI/Qwen | generate_video | ← 视频师 |

配置步骤：
1. Agent 管理页创建 5 个 Agent，各自配置 LLM provider/model + 媒体工具
2. Team 管理页创建 Team，定义 5 个成员的 DAG 依赖
3. 用户在聊天中对 Spirit 说"制作 XX 主题的视频"
4. Spirit 调用 plan_and_execute，自动组建团队

### 7.3 Phase 4 改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/tools/media/generate_video.go` | 新增进度推送逻辑 |
| `internal/tools/media/image_to_video.go` | 新增进度推送逻辑 |
| `internal/tools/media/progress.go` | 新建：PublishActivityProgress 辅助函数 |
| 团队模板配置 | 通过现有 UI 配置（无代码改动） |

---

## 八、数据流总览

```
用户在聊天输入"制作宣传片"
    ↓
Spirit（LLM 思考）调 plan_and_execute → 组建视频团队
    ↓
Team 编译为规划图（GraphDef，internal/graph/builder.go）
    ↓
GraphAgent 执行 → 产生 ActivityEvent 流
    ↓
各成员 Agent（各自 LLM 思考）→ 调用媒体工具
    ↓ 工具调用 MediaProvider（ComfyUI/Qwen/Kling）
    ↓ 媒体生成（异步任务，定期推送 progress）
    ↓ 结果存为 Artifact，引用写入 Activity.Meta
WebSocket → activityV2Store
    ↓
两条消费路径：
  1. 聊天流（viewMode='chat'）：
     ActionBlock → MediaToolDetail（媒体卡片）
  2. 观测画布（viewMode='observe'）：
     useObserveGraph 转换 → ObservationCanvas（Vue Flow）
       → ObserveNode（节点内媒体预览 + 进度条 + 实时状态）
       → nodeOutputStore（管理媒体 URL 生命周期）
    ↓
状态栏切换按钮全屏切换 chat ↔ observe
Composer 独立可开关（composerVisible）
```

---

## 九、接口契约

### 9.1 MediaProvider 接口

```go
type MediaProvider interface {
    Name() string
    GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
    GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error)
    ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error)
}
```

### 9.2 工具输入输出

```go
// generate_image
type GenerateImageInput struct {
    Prompt string `json:"prompt" jsonschema:"description=图像描述,required"`
    Size   string `json:"size,omitempty" jsonschema:"description=尺寸,enum=1024x1024,enum=1792x1024,enum=1024x1792"`
    Style  string `json:"style,omitempty" jsonschema:"description=风格,enum=realistic,enum=anime,enum=oil_painting"`
    Count  int    `json:"count,omitempty" jsonschema:"description=生成数量,1-4"`
}
type GenerateImageOutput struct {
    Artifacts []MediaArtifact `json:"artifacts"`
}

// generate_video
type GenerateVideoInput struct {
    Prompt     string `json:"prompt" jsonschema:"description=视频描述,required"`
    DurationMs int64  `json:"duration_ms,omitempty" jsonschema:"description=时长,1000-30000"`
    FPS        int    `json:"fps,omitempty" jsonschema:"description=帧率,24-60"`
    Resolution string `json:"resolution,omitempty" jsonschema:"description=分辨率,enum=720p,enum=1080p"`
}
type GenerateVideoOutput struct {
    Artifacts []MediaArtifact `json:"artifacts"`
}

// image_to_video
type ImageToVideoInput struct {
    ImageArtifactID string `json:"image_artifact_id" jsonschema:"description=输入图片ID,required"`
    Prompt          string `json:"prompt,omitempty" jsonschema:"description=运动描述"`
    DurationMs      int64  `json:"duration_ms,omitempty"`
    FPS             int    `json:"fps,omitempty"`
}
type ImageToVideoOutput struct {
    Artifacts []MediaArtifact `json:"artifacts"`
}
```

### 9.3 Activity.Meta 约定

| 字段 | 类型 | 含义 |
|------|------|------|
| `media_artifacts` | `[]MediaArtifact` | 工具产出的媒体列表 |
| `progress` | `{value, max, label}` | 工具执行进度（长任务） |

### 9.4 前端 Store 接口

```typescript
// nodeOutputStore
setNodeOutput(nodeId: string, artifacts: MediaArtifact[]): void
appendNodeOutput(nodeId: string, artifact: MediaArtifact): void
getNodeOutput(nodeId: string): MediaArtifact[]
clearSession(): void

// spiritStore
viewMode: 'chat' | 'observe'
composerVisible: boolean
toggleViewMode(): void
toggleComposer(): void
setViewMode(mode: 'chat' | 'observe'): void
```

---

## 十、风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| Vue Flow 节点内放视频性能 | 中 | 缩略图用 `<video poster>` + `preload="metadata"`，点击才加载完整视频 |
| 媒体生成耗时长（视频 30s+） | 中 | Phase 4 节点级进度条 + 工具异步轮询 |
| 媒体文件存储成本 | 低 | Artifact 支持 S3/COS，缩略图懒加载 |
| Activity.Meta 约定 vs Schema 改动 | 低 | 媒体产出引用走 `Activity.Meta`（不动 `activities` 表）；`media_providers` 是新增配置表，通过 DDL Migration Registry 管理 |
| MediaProvider 配置管理 | 低 | 参考 agent_catalog 模式，新建 media_providers 表（独立配置，不污染 Agent.config_json） |
| ComfyUI 本地部署依赖 GPU | 中 | Provider 可插拔，ComfyUI 不可用时降级到 Qwen/Kling API |
| 观测画布与聊天流状态同步 | 中 | 共用 activityV2Store 作为唯一数据源，nodeOutputStore 派生 |
| Composer 在 observe 模式下的输入行为 | 低 | 复用现有 ChatComposer，输入仍走聊天逻辑，与观测视图解耦 |

---

## 十一、验收标准

### 11.1 Phase 1 验收

- [ ] `internal/provider/media/` 接口定义完整，至少 1 个 Provider 实现（ComfyUI 或 Qwen）
- [ ] 3 个媒体生成工具注册成功，Agent 可调用
- [ ] 工具结果存为 Artifact，Activity.Meta 携带 media_artifacts
- [ ] `go test ./internal/provider/media/... ./internal/tools/media/... -count=1` 通过
- [ ] `make wire && make build` 通过

### 11.2 Phase 2 验收

- [ ] ObservationCanvas 能从 activityV2Store 渲染成员节点
- [ ] ObserveNode 显示状态/进度/媒体预览/最新活动
- [ ] nodeOutputStore 正确同步 activityV2Store 的 media_artifacts
- [ ] NodeMediaPreview 正确渲染图片/视频缩略图
- [ ] MediaLightbox 全屏预览可用
- [ ] `cd web && pnpm lint && pnpm test` 通过

### 11.3 Phase 3 验收

- [ ] SpiritStatusBar 右侧显示视图切换按钮
- [ ] 点击切换按钮全屏切换 chat ↔ observe
- [ ] observe 模式下 Composer 可独立开关
- [ ] 聊天流逻辑不受影响（viewMode='chat' 时行为完全一致）
- [ ] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### 11.4 Phase 4 验收

- [ ] 视频生成工具执行时节点显示进度条
- [ ] 视频制作团队模板可配置
- [ ] 完整流程：聊天输入任务 → 团队执行 → 观测视图实时显示各成员产出

---

## 十二、与 ComfyUI 对比

| 维度 | ComfyUI | 本方案 |
|------|---------|--------|
| 定位 | 独立媒体生成工作流工具 | Agent 编排平台的媒体生成场景 |
| 节点图引擎 | LiteGraph (Canvas2D) → Nodes 2.0 (Vue) | Vue Flow (SVG/DOM) |
| 节点内预览 | ✅（旧版难，2.0 易） | ✅（Vue Flow 天然支持） |
| 执行模型 | DAG 节点直接执行 | Agent 调用工具（更灵活） |
| 聊天集成 | ❌ | ✅ 双模式切换（差异化优势） |
| 实时预览帧 | ✅ 二进制 WebSocket | ❌（Agent 模型不需要，用进度条替代） |
| 队列管理 | ✅ | ❌（用 Session/Run 替代） |
| 规划图 vs 观测图 | 混合 | 明确分离（GraphDef 规划 + GraphStage 观测） |

**差异化优势**：本方案在"Agent 驱动的媒体生成 + 聊天集成实时观测"场景上比 ComfyUI 更适合——ComfyUI 是"人手动搭工作流"，本方案是"Agent 自动编排 + 人实时观测"。

---

## 十三、工作量估算

| 阶段 | 内容 | 估算 |
|------|------|------|
| Phase 1 | MediaProvider + 3 工具 | 3 天 |
| Phase 2 | 观测画布 + 节点内预览 | 4 天 |
| Phase 3 | 状态栏切换 + 双状态 | 2 天 |
| Phase 4 | 节点进度 + 团队模板 | 1 天 |
| **合计** | | **~10 天** |

---

## 十四、文档同步（DOC-SYNC 纪律）

本设计实施时需同步更新的文档：

| 文档 | 同步内容 |
|------|---------|
| `docs/development/0-system-diagram.md` | 新增 MediaProvider 模块、观测视图模块 |
| `docs/development/65-module-cross-reference-full.md` | 新增 media provider / media tools / observation view 卡片 |
| 相关模块 `.design.md` | 若有 media 模块设计文档，记录接口契约 |

**豁免**：Phase 1-3 用 Activity.Meta 约定字段，不改 Ent Schema，无需数据库文档同步。Phase 1 新建 `media_providers` 表需同步数据库架构文档。
