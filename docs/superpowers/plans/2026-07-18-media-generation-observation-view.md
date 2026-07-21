# 媒体生成 + 成员节点观测视图实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Aranea-Agents 添加媒体生成能力（文生图/文生视频/图生视频）和聊天 UI 内 ComfyUI 风格的成员节点实时观测视图。

**Architecture:** 后端新建独立 MediaProvider 系统（不混入 LLM Provider），通过 3 个 FunctionTool 暴露给 Agent；前端新建 ObservationCanvas（Vue Flow）观测画布，数据源复用 activityV2Store，通过 SpiritStatusBar 右侧切换按钮全屏切换 chat ↔ observe 视图。

**Tech Stack:** Go, Kratos v2, Ent ORM, SQLite, Wire DI, Vue 3, Quasar, Pinia, TypeScript, Vue Flow

**Spec:** `docs/superpowers/specs/2026-07-18-media-generation-observation-view-design.md`

---

## File Structure

### Phase 1: 新建文件（后端）

| 文件 | 职责 |
|------|------|
| `internal/provider/media/provider.go` | MediaProvider 接口 + 类型定义 |
| `internal/provider/media/registry.go` | 注册中心（参考 register_extra.go 模式） |
| `internal/provider/media/comfyui_local.go` | ComfyUI 本地实现 |
| `internal/provider/media/qwen.go` | 通义万相实现 |
| `internal/provider/media/provider_test.go` | 接口 + 注册中心测试 |
| `internal/tools/media/generate_image.go` | 文生图工具 |
| `internal/tools/media/generate_video.go` | 文生视频工具 |
| `internal/tools/media/image_to_video.go` | 图生视频工具 |
| `internal/tools/media/toolset.go` | 工具注册到 Registry |
| `internal/tools/media/progress.go` | 进度推送辅助函数（Phase 4） |
| `internal/tools/media/generate_image_test.go` | 工具测试 |
| `internal/data/ent/schema/media_provider.go` | Ent Schema |
| `sql/migrations/20260718_media_providers.sql` | DDL 迁移 |

### Phase 1: 修改文件（后端）

| 文件 | 改动 |
|------|------|
| `internal/biz/activity.go` | ToolCategory 新增 `media` 常量 |
| `internal/data/ddl_migration_registry.go` | 注册新迁移 |
| `internal/tools/toolset.go` | Registry() 中注册媒体工具条目 |

### Phase 2: 新建文件（前端）

| 文件 | 职责 |
|------|------|
| `web/src/features/chat/mediaTypes.ts` | MediaArtifact / MediaProgress 类型 |
| `web/src/stores/chat/nodeOutputStore.ts` | 节点产出 Store（对标 ComfyUI NodeOutputStore） |
| `web/src/stores/chat/__tests__/nodeOutputStore.spec.ts` | Store 测试 |
| `web/src/features/chat/composables/useObserveGraph.ts` | 数据转换 composable |
| `web/src/components/chat/observe/ObservationPanel.vue` | 观测视图容器 |
| `web/src/components/chat/observe/ObservationCanvas.vue` | Vue Flow 画布 |
| `web/src/components/chat/observe/ObserveNode.vue` | ComfyUI 风格节点 |
| `web/src/components/chat/observe/ObserveStatusBadge.vue` | 节点状态徽章 |
| `web/src/components/chat/observe/NodeMediaPreview.vue` | 节点内媒体预览 |
| `web/src/components/chat/observe/MediaLightbox.vue` | 全屏媒体预览 |
| `web/src/components/chat/observe/ObserveNodeDetail.vue` | 节点详情侧栏 |
| `web/src/components/chat/tools/MediaToolDetail.vue` | 聊天流媒体渲染 |

### Phase 2: 修改文件（前端）

| 文件 | 改动 |
|------|------|
| `web/src/components/chat/tools/classifyTool.ts` | ToolCategory 新增 `media`，新增 MEDIA_TOOLS 集合 |
| `web/src/components/chat/ActionBlock.vue` | switch 新增 media 分支 → MediaToolDetail |
| `web/src/stores/chat/activityV2Store.ts` | upsertStep 中新增 media_artifacts 同步到 nodeOutputStore |

### Phase 3: 修改文件（前端）

| 文件 | 改动 |
|------|------|
| `web/src/stores/spirit/index.ts` | 新增 viewMode / composerVisible 状态 + toggle 方法 |
| `web/src/components/spirit/SpiritStatusBar.vue` | 新增视图切换按钮 + composer toggle 按钮 |
| `web/src/components/chat/ChatMessagePanel.vue` | 条件渲染 chat/observe + 透传 props |
| `web/src/i18n/zh-CN/spirit.json` | 新增 observe 相关键 |
| `web/src/i18n/en-US/spirit.json` | 新增 observe 相关键 |

---

## Phase 1: MediaProvider 系统 + 媒体生成工具（后端）

### Task 1: MediaProvider 接口 + 类型定义

**Files:**
- Create: `internal/provider/media/provider.go`
- Test: `internal/provider/media/provider_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/provider/media/provider_test.go
package media

import (
	"context"
	"testing"
)

func TestMediaProviderInterface(t *testing.T) {
	// Verify interface is satisfiable
	var _ MediaProvider = (*mockProvider)(nil)
}

type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) GenerateImage(_ context.Context, _ ImageRequest) (*ImageResult, error) {
	return &ImageResult{Artifacts: []MediaArtifact{{ArtifactID: "a1", MimeType: "image/png"}}}, nil
}
func (m *mockProvider) GenerateVideo(_ context.Context, _ VideoRequest) (*VideoResult, error) {
	return &VideoResult{Artifacts: []MediaArtifact{{ArtifactID: "v1", MimeType: "video/mp4"}}}, nil
}
func (m *mockProvider) ImageToVideo(_ context.Context, _ ImageToVideoRequest) (*VideoResult, error) {
	return &VideoResult{Artifacts: []MediaArtifact{{ArtifactID: "v2", MimeType: "video/mp4"}}}, nil
}

func TestImageRequestFields(t *testing.T) {
	req := ImageRequest{Prompt: "a cat", Size: "1024x1024", Count: 1}
	if req.Prompt != "a cat" {
		t.Errorf("expected prompt 'a cat', got %q", req.Prompt)
	}
}

func TestMediaArtifactFields(t *testing.T) {
	a := MediaArtifact{
		ArtifactID: "art_1", URL: "https://example.com/img.png",
		MimeType: "image/png", Width: 1024, Height: 1024,
	}
	if a.Width != 1024 {
		t.Errorf("expected width 1024, got %d", a.Width)
	}
}
```

Run: `go test ./internal/provider/media/ -run TestMediaProvider -v`
Expected: FAIL — package does not exist

- [ ] **Step 2: Write minimal implementation**

```go
// internal/provider/media/provider.go

// Package media defines the MediaProvider interface for media generation
// (text-to-image, text-to-video, image-to-video). It is independent of the
// LLM Provider system (internal/provider/) because media generation is an
// asynchronous long-running task (submit → poll → fetch result), which is
// fundamentally different from LLM sync/stream text generation.
package media

import "context"

// MediaProvider generates media content (images, videos).
type MediaProvider interface {
	Name() string
	GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error)
	GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error)
	ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error)
}

// ImageRequest describes a text-to-image generation request.
type ImageRequest struct {
	Prompt string
	Size   string // "1024x1024" / "1792x1024" / "1024x1792"
	Style  string // "realistic" / "anime" / "oil_painting" / ""
	Count  int    // 1-4
	Seed   int64  // 0 = random
}

// ImageResult contains generated image artifacts.
type ImageResult struct {
	Artifacts []MediaArtifact
}

// VideoRequest describes a text-to-video generation request.
type VideoRequest struct {
	Prompt     string
	DurationMs int64
	FPS        int
	Resolution string // "720p" / "1080p"
	Seed       int64
}

// ImageToVideoRequest describes an image-to-video generation request.
type ImageToVideoRequest struct {
	ImageArtifactID string // input image Artifact ID
	Prompt          string
	DurationMs      int64
	FPS             int
}

// VideoResult contains generated video artifacts.
type VideoResult struct {
	Artifacts []MediaArtifact
}

// MediaArtifact represents a single generated media file.
type MediaArtifact struct {
	ArtifactID string `json:"artifact_id"`
	URL        string `json:"url"`
	MimeType   string `json:"mime_type"` // "image/png" / "video/mp4"
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Thumbnail  string `json:"thumbnail,omitempty"` // video poster
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/provider/media/ -run TestMediaProvider -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/provider/media/provider.go internal/provider/media/provider_test.go
git commit -m "feat(provider/media): add MediaProvider interface and type definitions"
```

---

### Task 2: Provider 注册中心

**Files:**
- Create: `internal/provider/media/registry.go`
- Modify: `internal/provider/media/provider_test.go` (add registry tests)

- [ ] **Step 1: Write the failing test**

Add to `provider_test.go`:

```go
func TestRegistry(t *testing.T) {
	// Register a mock constructor
	Register("test_mock", func(cfg ProviderConfig) (MediaProvider, error) {
		return &mockProvider{}, nil
	})

	p, err := Get("test_mock", ProviderConfig{Name: "test_mock"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if p.Name() != "mock" {
		t.Errorf("expected name 'mock', got %q", p.Name())
	}
}

func TestRegistryNotFound(t *testing.T) {
	_, err := Get("nonexistent", ProviderConfig{Name: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unregistered provider")
	}
}
```

Run: `go test ./internal/provider/media/ -run TestRegistry -v`
Expected: FAIL — `Register` and `Get` undefined

- [ ] **Step 2: Write minimal implementation**

```go
// internal/provider/media/registry.go
package media

import "fmt"

// ProviderConfig holds the configuration for a media provider instance.
type ProviderConfig struct {
	Name         string
	ProviderType string // "comfyui_local" / "qwen" / "kling"
	BaseURL      string
	APIKey       string
	Extra        map[string]any
}

// MediaProviderConstructor creates a MediaProvider from config.
type MediaProviderConstructor func(cfg ProviderConfig) (MediaProvider, error)

var providers = map[string]MediaProviderConstructor{}

// Register adds a provider constructor to the registry.
// Must be called during application startup (Wire provider or init).
func Register(name string, c MediaProviderConstructor) {
	providers[name] = c
}

// Get returns a MediaProvider by name.
func Get(name string, cfg ProviderConfig) (MediaProvider, error) {
	c, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("media provider %q not registered", name)
	}
	return c(cfg)
}

// RegisteredNames returns all registered provider names.
func RegisteredNames() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	return names
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/provider/media/ -run TestRegistry -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/provider/media/registry.go internal/provider/media/provider_test.go
git commit -m "feat(provider/media): add provider registry with Register/Get"
```

---

### Task 3: QwenProvider 实现（通义万相）

**Files:**
- Create: `internal/provider/media/qwen.go`

- [ ] **Step 1: Write the failing test**

Add to `provider_test.go`:

```go
func TestQwenProviderName(t *testing.T) {
	p := NewQwenProvider(ProviderConfig{
		ProviderType: "qwen",
		APIKey:       "test-key",
		BaseURL:      "https://dashscope.aliyuncs.com",
	})
	if p.Name() != "qwen" {
		t.Errorf("expected name 'qwen', got %q", p.Name())
	}
}
```

Run: `go test ./internal/provider/media/ -run TestQwen -v`
Expected: FAIL — `NewQwenProvider` undefined

- [ ] **Step 2: Write minimal implementation**

```go
// internal/provider/media/qwen.go
package media

import (
	"context"
	"fmt"
)

// QwenProvider implements MediaProvider for Alibaba Tongyi Wanxiang (通义万相).
// Supports text-to-image and text-to-video via DashScope API.
type QwenProvider struct {
	cfg ProviderConfig
}

// NewQwenProvider creates a new QwenProvider.
func NewQwenProvider(cfg ProviderConfig) MediaProvider {
	return &QwenProvider{cfg: cfg}
}

func (p *QwenProvider) Name() string { return "qwen" }

func (p *QwenProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	// TODO: Implement DashScope text-to-image API call
	// POST /services/aigc/text2image/image-synthesis
	// Async: submit → poll GET /tasks/{task_id} → fetch result
	return nil, fmt.Errorf("qwen GenerateImage not yet implemented")
}

func (p *QwenProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	// TODO: Implement DashScope video-generation API call
	// POST /services/aigc/video-generation/video-synthesis
	return nil, fmt.Errorf("qwen GenerateVideo not yet implemented")
}

func (p *QwenProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("qwen ImageToVideo not yet implemented")
}

func init() {
	Register("qwen", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewQwenProvider(cfg), nil
	})
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/provider/media/ -run TestQwen -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/provider/media/qwen.go
git commit -m "feat(provider/media): add QwenProvider stub (Tongyi Wanxiang)"
```

---

### Task 4: ComfyUILocalProvider 实现

**Files:**
- Create: `internal/provider/media/comfyui_local.go`

- [ ] **Step 1: Write the failing test**

Add to `provider_test.go`:

```go
func TestComfyUILocalProviderName(t *testing.T) {
	p := NewComfyUILocalProvider(ProviderConfig{
		ProviderType: "comfyui_local",
		BaseURL:      "http://127.0.0.1:8188",
	})
	if p.Name() != "comfyui_local" {
		t.Errorf("expected name 'comfyui_local', got %q", p.Name())
	}
}
```

Run: `go test ./internal/provider/media/ -run TestComfyUI -v`
Expected: FAIL — `NewComfyUILocalProvider` undefined

- [ ] **Step 2: Write minimal implementation**

```go
// internal/provider/media/comfyui_local.go
package media

import (
	"context"
	"fmt"
)

// ComfyUILocalProvider implements MediaProvider for a locally deployed ComfyUI.
// Communicates via ComfyUI HTTP API (POST /prompt + GET /history/{prompt_id})
// and WebSocket (ws://<host>/ws) for execution progress.
type ComfyUILocalProvider struct {
	cfg ProviderConfig
}

// NewComfyUILocalProvider creates a new ComfyUILocalProvider.
func NewComfyUILocalProvider(cfg ProviderConfig) MediaProvider {
	return &ComfyUILocalProvider{cfg: cfg}
}

func (p *ComfyUILocalProvider) Name() string { return "comfyui_local" }

func (p *ComfyUILocalProvider) GenerateImage(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	// TODO: Build ComfyUI workflow JSON from ImageRequest
	// POST /prompt → poll GET /history/{prompt_id}
	return nil, fmt.Errorf("comfyui_local GenerateImage not yet implemented")
}

func (p *ComfyUILocalProvider) GenerateVideo(ctx context.Context, req VideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local GenerateVideo not yet implemented")
}

func (p *ComfyUILocalProvider) ImageToVideo(ctx context.Context, req ImageToVideoRequest) (*VideoResult, error) {
	return nil, fmt.Errorf("comfyui_local ImageToVideo not yet implemented")
}

func init() {
	Register("comfyui_local", func(cfg ProviderConfig) (MediaProvider, error) {
		return NewComfyUILocalProvider(cfg), nil
	})
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/provider/media/ -run TestComfyUI -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/provider/media/comfyui_local.go
git commit -m "feat(provider/media): add ComfyUILocalProvider stub"
```

---

### Task 5: media_providers 表 Schema + DDL 迁移

**Files:**
- Create: `internal/data/ent/schema/media_provider.go`
- Create: `sql/migrations/20260718_media_providers.sql`
- Modify: `internal/data/ddl_migration_registry.go`

- [ ] **Step 1: Create Ent Schema**

```go
// internal/data/ent/schema/media_provider.go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema"
	"entgo.io/ent/entsql"
)

// MediaProvider holds configuration for media generation providers.
type MediaProvider struct {
	ent.Schema
}

func (MediaProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "media_providers"},
	}
}

func (MediaProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").NotEmpty().Unique(),
		field.String("name").NotEmpty().Unique(),
		field.String("provider_type").NotEmpty(), // "comfyui_local" / "qwen" / "kling"
		field.String("base_url").Default(""),
		field.String("api_key").Default("").Sensitive(), // DB-N8: sensitive field
		field.String("config_json").Default("{}"),
		field.String("capabilities").Default("[]"), // JSON array: ["image","video","image_to_video"]
		field.String("status").Default("active"),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
	}
}
```

- [ ] **Step 2: Create DDL migration SQL**

```sql
-- sql/migrations/20260718_media_providers.sql
CREATE TABLE IF NOT EXISTS media_providers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider_type TEXT NOT NULL,
    base_url TEXT DEFAULT '',
    api_key TEXT DEFAULT '',
    config_json TEXT DEFAULT '{}',
    capabilities TEXT DEFAULT '[]',
    status TEXT DEFAULT 'active',
    created_at TEXT DEFAULT '',
    updated_at TEXT DEFAULT ''
);
```

- [ ] **Step 3: Register migration in ddl_migration_registry.go**

In `internal/data/ddl_migration_registry.go`, add to `ddlMigrations`:

```go
{Version: 20260718, Name: "media_providers", SQL: "sql/migrations/20260718_media_providers.sql"},
```

- [ ] **Step 4: Regenerate Ent code**

Run: `go generate ./internal/data/ent`
Expected: New files generated for MediaProvider entity

- [ ] **Step 5: Verify build**

Run: `go build -tags=pgvector ./internal/data/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/data/ent/schema/media_provider.go sql/migrations/20260718_media_providers.sql internal/data/ddl_migration_registry.go internal/data/ent/
git commit -m "feat(data): add media_providers table schema and DDL migration"
```

---

### Task 6: ToolCategory 新增 media

**Files:**
- Modify: `internal/biz/activity.go`

- [ ] **Step 1: Add media constant**

In `internal/biz/activity.go`, add to the ToolCategory constants block (after `ToolCategoryOther`):

```go
ToolCategoryMedia ToolCategory = "media" // Media generation (image/video)
```

- [ ] **Step 2: Verify build**

Run: `go build ./internal/biz/`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/biz/activity.go
git commit -m "feat(biz): add ToolCategoryMedia constant"
```

---

### Task 7: 媒体生成工具（generate_image / generate_video / image_to_video）

**Files:**
- Create: `internal/tools/media/generate_image.go`
- Create: `internal/tools/media/generate_video.go`
- Create: `internal/tools/media/image_to_video.go`
- Create: `internal/tools/media/toolset.go`
- Test: `internal/tools/media/generate_image_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/media/generate_image_test.go
package media

import (
	"context"
	"testing"

	"aranea-agents/internal/provider/media"
)

func TestGenerateImageToolName(t *testing.T) {
	// Verify tool can be created with a nil provider (stub)
	tool := NewGenerateImageTool(nil, nil)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	decl := tool.Declaration()
	if decl == nil {
		t.Fatal("expected non-nil declaration")
	}
	if decl.Name != "generate_image" {
		t.Errorf("expected name 'generate_image', got %q", decl.Name)
	}
}

func TestGenerateImageInputValidation(t *testing.T) {
	// Empty prompt should be caught
	_, err := executeGenerateImage(context.Background(), nil, GenerateImageInput{Prompt: ""})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}
```

Run: `go test ./internal/tools/media/ -run TestGenerateImage -v`
Expected: FAIL — package does not exist

- [ ] **Step 2: Write generate_image.go**

```go
// internal/tools/media/generate_image.go
package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// GenerateImageInput is the input for the generate_image tool.
type GenerateImageInput struct {
	Prompt string `json:"prompt" jsonschema:"description=图像描述,required"`
	Size   string `json:"size,omitempty" jsonschema:"description=尺寸,enum=1024x1024,enum=1792x1024,enum=1024x1792"`
	Style  string `json:"style,omitempty" jsonschema:"description=风格,enum=realistic,enum=anime,enum=oil_painting"`
	Count  int    `json:"count,omitempty" jsonschema:"description=生成数量 1-4"`
}

// GenerateImageOutput is the output for the generate_image tool.
type GenerateImageOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewGenerateImageTool creates the generate_image tool.
func NewGenerateImageTool(mp media.MediaProvider, artifactSaver ArtifactSaver) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in GenerateImageInput) (GenerateImageOutput, error) {
			return executeGenerateImage(ctx, mp, in)
		},
	)
}

func executeGenerateImage(ctx context.Context, mp media.MediaProvider, in GenerateImageInput) (GenerateImageOutput, error) {
	if in.Prompt == "" {
		return GenerateImageOutput{}, fmt.Errorf("prompt is required")
	}
	if mp == nil {
		return GenerateImageOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.Count <= 0 {
		in.Count = 1
	}
	if in.Count > 4 {
		in.Count = 4
	}
	if in.Size == "" {
		in.Size = "1024x1024"
	}

	result, err := mp.GenerateImage(ctx, media.ImageRequest{
		Prompt: in.Prompt,
		Size:   in.Size,
		Style:  in.Style,
		Count:  in.Count,
	})
	if err != nil {
		return GenerateImageOutput{}, fmt.Errorf("generate image: %w", err)
	}
	return GenerateImageOutput{Artifacts: result.Artifacts}, nil
}
```

- [ ] **Step 3: Write generate_video.go**

```go
// internal/tools/media/generate_video.go
package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// GenerateVideoInput is the input for the generate_video tool.
type GenerateVideoInput struct {
	Prompt     string `json:"prompt" jsonschema:"description=视频描述,required"`
	DurationMs int64  `json:"duration_ms,omitempty" jsonschema:"description=时长 1000-30000"`
	FPS        int    `json:"fps,omitempty" jsonschema:"description=帧率 24-60"`
	Resolution string `json:"resolution,omitempty" jsonschema:"description=分辨率,enum=720p,enum=1080p"`
}

// GenerateVideoOutput is the output for the generate_video tool.
type GenerateVideoOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewGenerateVideoTool creates the generate_video tool.
func NewGenerateVideoTool(mp media.MediaProvider, artifactSaver ArtifactSaver) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in GenerateVideoInput) (GenerateVideoOutput, error) {
			return executeGenerateVideo(ctx, mp, in)
		},
	)
}

func executeGenerateVideo(ctx context.Context, mp media.MediaProvider, in GenerateVideoInput) (GenerateVideoOutput, error) {
	if in.Prompt == "" {
		return GenerateVideoOutput{}, fmt.Errorf("prompt is required")
	}
	if mp == nil {
		return GenerateVideoOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.DurationMs <= 0 {
		in.DurationMs = 5000
	}
	if in.FPS <= 0 {
		in.FPS = 24
	}
	if in.Resolution == "" {
		in.Resolution = "720p"
	}

	result, err := mp.GenerateVideo(ctx, media.VideoRequest{
		Prompt:     in.Prompt,
		DurationMs: in.DurationMs,
		FPS:        in.FPS,
		Resolution: in.Resolution,
	})
	if err != nil {
		return GenerateVideoOutput{}, fmt.Errorf("generate video: %w", err)
	}
	return GenerateVideoOutput{Artifacts: result.Artifacts}, nil
}
```

- [ ] **Step 4: Write image_to_video.go**

```go
// internal/tools/media/image_to_video.go
package media

import (
	"context"
	"fmt"

	"aranea-agents/internal/provider/media"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ImageToVideoInput is the input for the image_to_video tool.
type ImageToVideoInput struct {
	ImageArtifactID string `json:"image_artifact_id" jsonschema:"description=输入图片 Artifact ID,required"`
	Prompt          string `json:"prompt,omitempty" jsonschema:"description=运动描述"`
	DurationMs      int64  `json:"duration_ms,omitempty"`
	FPS             int    `json:"fps,omitempty"`
}

// ImageToVideoOutput is the output for the image_to_video tool.
type ImageToVideoOutput struct {
	Artifacts []media.MediaArtifact `json:"artifacts"`
}

// NewImageToVideoTool creates the image_to_video tool.
func NewImageToVideoTool(mp media.MediaProvider, artifactSaver ArtifactSaver) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in ImageToVideoInput) (ImageToVideoOutput, error) {
			return executeImageToVideo(ctx, mp, in)
		},
	)
}

func executeImageToVideo(ctx context.Context, mp media.MediaProvider, in ImageToVideoInput) (ImageToVideoOutput, error) {
	if in.ImageArtifactID == "" {
		return ImageToVideoOutput{}, fmt.Errorf("image_artifact_id is required")
	}
	if mp == nil {
		return ImageToVideoOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.DurationMs <= 0 {
		in.DurationMs = 5000
	}
	if in.FPS <= 0 {
		in.FPS = 24
	}

	result, err := mp.ImageToVideo(ctx, media.ImageToVideoRequest{
		ImageArtifactID: in.ImageArtifactID,
		Prompt:          in.Prompt,
		DurationMs:      in.DurationMs,
		FPS:             in.FPS,
	})
	if err != nil {
		return ImageToVideoOutput{}, fmt.Errorf("image to video: %w", err)
	}
	return ImageToVideoOutput{Artifacts: result.Artifacts}, nil
}
```

- [ ] **Step 5: Write toolset.go (ArtifactSaver interface + tool registration)**

```go
// internal/tools/media/toolset.go
package media

import (
	"context"

	"aranea-agents/internal/provider/media"
	"aranea-agents/internal/tools"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ArtifactSaver persists generated media as an Artifact and returns its ID + signed URL.
// Implemented by biz.ArtifactUsecase adapter.
type ArtifactSaver interface {
	SaveMedia(ctx context.Context, sessionID string, mimeType string, data []byte) (artifactID string, signedURL string, err error)
}

// NewMediaToolSet creates all media generation tools.
func NewMediaToolSet(mp media.MediaProvider, saver ArtifactSaver) []trpctool.Tool {
	return []trpctool.Tool{
		NewGenerateImageTool(mp, saver),
		NewGenerateVideoTool(mp, saver),
		NewImageToVideoTool(mp, saver),
	}
}

- [ ] **Step 5: Write toolset.go**

```go
// internal/tools/media/toolset.go
package media

import (
	"context"

	"aranea-agents/internal/provider/media"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ArtifactSaver persists generated media as an Artifact and returns its ID + signed URL.
// Implemented by biz.ArtifactUsecase adapter.
type ArtifactSaver interface {
	SaveMedia(ctx context.Context, sessionID string, mimeType string, data []byte) (artifactID string, signedURL string, err error)
}

// NewMediaTools creates all media generation tools.
func NewMediaTools(mp media.MediaProvider, saver ArtifactSaver) []trpctool.Tool {
	return []trpctool.Tool{
		NewGenerateImageTool(mp, saver),
		NewGenerateVideoTool(mp, saver),
		NewImageToVideoTool(mp, saver),
	}
}
```

- [ ] **Step 6: Register media tools in toolset.go Registry()**

In `internal/tools/toolset.go`, add to the `Registry()` hardcoded list (after existing entries):

```go
{
	Name:        "media",
	Description: "Media generation tools (text-to-image, text-to-video, image-to-video)",
	Category:    "media",
	Tags:        []string{"media", "image", "video", "generation"},
	Factory: func(ctx context.Context) (Tool, error) {
		// Media tools require MediaProvider + ArtifactSaver which are not
		// available in the global tool factory context. They are assembled
		// separately via media.NewMediaTools() and injected at the Agent level.
		return nil, nil
	},
	EnabledByDefault: true,
	RiskLevel:        "medium",
},
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/tools/media/ -run TestGenerateImage -v`
Expected: PASS

Run: `go build -tags=pgvector ./internal/tools/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/tools/media/ internal/tools/toolset.go
git commit -m "feat(tools/media): add generate_image, generate_video, image_to_video tools"
```

---

### Task 8: Phase 1 集成验证

**Files:**
- Verify only

- [ ] **Step 1: Full backend build**

Run: `go build -tags=pgvector ./...`
Expected: PASS

- [ ] **Step 2: Run media tests**

Run: `go test -tags=pgvector ./internal/provider/media/... ./internal/tools/media/... -count=1 -v`
Expected: PASS

- [ ] **Step 3: Regenerate wire (verify no breakage)**

Run: `make wire`
Expected: PASS (wire_gen.go may or may not change — media tools are not yet wired)

- [ ] **Step 4: Full backend test suite**

Run: `go test -tags=pgvector ./internal/biz/... ./internal/data/... -count=1`
Expected: PASS (no regressions)

- [ ] **Step 5: Commit (if any changes)**

```bash
git add -A
git commit -m "chore: Phase 1 integration verification"
```

---

## Phase 2: 成员节点观测画布（前端）

### Task 9: 媒体类型定义 + nodeOutputStore

**Files:**
- Create: `web/src/features/chat/mediaTypes.ts`
- Create: `web/src/stores/chat/nodeOutputStore.ts`
- Test: `web/src/stores/chat/__tests__/nodeOutputStore.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// web/src/stores/chat/__tests__/nodeOutputStore.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useNodeOutputStore } from '../nodeOutputStore';

describe('nodeOutputStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('starts empty', () => {
    const store = useNodeOutputStore();
    expect(store.getNodeOutput('node1')).toEqual([]);
  });

  it('setNodeOutput stores artifacts', () => {
    const store = useNodeOutputStore();
    const artifacts = [
      { artifact_id: 'a1', url: 'https://example.com/img.png', mime_type: 'image/png' },
    ];
    store.setNodeOutput('node1', artifacts);
    expect(store.getNodeOutput('node1')).toEqual(artifacts);
  });

  it('appendNodeOutput adds to existing artifacts', () => {
    const store = useNodeOutputStore();
    store.setNodeOutput('node1', [
      { artifact_id: 'a1', url: 'url1', mime_type: 'image/png' },
    ]);
    store.appendNodeOutput('node1', { artifact_id: 'a2', url: 'url2', mime_type: 'video/mp4' });
    expect(store.getNodeOutput('node1')).toHaveLength(2);
  });

  it('clearSession removes all outputs', () => {
    const store = useNodeOutputStore();
    store.setNodeOutput('node1', [{ artifact_id: 'a1', url: 'u', mime_type: 'image/png' }]);
    store.clearSession();
    expect(store.getNodeOutput('node1')).toEqual([]);
  });
});
```

Run: `cd web && pnpm test -- --run src/stores/chat/__tests__/nodeOutputStore.spec.ts`
Expected: FAIL — module does not exist

- [ ] **Step 2: Create mediaTypes.ts**

```typescript
// web/src/features/chat/mediaTypes.ts

/** A single media artifact produced by a media generation tool. */
export interface MediaArtifact {
  artifact_id: string;
  url: string;
  mime_type: string; // "image/png" / "video/mp4"
  width?: number;
  height?: number;
  duration_ms?: number;
  thumbnail?: string; // video poster URL
}

/** Progress info for long-running media generation tasks. */
export interface MediaProgress {
  value: number;
  max: number;
  label?: string;
}
```

- [ ] **Step 3: Create nodeOutputStore.ts**

```typescript
// web/src/stores/chat/nodeOutputStore.ts
import { defineStore } from 'pinia';
import { reactive } from 'vue';
import type { MediaArtifact } from '../../features/chat/mediaTypes';

/**
 * nodeOutputStore manages per-node media outputs for the observation canvas.
 * Analogous to ComfyUI's NodeOutputStore — maps nodeId to its media artifacts.
 */
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

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm test -- --run src/stores/chat/__tests__/nodeOutputStore.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/chat/mediaTypes.ts web/src/stores/chat/nodeOutputStore.ts web/src/stores/chat/__tests__/nodeOutputStore.spec.ts
git commit -m "feat(web): add mediaTypes and nodeOutputStore for observation canvas"
```

---

### Task 10: classifyTool 新增 media 分类 + MediaToolDetail

**Files:**
- Modify: `web/src/components/chat/tools/classifyTool.ts`
- Create: `web/src/components/chat/tools/MediaToolDetail.vue`
- Modify: `web/src/components/chat/ActionBlock.vue`

- [ ] **Step 1: Add media to classifyTool.ts**

In `web/src/components/chat/tools/classifyTool.ts`:

1. Add `'media'` to the `ToolCategory` type union (after `'todo'`):

```typescript
export type ToolCategory =
  | 'shell'
  | 'browser'
  | 'file_read'
  | 'file_write'
  | 'file_search'
  | 'web_search'
  | 'mcp'
  | 'code'
  | 'todo'
  | 'media'
  | 'other';
```

2. Add the media tools set (after `TODO_TOOLS`):

```typescript
/** Media generation tools. */
const MEDIA_TOOLS = new Set([
  'generate_image',
  'generate_video',
  'image_to_video',
]);
```

3. Add media classification in `classifyTool()` (before `return 'other'`):

```typescript
if (MEDIA_TOOLS.has(name)) return 'media';
```

4. Add media icon to `TOOL_CATEGORY_ICON`:

```typescript
media: '🎨',
```

- [ ] **Step 2: Create MediaToolDetail.vue**

```vue
<!-- web/src/components/chat/tools/MediaToolDetail.vue -->
<template>
  <div class="tool-detail">
    <!-- Prompt -->
    <div v-if="prompt" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.prompt') }}</div>
      <code class="tool-detail__inline">{{ prompt }}</code>
    </div>
    <!-- Media artifacts grid -->
    <div v-if="artifacts.length" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.mediaOutputs') }}</div>
      <div class="media-tool-detail__grid">
        <div
          v-for="art in artifacts"
          :key="art.artifact_id"
          class="media-tool-detail__item"
          @click="onPreview(art)"
        >
          <video
            v-if="art.mime_type.startsWith('video/')"
            :src="art.url"
            :poster="art.thumbnail"
            muted
            preload="metadata"
            class="media-tool-detail__media"
          />
          <img
            v-else
            :src="art.url"
            loading="lazy"
            class="media-tool-detail__media"
          />
        </div>
      </div>
    </div>
    <!-- Raw result fallback -->
    <div v-if="!artifacts.length && rawResult" class="tool-detail__row">
      <div class="tool-detail__label">{{ t('chat.toolDetail.result') }}</div>
      <pre class="tool-detail__pre">{{ rawResult }}</pre>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Step } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { asRecord } from './toolDetailShared';

const { t } = useI18n();

const props = defineProps<{ step: Step }>();

const emit = defineEmits<{
  preview: [art: MediaArtifact];
}>();

const parsedArgs = computed(() => asRecord(props.step.ToolArgs));
const parsedResult = computed(() => asRecord(props.step.ToolResult));

const prompt = computed(() => String(parsedArgs.value?.prompt ?? ''));

const artifacts = computed<MediaArtifact[]>(() => {
  const raw = parsedResult.value?.artifacts;
  if (!Array.isArray(raw)) return [];
  return raw.filter(
    (a): a is MediaArtifact =>
      typeof a === 'object' && a !== null && 'artifact_id' in a && 'mime_type' in a,
  );
});

const rawResult = computed(() => {
  if (!parsedResult.value) return '';
  return JSON.stringify(parsedResult.value, null, 2).slice(0, 500);
});

function onPreview(art: MediaArtifact) {
  emit('preview', art);
}
</script>

<style scoped lang="sass">
.media-tool-detail__grid
  display: flex
  gap: 8px
  flex-wrap: wrap
  margin-top: 4px

.media-tool-detail__item
  width: 120px
  height: 120px
  border-radius: 6px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border)
  transition: border-color 0.2s

  &:hover
    border-color: var(--color-primary)

.media-tool-detail__media
  width: 100%
  height: 100%
  object-fit: cover
</style>
```

- [ ] **Step 3: Add media branch to ActionBlock.vue**

In `web/src/components/chat/ActionBlock.vue`:

1. Add import (after `CodeToolDetail`):

```typescript
import MediaToolDetail from './tools/MediaToolDetail.vue';
```

2. Add case in `detailComponent` switch (before `default`):

```typescript
    case 'media':
      return MediaToolDetail;
```

- [ ] **Step 4: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/chat/tools/classifyTool.ts web/src/components/chat/tools/MediaToolDetail.vue web/src/components/chat/ActionBlock.vue
git commit -m "feat(web): add media tool category and MediaToolDetail component"
```

---

### Task 11: activityV2Store 同步 media_artifacts 到 nodeOutputStore

**Files:**
- Modify: `web/src/stores/chat/activityV2Store.ts`

- [ ] **Step 1: Read existing upsertStep to find insertion point**

Read `web/src/stores/chat/activityV2Store.ts` and locate the `upsertStep` function. The media sync logic should be added after the existing upsert logic.

- [ ] **Step 2: Add media sync logic**

In `web/src/stores/chat/activityV2Store.ts`:

1. Add import at the top:

```typescript
import { useNodeOutputStore } from './nodeOutputStore';
import type { MediaArtifact } from '../../features/chat/mediaTypes';
```

2. Inside the store setup function, add the nodeOutputStore reference:

```typescript
const nodeOutputStore = useNodeOutputStore();
```

3. In the `upsertStep` function (or wherever action steps are processed), after the existing logic, add:

```typescript
// Sync media outputs to nodeOutputStore for observation canvas.
// When a media tool completes, extract artifacts from ToolResult and map to the node.
if (s.Kind === 'action' && s.Status === 'completed' && s.ToolName) {
  const mediaTools = ['generate_image', 'generate_video', 'image_to_video'];
  if (mediaTools.includes(s.ToolName)) {
    const result = s.ToolResult as Record<string, unknown> | null;
    const artifacts = result?.artifacts;
    if (Array.isArray(artifacts) && artifacts.length > 0) {
      // Map agent key to node ID via TeamStageID
      const nodeId = s.TeamStageID || s.AuthorAgentKey;
      if (nodeId) {
        nodeOutputStore.setNodeOutput(nodeId, artifacts as MediaArtifact[]);
      }
    }
  }
}
```

- [ ] **Step 3: Verify lint + tests**

Run: `cd web && pnpm lint && pnpm test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/stores/chat/activityV2Store.ts
git commit -m "feat(web): sync media_artifacts from activityV2Store to nodeOutputStore"
```

---

### Task 12: useObserveGraph composable

**Files:**
- Create: `web/src/features/chat/composables/useObserveGraph.ts`

- [ ] **Step 1: Create useObserveGraph.ts**

```typescript
// web/src/features/chat/composables/useObserveGraph.ts
import { computed, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { usePlanDAGLayout } from './usePlanDAGLayout';
import { useNodeOutputStore } from '../../../stores/chat/nodeOutputStore';
import type { GraphStage, GraphNode } from '../v2Types';

/**
 * useObserveGraph transforms activityV2Store data into a format suitable
 * for the ObservationCanvas (Vue Flow). It extracts the GraphStage and its
 * nodes, then computes layout positions using the existing DAG layout algorithm.
 */
export function useObserveGraph(spiritSessionId: Ref<string>) {
  const activityStore = useChatActivityStore();
  const nodeOutputStore = useNodeOutputStore();

  // Find the GraphStage for this spirit session
  const graphStage = computed<GraphStage | null>(() => {
    for (const [, gs] of activityStore.graphStages) {
      if (gs.SessionID === spiritSessionId.value) return gs;
    }
    return null;
  });

  // Get nodes for the current GraphStage
  const nodes = computed<GraphNode[]>(() => {
    if (!graphStage.value) return [];
    return graphStage.value.Nodes || [];
  });

  // Compute DAG layout positions
  const { positions, computedWidth } = usePlanDAGLayout(nodes);

  // Convert GraphNode[] to Vue Flow node format
  const flowNodes = computed(() =>
    nodes.value.map((n) => {
      const pos = positions.value.get(n.ID) || { x: 0, y: 0 };
      return {
        id: n.ID,
        type: 'observe',
        position: { x: pos.x, y: pos.y },
        data: {
          label: n.Label,
          dagNodeId: n.DagNodeID,
          teamStageId: n.TeamStageID,
          status: n.Status,
          dependsOn: n.DependsOn,
          mediaOutput: nodeOutputStore.getNodeOutput(n.TeamStageID || n.ID),
        },
      };
    }),
  );

  // Convert DependsOn to Vue Flow edges
  const flowEdges = computed(() =>
    nodes.value.flatMap((n) =>
      (n.DependsOn || []).map((depId) => ({
        id: `${depId}->${n.ID}`,
        source: depId,
        target: n.ID,
        animated: true,
      })),
    ),
  );

  return {
    graphStage,
    nodes,
    flowNodes,
    flowEdges,
    positions,
    computedWidth,
  };
}
```

- [ ] **Step 2: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/features/chat/composables/useObserveGraph.ts
git commit -m "feat(web): add useObserveGraph composable for observation canvas data"
```

---

### Task 13: ObserveStatusBadge + NodeMediaPreview 组件

**Files:**
- Create: `web/src/components/chat/observe/ObserveStatusBadge.vue`
- Create: `web/src/components/chat/observe/NodeMediaPreview.vue`

- [ ] **Step 1: Create ObserveStatusBadge.vue**

```vue
<!-- web/src/components/chat/observe/ObserveStatusBadge.vue -->
<template>
  <span :class="['observe-status-badge', `observe-status-badge--${status}`]">
    {{ statusLabel }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNodeStatus } from '../../../features/chat/v2Types';

const { t } = useI18n();

const props = defineProps<{ status: GraphNodeStatus }>();

const STATUS_MAP: Record<GraphNodeStatus, string> = {
  pending: 'observe.statusPending',
  running: 'observe.statusRunning',
  completed: 'observe.statusCompleted',
  failed: 'observe.statusFailed',
  interrupted: 'observe.statusInterrupted',
};

const statusLabel = computed(() => t(STATUS_MAP[props.status] || props.status));
</script>

<style scoped lang="sass">
.observe-status-badge
  font-size: 10px
  padding: 1px 6px
  border-radius: 8px
  font-weight: 500

  &--pending
    background: var(--color-surface)
    color: var(--color-text-tertiary)

  &--running
    background: color-mix(in srgb, var(--color-warning) 20%, transparent)
    color: var(--color-warning)

  &--completed
    background: color-mix(in srgb, var(--color-positive) 20%, transparent)
    color: var(--color-positive)

  &--failed
    background: color-mix(in srgb, var(--color-negative) 20%, transparent)
    color: var(--color-negative)

  &--interrupted
    background: color-mix(in srgb, var(--color-warning) 15%, transparent)
    color: var(--color-text-secondary)
</style>
```

- [ ] **Step 2: Create NodeMediaPreview.vue**

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

<style scoped lang="sass">
.node-media-preview
  display: flex
  gap: 4px
  align-items: center
  margin-top: 6px

.node-media-preview__item
  width: 64px
  height: 64px
  border-radius: 4px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border)
  flex-shrink: 0

  &:hover
    border-color: var(--color-primary)

.node-media-preview__media
  width: 100%
  height: 100%
  object-fit: cover

.node-media-preview__more
  font-size: 11px
  color: var(--color-text-tertiary)
  margin-left: 4px
</style>
```

- [ ] **Step 3: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/components/chat/observe/ObserveStatusBadge.vue web/src/components/chat/observe/NodeMediaPreview.vue
git commit -m "feat(web): add ObserveStatusBadge and NodeMediaPreview components"
```

---

### Task 14: ObserveNode 组件（ComfyUI 风格节点）

**Files:**
- Create: `web/src/components/chat/observe/ObserveNode.vue`

- [ ] **Step 1: Create ObserveNode.vue**

```vue
<!-- web/src/components/chat/observe/ObserveNode.vue -->
<template>
  <div :class="['observe-node', `observe-node--${data.status}`]">
    <!-- Header: avatar + name + status -->
    <header class="observe-node__header">
      <span class="observe-node__avatar">{{ agentInitial }}</span>
      <span class="observe-node__name">{{ data.label }}</span>
      <ObserveStatusBadge :status="data.status" />
    </header>

    <!-- Progress bar (running) -->
    <div v-if="data.status === 'running'" class="observe-node__progress">
      <q-linear-progress
        :value="progressValue"
        color="warning"
        size="3px"
        rounded
      />
      <span class="observe-node__progress-label">{{ progressLabel }}</span>
    </div>

    <!-- Media preview -->
    <NodeMediaPreview
      v-if="data.mediaOutput?.length"
      :artifacts="data.mediaOutput"
      @preview="$emit('preview', $event)"
    />

    <!-- Latest activity -->
    <div class="observe-node__activity">
      <q-icon :name="activityIcon" size="12px" />
      <span class="observe-node__activity-text">{{ activitySummary }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { GraphNodeStatus } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import ObserveStatusBadge from './ObserveStatusBadge.vue';
import NodeMediaPreview from './NodeMediaPreview.vue';

interface ObserveNodeData {
  label: string;
  dagNodeId: string;
  teamStageId: string;
  status: GraphNodeStatus;
  dependsOn: string[];
  mediaOutput: MediaArtifact[];
}

const props = defineProps<{ data: ObserveNodeData }>();
defineEmits<{ preview: [art: MediaArtifact] }>();

const agentInitial = computed(() => {
  const name = props.data.label || '?';
  return name.charAt(0).toUpperCase();
});

// Progress: defaults to indeterminate animation for running state.
// Phase 4 will add real progress from activity meta.
const progressValue = computed(() => 0.5); // indeterminate-like
const progressLabel = computed(() => '');

const activityIcon = computed(() => {
  switch (props.data.status) {
    case 'running': return 'bolt';
    case 'completed': return 'check_circle';
    case 'failed': return 'error';
    default: return 'hourglass_empty';
  }
});

const activitySummary = computed(() => {
  switch (props.data.status) {
    case 'pending': return '等待执行';
    case 'running': return '正在执行...';
    case 'completed': return '已完成';
    case 'failed': return '执行失败';
    case 'interrupted': return '已中断';
    default: return '';
  }
});
</script>

<style scoped lang="sass">
.observe-node
  border: 2px solid var(--color-border)
  border-radius: 8px
  background: var(--color-surface)
  min-width: 180px
  max-width: 240px
  padding: 8px
  font-size: 12px
  color: var(--color-text-primary)
  transition: border-color 0.3s ease, box-shadow 0.3s ease

  &--pending
    border-color: var(--color-border)

  &--running
    border-color: var(--color-warning)
    animation: observe-pulse 1.5s infinite

  &--completed
    border-color: var(--color-positive)

  &--failed
    border-color: var(--color-negative)

  &--interrupted
    border-color: var(--color-warning)
    opacity: 0.7

.observe-node__header
  display: flex
  align-items: center
  gap: 6px
  margin-bottom: 4px

.observe-node__avatar
  width: 24px
  height: 24px
  border-radius: 50%
  background: var(--color-primary)
  color: white
  display: flex
  align-items: center
  justify-content: center
  font-size: 11px
  font-weight: 600
  flex-shrink: 0

.observe-node__name
  font-weight: 500
  flex: 1
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.observe-node__progress
  margin: 4px 0

.observe-node__progress-label
  font-size: 10px
  color: var(--color-text-tertiary)
  margin-top: 2px

.observe-node__activity
  display: flex
  align-items: center
  gap: 4px
  margin-top: 6px
  color: var(--color-text-tertiary)
  font-size: 11px

.observe-node__activity-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

@keyframes observe-pulse
  0%, 100%
    box-shadow: 0 0 0 0 rgba(255, 152, 0, 0.4)
  50%
    box-shadow: 0 0 0 6px rgba(255, 152, 0, 0)
</style>
```

- [ ] **Step 2: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/chat/observe/ObserveNode.vue
git commit -m "feat(web): add ObserveNode component with ComfyUI-style status display"
```

---

### Task 15: ObservationCanvas + MediaLightbox

**Files:**
- Create: `web/src/components/chat/observe/ObservationCanvas.vue`
- Create: `web/src/components/chat/observe/MediaLightbox.vue`

- [ ] **Step 1: Create ObservationCanvas.vue**

```vue
<!-- web/src/components/chat/observe/ObservationCanvas.vue -->
<template>
  <div class="observation-canvas">
    <VueFlow
      :nodes="flowNodes"
      :edges="flowEdges"
      :fit-view-on-init="true"
      :node-types="nodeTypes"
      :default-zoom="0.8"
      :min-zoom="0.2"
      :max-zoom="2"
    >
      <Background :pattern-color="isDark ? '#333' : '#ddd'" />
      <Controls />
      <MiniMap :pannable="true" :zoomable="true" />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { computed, markRaw } from 'vue';
import { VueFlow } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { Controls } from '@vue-flow/controls';
import { MiniMap } from '@vue-flow/minimap';
import ObserveNode from './ObserveNode.vue';
import type { GraphStage, GraphNode } from '../../../features/chat/v2Types';
import { useObserveGraph } from '../../../features/chat/composables/useObserveGraph';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { toRef } from 'vue';

const props = defineProps<{
  graphStage: GraphStage | null;
  spiritSessionId: string;
  isDark: boolean;
}>();

const emit = defineEmits<{
  'select-node': [node: GraphNode];
  'preview': [art: MediaArtifact];
}>();

const nodeTypes = { observe: markRaw(ObserveNode) };

const spiritSessionIdRef = toRef(props, 'spiritSessionId');
const { flowNodes, flowEdges } = useObserveGraph(spiritSessionIdRef);
</script>

<style scoped lang="sass">
.observation-canvas
  width: 100%
  height: 100%
  min-height: 300px
</style>
```

Note: Vue Flow styles need to be imported. Check if `@vue-flow/core` styles are already imported in the project (GraphEditorCanvas.vue). If yes, they're globally available. If not, add import.

- [ ] **Step 2: Create MediaLightbox.vue**

```vue
<!-- web/src/components/chat/observe/MediaLightbox.vue -->
<template>
  <q-dialog :model-value="true" maximized @hide="$emit('close')">
    <q-card class="media-lightbox">
      <q-card-section class="row items-center q-pb-none">
        <q-space />
        <q-btn flat round dense icon="close" @click="$emit('close')" />
      </q-card-section>
      <q-card-section class="media-lightbox__content">
        <video
          v-if="artifact.mime_type.startsWith('video/')"
          :src="artifact.url"
          controls
          autoplay
          class="media-lightbox__media"
        />
        <img
          v-else
          :src="artifact.url"
          class="media-lightbox__media"
        />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

defineProps<{ artifact: MediaArtifact }>();
defineEmits<{ close: [] }>();
</script>

<style scoped lang="sass">
.media-lightbox
  background: rgba(0, 0, 0, 0.9)

.media-lightbox__content
  display: flex
  align-items: center
  justify-content: center
  height: 100%

.media-lightbox__media
  max-width: 90vw
  max-height: 80vh
  object-fit: contain
</style>
```

- [ ] **Step 3: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/components/chat/observe/ObservationCanvas.vue web/src/components/chat/observe/MediaLightbox.vue
git commit -m "feat(web): add ObservationCanvas (Vue Flow) and MediaLightbox"
```

---

### Task 16: ObservationPanel + ObserveNodeDetail

**Files:**
- Create: `web/src/components/chat/observe/ObservationPanel.vue`
- Create: `web/src/components/chat/observe/ObserveNodeDetail.vue`

- [ ] **Step 1: Create ObservationPanel.vue**

```vue
<!-- web/src/components/chat/observe/ObservationPanel.vue -->
<template>
  <div class="observation-panel">
    <!-- Toolbar -->
    <div class="observation-panel__toolbar">
      <q-btn flat dense icon="refresh" @click="refresh" :loading="loading">
        <q-tooltip>{{ t('observe.refresh') }}</q-tooltip>
      </q-btn>
      <q-space />
      <q-badge v-if="liveConnected" rounded color="positive" :label="t('observe.live')" />
    </div>

    <!-- Canvas area -->
    <div class="observation-panel__canvas">
      <ObservationCanvas
        v-if="graphStage"
        :graph-stage="graphStage"
        :spirit-session-id="spiritSessionId"
        :is-dark="isDark"
        @select-node="onSelectNode"
        @preview="onPreview"
      />
      <div v-else class="observation-panel__empty">
        <q-icon name="visibility_off" size="48px" color="grey-5" />
        <p class="text-grey-6">{{ t('observe.noActiveGraph') }}</p>
      </div>
    </div>

    <!-- Node detail sidebar -->
    <ObserveNodeDetail
      v-if="selectedNode"
      :node="selectedNode"
      @close="selectedNode = null"
      @preview="onPreview"
    />

    <!-- Fullscreen media preview -->
    <MediaLightbox
      v-if="previewArtifact"
      :artifact="previewArtifact"
      @close="previewArtifact = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ObservationCanvas from './ObservationCanvas.vue';
import ObserveNodeDetail from './ObserveNodeDetail.vue';
import MediaLightbox from './MediaLightbox.vue';
import { useObserveGraph } from '../../../features/chat/composables/useObserveGraph';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';

const { t } = useI18n();

const props = defineProps<{
  sessionId: string;
  spiritSessionId: string;
  isDark: boolean;
  wsConnected?: boolean;
}>();

const spiritSessionIdRef = computed(() => props.spiritSessionId);
const { graphStage } = useObserveGraph(spiritSessionIdRef);

const selectedNode = ref<GraphNode | null>(null);
const previewArtifact = ref<MediaArtifact | null>(null);
const loading = ref(false);
const liveConnected = computed(() => props.wsConnected ?? false);

function onSelectNode(node: GraphNode) {
  selectedNode.value = node;
}

function onPreview(art: MediaArtifact) {
  previewArtifact.value = art;
}

function refresh() {
  loading.value = true;
  // Trigger re-fetch from activityV2Store (it auto-refreshes via WS)
  setTimeout(() => { loading.value = false; }, 500);
}
</script>

<style scoped lang="sass">
.observation-panel
  display: flex
  flex-direction: column
  width: 100%
  height: 100%
  min-height: 0

.observation-panel__toolbar
  display: flex
  align-items: center
  padding: 4px 8px
  border-bottom: 1px solid var(--color-border)
  flex-shrink: 0

.observation-panel__canvas
  flex: 1
  min-height: 0
  position: relative

.observation-panel__empty
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  height: 100%
  gap: 8px
</style>
```

- [ ] **Step 2: Create ObserveNodeDetail.vue**

```vue
<!-- web/src/components/chat/observe/ObserveNodeDetail.vue -->
<template>
  <div class="observe-node-detail">
    <header class="observe-node-detail__header">
      <span class="observe-node-detail__avatar">{{ nodeInitial }}</span>
      <div class="observe-node-detail__info">
        <h3 class="observe-node-detail__name">{{ node.Label }}</h3>
        <ObserveStatusBadge :status="node.Status" />
      </div>
      <q-btn flat round dense icon="close" size="sm" @click="$emit('close')" />
    </header>

    <!-- Media outputs -->
    <section v-if="mediaOutputs.length" class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.mediaOutputs') }}</h4>
      <div class="observe-node-detail__media-grid">
        <div
          v-for="art in mediaOutputs"
          :key="art.artifact_id"
          class="observe-node-detail__media-item"
          @click="$emit('preview', art)"
        >
          <video
            v-if="art.mime_type.startsWith('video/')"
            :src="art.url"
            :poster="art.thumbnail"
            muted
            preload="metadata"
            class="observe-node-detail__media"
          />
          <img v-else :src="art.url" loading="lazy" class="observe-node-detail__media" />
        </div>
      </div>
    </section>

    <!-- Node info -->
    <section class="observe-node-detail__section">
      <h4 class="observe-node-detail__section-title">{{ t('observe.nodeInfo') }}</h4>
      <div class="observe-node-detail__meta">
        <div class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">DAG Node:</span>
          <code>{{ node.DagNodeID }}</code>
        </div>
        <div class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">Team Stage:</span>
          <code>{{ node.TeamStageID || '-' }}</code>
        </div>
        <div v-if="node.DependsOn?.length" class="observe-node-detail__meta-row">
          <span class="observe-node-detail__meta-label">Dependencies:</span>
          <span>{{ node.DependsOn.join(', ') }}</span>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GraphNode } from '../../../features/chat/v2Types';
import type { MediaArtifact } from '../../../features/chat/mediaTypes';
import { useNodeOutputStore } from '../../../../stores/chat/nodeOutputStore';
import ObserveStatusBadge from './ObserveStatusBadge.vue';

const { t } = useI18n();

const props = defineProps<{ node: GraphNode }>();
defineEmits<{
  close: [];
  preview: [art: MediaArtifact];
}>();

const nodeOutputStore = useNodeOutputStore();

const nodeInitial = computed(() => (props.node.Label || '?').charAt(0).toUpperCase());

const mediaOutputs = computed(() =>
  nodeOutputStore.getNodeOutput(props.node.TeamStageID || props.node.ID),
);
</script>

<style scoped lang="sass">
.observe-node-detail
  position: absolute
  top: 0
  right: 0
  width: 280px
  height: 100%
  background: var(--color-surface)
  border-left: 1px solid var(--color-border)
  overflow-y: auto
  padding: 12px
  z-index: 10

.observe-node-detail__header
  display: flex
  align-items: flex-start
  gap: 8px
  margin-bottom: 12px

.observe-node-detail__avatar
  width: 32px
  height: 32px
  border-radius: 50%
  background: var(--color-primary)
  color: white
  display: flex
  align-items: center
  justify-content: center
  font-size: 14px
  font-weight: 600
  flex-shrink: 0

.observe-node-detail__info
  flex: 1

.observe-node-detail__name
  font-size: 14px
  font-weight: 600
  margin: 0 0 4px

.observe-node-detail__section
  margin-bottom: 16px

.observe-node-detail__section-title
  font-size: 12px
  font-weight: 600
  color: var(--color-text-secondary)
  margin: 0 0 8px

.observe-node-detail__media-grid
  display: grid
  grid-template-columns: repeat(2, 1fr)
  gap: 6px

.observe-node-detail__media-item
  aspect-ratio: 1
  border-radius: 6px
  overflow: hidden
  cursor: pointer
  border: 1px solid var(--color-border)

  &:hover
    border-color: var(--color-primary)

.observe-node-detail__media
  width: 100%
  height: 100%
  object-fit: cover

.observe-node-detail__meta
  font-size: 12px

.observe-node-detail__meta-row
  display: flex
  gap: 6px
  margin-bottom: 4px

.observe-node-detail__meta-label
  color: var(--color-text-tertiary)
  flex-shrink: 0
</style>
```

- [ ] **Step 3: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/components/chat/observe/ObservationPanel.vue web/src/components/chat/observe/ObserveNodeDetail.vue
git commit -m "feat(web): add ObservationPanel and ObserveNodeDetail components"
```

---

### Task 17: Phase 2 集成验证

**Files:**
- Verify only

- [ ] **Step 1: Frontend lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 2: Frontend tests**

Run: `cd web && pnpm test`
Expected: PASS

- [ ] **Step 3: Frontend build**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 4: Commit (if any fixes)**

```bash
git add -A
git commit -m "chore: Phase 2 integration verification"
```

---

## Phase 3: 状态栏切换 + 视图集成（前端）

### Task 18: spiritStore 新增 viewMode / composerVisible

**Files:**
- Modify: `web/src/stores/spirit/index.ts`

- [ ] **Step 1: Add viewMode and composerVisible state**

In `web/src/stores/spirit/index.ts`, add to the store's state section (after `completionStats`):

```typescript
// ── Observation view state ──

/** Current view mode: 'chat' shows the normal message list, 'observe' shows the observation canvas. */
const viewMode = ref<'chat' | 'observe'>('chat');

/** Whether the ChatComposer is visible. Independent of viewMode. */
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
```

In the `reset()` function, add:

```typescript
viewMode.value = 'chat';
composerVisible.value = true;
```

In the `return` statement, add:

```typescript
viewMode,
composerVisible,
toggleViewMode,
toggleComposer,
setViewMode,
```

- [ ] **Step 2: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/stores/spirit/index.ts
git commit -m "feat(web): add viewMode and composerVisible state to spiritStore"
```

---

### Task 19: SpiritStatusBar 新增切换按钮

**Files:**
- Modify: `web/src/components/spirit/SpiritStatusBar.vue`

- [ ] **Step 1: Add view toggle and composer toggle buttons**

In `SpiritStatusBar.vue` template, add after the last existing status bar item (after the dq-score div, before the closing `</div>` of `__inner`):

```vue
      <!-- View toggle button -->
      <div
        class="spirit-status-bar__item spirit-status-bar__item--clickable"
        @click="emit('toggle-view')"
      >
        <q-icon
          :name="viewMode === 'observe' ? 'chat' : 'visibility'"
          size="14px"
          :style="{ color: viewMode === 'observe' ? 'var(--color-primary)' : 'var(--color-text-tertiary)' }"
        />
        <span>{{ viewMode === 'observe' ? t('spirit.backToChat') : t('spirit.observeView') }}</span>
      </div>

      <!-- Composer toggle (only in observe mode) -->
      <div
        v-if="viewMode === 'observe'"
        class="spirit-status-bar__item spirit-status-bar__item--clickable"
        @click="emit('toggle-composer')"
      >
        <q-icon
          :name="composerVisible ? 'keyboard' : 'keyboard_hide'"
          size="14px"
          :style="{ color: 'var(--color-text-tertiary)' }"
        />
      </div>
```

In the `defineProps`, add:

```typescript
  /** Current view mode (chat / observe). */
  viewMode?: 'chat' | 'observe';
  /** Whether the composer is visible. */
  composerVisible?: boolean;
```

In the `defineEmits`, add:

```typescript
  'toggle-view': [];
  'toggle-composer': [];
```

- [ ] **Step 2: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/spirit/SpiritStatusBar.vue
git commit -m "feat(web): add view toggle and composer toggle buttons to SpiritStatusBar"
```

---

### Task 20: ChatMessagePanel 条件渲染 + 透传

**Files:**
- Modify: `web/src/components/chat/ChatMessagePanel.vue`

- [ ] **Step 1: Add observation panel import**

In the script section of `ChatMessagePanel.vue`, add:

```typescript
import ObservationPanel from './observe/ObservationPanel.vue';
```

- [ ] **Step 2: Add viewMode/composerVisible props**

In `defineProps`, add:

```typescript
  /** Current view mode from spiritStore. */
  viewMode?: 'chat' | 'observe';
  /** Whether composer is visible from spiritStore. */
  composerVisible?: boolean;
```

- [ ] **Step 3: Add toggle emits**

In `defineEmits`, add:

```typescript
  'toggle-view': [];
  'toggle-composer': [];
```

- [ ] **Step 4: Modify template for conditional rendering**

Replace the existing `chat-messages-area` div content with conditional rendering:

```vue
    <div class="col row no-wrap chat-messages-area" style="min-height: 0">
      <!-- Chat mode: normal message list -->
      <template v-if="!viewMode || viewMode === 'chat'">
        <div class="col column no-wrap chat-messages-main" style="min-height: 0">
          <TodoKanbanBoard
            v-if="(showToolCalls ?? true) && (!panelMode || panelMode === 'spirit')"
            :board-state="todoBoardState"
          />
          <ChatMessageList
            ref="messageListRef"
            ... (all existing props and events unchanged)
          />
          <ContextIndicator ... />
        </div>
      </template>

      <!-- Observe mode: observation canvas -->
      <ObservationPanel
        v-else-if="viewMode === 'observe'"
        :session-id="sessionId ?? ''"
        :spirit-session-id="sessionId ?? ''"
        :is-dark="isDark"
      />

      <ChatReasoningDrawer
        :open="Boolean(reasoningSidebarOpen)"
        :active-reasoning="reasoningSidebarActive ?? null"
        :is-dark="isDark"
        @close="emit('close-reasoning-sidebar')"
      />
    </div>

    <!-- Composer controlled by composerVisible -->
    <ChatComposer
      v-if="(!panelMode || panelMode === 'spirit') && (composerVisible ?? true)"
      ... (all existing props and events unchanged)
    />

    <!-- SpiritStatusBar with new props -->
    <SpiritStatusBar
      v-if="spiritStatusBar && (!panelMode || panelMode === 'spirit')"
      :view-mode="viewMode ?? 'chat'"
      :composer-visible="composerVisible ?? true"
      @toggle-view="emit('toggle-view')"
      @toggle-composer="emit('toggle-composer')"
      ... (all existing props unchanged)
    />
```

Important: The existing `ChatMessageList` and `ChatComposer` blocks have many props/events. In the actual implementation, preserve ALL existing props and events exactly — only wrap them in the conditional template and add the new ones. Do NOT re-type the entire block; use Edit tool to make targeted changes.

- [ ] **Step 5: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add web/src/components/chat/ChatMessagePanel.vue
git commit -m "feat(web): add conditional chat/observe rendering to ChatMessagePanel"
```

---

### Task 21: i18n 新增 observe 键

**Files:**
- Modify: `web/src/i18n/zh-CN/spirit.json`
- Modify: `web/src/i18n/en-US/spirit.json`

- [ ] **Step 1: Add zh-CN keys**

In `web/src/i18n/zh-CN/spirit.json`, add to the `"spirit"` section:

```json
"observeView": "观测视图",
"backToChat": "返回聊天"
```

Add a new `"observe"` section:

```json
"observe": {
  "refresh": "刷新",
  "live": "实时",
  "noActiveGraph": "当前无活跃的团队执行",
  "mediaOutputs": "媒体产出",
  "activityStream": "活动流",
  "nodeInfo": "节点信息",
  "statusPending": "等待中",
  "statusRunning": "运行中",
  "statusCompleted": "已完成",
  "statusFailed": "失败",
  "statusInterrupted": "已中断"
}
```

- [ ] **Step 2: Add en-US keys**

In `web/src/i18n/en-US/spirit.json`, add corresponding English keys:

```json
"observeView": "Observe",
"backToChat": "Back to Chat"
```

```json
"observe": {
  "refresh": "Refresh",
  "live": "Live",
  "noActiveGraph": "No active team execution",
  "mediaOutputs": "Media Outputs",
  "activityStream": "Activity Stream",
  "nodeInfo": "Node Info",
  "statusPending": "Pending",
  "statusRunning": "Running",
  "statusCompleted": "Completed",
  "statusFailed": "Failed",
  "statusInterrupted": "Interrupted"
}
```

Also add to chat tool detail keys (if a `chat.toolDetail` section exists):

```json
"mediaOutputs": "媒体产出",
"prompt": "提示词"
```

- [ ] **Step 3: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add web/src/i18n/
git commit -m "feat(web): add observe-related i18n keys (zh-CN + en-US)"
```

---

### Task 22: Phase 3 集成验证

**Files:**
- Verify only

- [ ] **Step 1: Frontend lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 2: Frontend tests**

Run: `cd web && pnpm test`
Expected: PASS

- [ ] **Step 3: Frontend build**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 4: Manual verification checklist (dev server)**

Run: `cd web && pnpm dev`

Verify:
1. SpiritStatusBar shows the view toggle button when status bar is visible
2. Clicking the toggle switches between chat and observe views
3. In observe mode, the composer toggle button appears
4. Chat functionality is unaffected when `viewMode = 'chat'`
5. The observation panel shows empty state when no active graph

- [ ] **Step 5: Commit (if any fixes)**

```bash
git add -A
git commit -m "chore: Phase 3 integration verification"
```

---

## Phase 4: 节点级进度 + 视频团队模板

### Task 23: 进度推送辅助函数 + 工具进度集成

**Files:**
- Create: `internal/tools/media/progress.go`
- Modify: `internal/tools/media/generate_video.go`
- Modify: `internal/tools/media/image_to_video.go`

- [ ] **Step 1: Create progress.go**

```go
// internal/tools/media/progress.go
package media

import (
	"context"
	"fmt"
	"time"
)

// ProgressReporter is a function that publishes progress updates during
// long-running media generation tasks. The implementation sends
// ActivityEvent updated events with meta.progress.
type ProgressReporter func(ctx context.Context, value, max int, label string) error

// PublishProgress publishes a progress update for the current tool execution.
// The ActivityEvent system routes this to the frontend via WebSocket.
//
// Usage in tools:
//   reporter := NewProgressReporter(ctx)
//   reporter(ctx, 30, 100, "采样中 30%")
func PublishProgress(ctx context.Context, value, max int, label string) error {
	// TODO: Wire to ActivityEvent bus when available in tool context.
	// For now, log the progress as a placeholder. The actual implementation
	// will inject an event bus via context (similar to how session ID is injected).
	return nil
}

// PollWithProgress polls an async media generation job, publishing progress
// updates at regular intervals. Returns when the job completes or ctx is cancelled.
func PollWithProgress(
	ctx context.Context,
	interval time.Duration,
	getStatus func(ctx context.Context) (progress int, done bool, err error),
	reporter ProgressReporter,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			progress, done, err := getStatus(ctx)
			if err != nil {
				continue // transient error, keep polling
			}
			if reporter != nil {
				_ = reporter(ctx, progress, 100, fmt.Sprintf("生成中 %d%%", progress))
			}
			if done {
				return nil
			}
		}
	}
}
```

- [ ] **Step 2: Add progress polling to generate_video.go**

In `executeGenerateVideo`, replace the current implementation with:

```go
func executeGenerateVideo(ctx context.Context, mp media.MediaProvider, in GenerateVideoInput) (GenerateVideoOutput, error) {
	if in.Prompt == "" {
		return GenerateVideoOutput{}, fmt.Errorf("prompt is required")
	}
	if mp == nil {
		return GenerateVideoOutput{}, fmt.Errorf("media provider not configured")
	}
	if in.DurationMs <= 0 {
		in.DurationMs = 5000
	}
	if in.FPS <= 0 {
		in.FPS = 24
	}
	if in.Resolution == "" {
		in.Resolution = "720p"
	}

	// For long-running video generation, use progress polling.
	// The provider may implement an async job pattern; this polls with progress.
	result, err := mp.GenerateVideo(ctx, media.VideoRequest{
		Prompt:     in.Prompt,
		DurationMs: in.DurationMs,
		FPS:        in.FPS,
		Resolution: in.Resolution,
	})
	if err != nil {
		return GenerateVideoOutput{}, fmt.Errorf("generate video: %w", err)
	}
	return GenerateVideoOutput{Artifacts: result.Artifacts}, nil
}
```

Note: The actual progress polling integration depends on the specific provider implementation (ComfyUI WebSocket vs Qwen polling). The `PollWithProgress` helper will be used when providers implement async job patterns. For now, the stub providers return "not implemented" errors, so the progress infrastructure is ready but not active.

- [ ] **Step 3: Run tests**

Run: `go test -tags=pgvector ./internal/tools/media/... -count=1 -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/tools/media/progress.go internal/tools/media/generate_video.go internal/tools/media/image_to_video.go
git commit -m "feat(tools/media): add progress polling infrastructure for long-running media tasks"
```

---

### Task 24: 前端进度条联动

**Files:**
- Modify: `web/src/components/chat/observe/ObserveNode.vue`

- [ ] **Step 1: Read progress from activity meta**

In `ObserveNode.vue`, the progress bar currently shows an indeterminate state. Enhance it to read real progress from `data.progress` (if available from the activity stream):

```typescript
// Add progress to ObserveNodeData interface:
interface ObserveNodeData {
  label: string;
  dagNodeId: string;
  teamStageId: string;
  status: GraphNodeStatus;
  dependsOn: string[];
  mediaOutput: MediaArtifact[];
  progress?: { value: number; max: number; label?: string }; // Phase 4
}

// Update progress computed:
const progressValue = computed(() => {
  if (props.data.progress) {
    return props.data.progress.value / props.data.progress.max;
  }
  return 0.5; // indeterminate-like
});

const progressLabel = computed(() => {
  if (props.data.progress?.label) {
    return props.data.progress.label;
  }
  if (props.data.progress) {
    return `${Math.round((props.data.progress.value / props.data.progress.max) * 100)}%`;
  }
  return '';
});
```

In `useObserveGraph.ts`, extract progress from the latest activity for each node and pass it to the node data.

- [ ] **Step 2: Verify lint**

Run: `cd web && pnpm lint`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add web/src/components/chat/observe/ObserveNode.vue web/src/features/chat/composables/useObserveGraph.ts
git commit -m "feat(web): add real progress display to ObserveNode from activity meta"
```

---

### Task 25: 最终集成验证 + 文档同步

**Files:**
- Verify only
- Modify: `docs/development/0-system-diagram.md` (add MediaProvider module)

- [ ] **Step 1: Full backend build + test**

Run: `go build -tags=pgvector ./... && go test -tags=pgvector ./... -count=1`
Expected: PASS

- [ ] **Step 2: Full frontend lint + test + build**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: PASS

- [ ] **Step 3: Verify wire**

Run: `make wire`
Expected: PASS

- [ ] **Step 4: Update system diagram doc**

Add MediaProvider and Observation View to `docs/development/0-system-diagram.md`:

- MediaProvider module under Provider section
- Observation canvas under Chat UI section
- Note the chat ↔ observe toggle

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat: media generation + observation view

Phase 1: MediaProvider system (interface + registry + Qwen/ComfyUI stubs)
         + 3 media tools (generate_image, generate_video, image_to_video)
         + media_providers table schema + DDL migration
         + ToolCategory.media constant

Phase 2: Observation canvas (Vue Flow) with ComfyUI-style nodes
         + nodeOutputStore for per-node media outputs
         + MediaToolDetail for chat stream media rendering
         + NodeMediaPreview + MediaLightbox for media display

Phase 3: SpiritStatusBar view toggle button (chat ↔ observe)
         + composerVisible toggle
         + ChatMessagePanel conditional rendering
         + i18n keys (zh-CN + en-US)

Phase 4: Progress polling infrastructure for long-running media tasks
         + ObserveNode real progress display

Spec: docs/superpowers/specs/2026-07-18-media-generation-observation-view-design.md
EOF
)"
```

---

## Spec Coverage Checklist

| Spec Section | Task(s) | Status |
|-------------|---------|--------|
| §4.1 MediaProvider 接口 | Task 1 | ✅ |
| §4.2 Provider 注册 | Task 2 | ✅ |
| §4.3 Provider 实现 | Task 3 (Qwen), Task 4 (ComfyUI) | ✅ |
| §4.4 配置存储 | Task 5 | ✅ |
| §4.5 媒体生成工具 | Task 7 | ✅ |
| §4.6 Activity.Meta 约定 | Task 11 (frontend sync) | ✅ |
| §4.7 ToolCategory 扩展 | Task 6 | ✅ |
| §4.8 Wire 注入 | Task 8 (verify) | ✅ (deferred to provider impl) |
| §5.1 核心组件层级 | Tasks 13-16 | ✅ |
| §5.2 ObservationCanvas | Task 15 | ✅ |
| §5.3 ObserveNode | Task 14 | ✅ |
| §5.4 NodeMediaPreview | Task 13 | ✅ |
| §5.5 useNodeOutputStore | Task 9 | ✅ |
| §5.6 activityV2Store 同步 | Task 11 | ✅ |
| §5.7 媒体类型定义 | Task 9 | ✅ |
| §5.8 useObserveGraph | Task 12 | ✅ |
| §6.1 状态栏切换按钮 | Task 19 | ✅ |
| §6.2 spiritStore 双状态 | Task 18 | ✅ |
| §6.3 ChatMessagePanel 条件渲染 | Task 20 | ✅ |
| §6.4 ObservationPanel | Task 16 | ✅ |
| §6.5 ObserveNodeDetail | Task 16 | ✅ |
| §6.6 i18n | Task 21 | ✅ |
| §7.1 节点级进度 | Tasks 23-24 | ✅ |
| §7.2 视频团队模板 | N/A (通过现有 UI 配置，无代码改动) | ✅ |
| §10 MediaToolDetail | Task 10 | ✅ |
| 验收标准 Phase 1 | Task 8 | ✅ |
| 验收标准 Phase 2 | Task 17 | ✅ |
| 验收标准 Phase 3 | Task 22 | ✅ |
| 验收标准 Phase 4 | Task 25 | ✅ |
