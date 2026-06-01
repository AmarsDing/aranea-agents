# 多模态Agent

## 一、需求文档

### 1.1 背景

当前平台 Agent 以文本对话为主，框架 `model.ContentPart` 已支持 Text/Image/Audio/File 四种内容类型，但前端和后端尚未完整打通多模态输入输出链路。多模态 Agent 旨在让 Agent 理解和生成语音、图像、视频内容，覆盖语音助手、视觉分析、文档理解等场景。

行业参考：
- **OpenAI GPT-4V**：支持图像输入理解 + 图像生成（DALL-E），Vision API 通过 `image_url` 传递图片
- **Google Gemini**：原生多模态模型，支持 Text + Image + Audio + Video 输入，通过 `Part` 统一表示
- **Anthropic Claude**：支持图像输入（Vision），通过 `content` 数组的 `image` 类型传递

### 1.2 目标

1. 打通多模态输入链路：用户可上传图片/音频/文件，Agent 可理解多模态内容
2. 打通多模态输出链路：Agent 可生成图片（DALL-E / Stable Diffusion）、语音（TTS）
3. 前端支持多模态消息渲染：图片预览、音频播放、文件下载
4. 多模态工具集成：图片分析工具、语音转文字工具、文字转语音工具

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | 图片输入 | P0 | 用户上传图片，Agent 通过 Vision API 理解图片内容 |
| F2 | 文件输入 | P0 | 用户上传 PDF/DOCX 等文件，Agent 提取文本内容 |
| F3 | 图片生成 | P1 | Agent 调用 DALL-E / Stable Diffusion 生成图片 |
| F4 | 语音输入 | P1 | 用户上传音频，Agent 通过 STT 转文字后理解 |
| F5 | 语音输出 | P1 | Agent 回复可通过 TTS 转语音播放 |
| F6 | 多模态消息渲染 | P0 | 前端渲染图片/音频/文件类型的 ContentPart |
| F7 | 多模态工具注册 | P1 | 图片分析/语音转写/TTS 等工具在 ToolRegistry 注册 |
| F8 | 视频输入 | P2 | 用户上传视频，Agent 提取关键帧后理解 |
| F9 | 多模态会话记忆 | P2 | 多模态消息持久化到 Session，支持回放 |
| F10 | 多模态 Graph 节点 | P2 | Graph 工作流中支持多模态处理节点 |

### 1.4 非功能需求

| # | 需求 | 指标 |
|---|------|------|
| NFR1 | 图片上传大小 | 单张 ≤ 20MB |
| NFR2 | 音频上传大小 | 单个 ≤ 50MB |
| NFR3 | 文件上传大小 | 单个 ≤ 100MB |
| NFR4 | 图片生成延迟 | DALL-E 调用 < 30s |
| NFR5 | 语音转写延迟 | 1 分钟音频 < 10s |
| NFR6 | 多模态消息持久化 | Session 压缩后存储 |

### 1.5 验收标准

1. 用户可上传图片并在对话中获得 Agent 的图片理解回复
2. 用户可上传 PDF 文件，Agent 可提取并回答文件内容相关问题
3. 前端正确渲染图片/音频/文件类型的 ContentPart
4. 图片生成工具可调用并返回图片 URL
5. 语音输入经 STT 转文字后 Agent 正常响应

---

## 二、设计文档

### 2.1 行业参考

**OpenAI GPT-4V**：
- 输入：`content` 数组支持 `text` + `image_url` 类型
- `image_url` 可传 URL 或 base64 data URI
- 输出：文本回复 + DALL-E 图片生成（通过 Tool 调用）

**Google Gemini**：
- 输入：`Part` 统一表示 Text / InlineData / FileData / FunctionCall
- `InlineData` 支持图片/音频/视频的 base64 编码
- `FileData` 支持通过 File URI 引用已上传文件

**框架可复用组件**：

| 框架组件 | 路径 | 复用方式 |
|----------|------|----------|
| `model.ContentPart` | `pkg/trpc-agent-go/model/request.go` | 多模态消息统一表示 |
| `model.ContentType` | `pkg/trpc-agent-go/model/request.go` | Text/Image/Audio/File 四种类型 |
| `model.Image` | `pkg/trpc-agent-go/model/request.go` | 图片数据（URL / Data / Detail / Format） |
| `model.Audio` | `pkg/trpc-agent-go/model/request.go` | 音频数据（Data / Format） |
| `model.File` | `pkg/trpc-agent-go/model/request.go` | 文件数据（Name / URL / Data / FileID / MimeType） |
| `Message.AddImageURL` | `pkg/trpc-agent-go/model/request.go` | 添加图片 URL |
| `Message.AddImageData` | `pkg/trpc-agent-go/model/request.go` | 添加图片二进制数据 |
| `Message.AddAudioData` | `pkg/trpc-agent-go/model/request.go` | 添加音频数据 |
| `Message.AddFileData` | `pkg/trpc-agent-go/model/request.go` | 添加文件数据 |
| `Message.AddFileURL` | `pkg/trpc-agent-go/model/request.go` | 添加文件 URL |
| `openai.New` | `pkg/trpc-agent-go/model/openai/` | OpenAI Vision API 适配 |
| `gemini.New` | `pkg/trpc-agent-go/model/gemini/` | Gemini 多模态 API 适配 |

**框架 `ContentPart` 结构**：

```go
type ContentType string

const (
    ContentTypeText  ContentType = "text"
    ContentTypeImage ContentType = "image"
    ContentTypeAudio ContentType = "audio"
    ContentTypeFile  ContentType = "file"
)

type ContentPart struct {
    Type  ContentType `json:"type"`
    Text  *string     `json:"text,omitempty"`
    Image *Image      `json:"image,omitempty"`
    Audio *Audio      `json:"audio,omitempty"`
    File  *File       `json:"file,omitempty"`
}

type Image struct {
    URL    string `json:"url"`
    Data   []byte `json:"data"`
    Detail string `json:"detail,omitempty"`
    Format string `json:"format,omitempty"`
}

type Audio struct {
    Data   []byte `json:"data"`
    Format string `json:"format"`
}

type File struct {
    Name     string `json:"filename"`
    URL      string `json:"url,omitempty"`
    Data     []byte `json:"data"`
    FileID   string `json:"file_id"`
    MimeType string `json:"format,omitempty"`
}
```

### 2.2 当前项目现状

| 现有代码 | 路径 | 说明 |
|----------|------|------|
| `internal/provider/` | Provider 集成 | 已有 Gemini / OpenAI Provider，支持多模态模型 |
| `internal/tools/` | 工具注册 | 有图片附件检测逻辑 |
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | Agent 构建，已支持 `WithModel` |
| `ChatService` | `internal/service/chat_native.go` | Chat Turn 处理，当前仅文本 |
| 前端消息渲染 | `components/chat/` | 当前仅渲染文本消息 |

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/kratos/chat/v1/chat.proto              ← 扩展：多模态消息字段
        ↓
internal/service/chat_native.go            ← 扩展：多模态 Turn 输入解析
        ↓
internal/biz/multimodal.go                 ← 新增：多模态领域模型 + 端口
internal/biz/multimodal_usecase.go         ← 新增：多模态处理用例
        ↓
internal/data/multimodal_repo.go           ← 新增：多模态资产持久化
        ↓
internal/multimodal/                       ← 新增：多模态处理运行时
  ├── processor/                           ← 多模态内容处理器
  │   ├── image_processor.go               ← 图片处理（压缩/格式转换/OCR）
  │   ├── audio_processor.go               ← 音频处理（STT/TTS）
  │   ├── file_processor.go                ← 文件处理（PDF/DOCX 提取）
  │   └── video_processor.go               ← 视频处理（关键帧提取）
  ├── generator/                           ← 多模态内容生成器
  │   ├── image_generator.go               ← 图片生成（DALL-E / SD）
  │   └── speech_generator.go              ← 语音生成（TTS）
  └── storage/                             ← 多模态资产存储
      └── asset_store.go                   ← 统一资产存储（本地/S3）
```

#### 新增/修改的文件清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 修改 | `api/kratos/chat/v1/chat.proto` | 扩展消息字段支持 ContentPart |
| 新增 | `internal/biz/multimodal.go` | 多模态领域模型 + 端口接口 |
| 新增 | `internal/biz/multimodal_usecase.go` | 多模态处理用例 |
| 新增 | `internal/data/multimodal_repo.go` | 多模态资产 Ent 持久化 |
| 新增 | `internal/multimodal/processor/image_processor.go` | 图片处理器 |
| 新增 | `internal/multimodal/processor/audio_processor.go` | 音频处理器 |
| 新增 | `internal/multimodal/processor/file_processor.go` | 文件处理器 |
| 新增 | `internal/multimodal/processor/video_processor.go` | 视频处理器 |
| 新增 | `internal/multimodal/generator/image_generator.go` | 图片生成器 |
| 新增 | `internal/multimodal/generator/speech_generator.go` | 语音生成器 |
| 新增 | `internal/multimodal/storage/asset_store.go` | 资产存储 |
| 新增 | `internal/tools/multimodal/image_analysis.go` | 图片分析工具 |
| 新增 | `internal/tools/multimodal/speech_to_text.go` | 语音转文字工具 |
| 新增 | `internal/tools/multimodal/text_to_speech.go` | 文字转语音工具 |
| 新增 | `internal/tools/multimodal/image_generation.go` | 图片生成工具 |
| 修改 | `internal/service/chat_native.go` | 扩展多模态 Turn 输入 |
| 修改 | `internal/agent/trpc_build.go` | 多模态工具装配 |
| 修改 | `cmd/admin/wire.go` | Wire 注入 |

#### 接口设计

**多模态领域模型**（`internal/biz/multimodal.go`）：

```go
type MultimodalAsset struct {
    ID        string
    SessionID string
    Type      string
    MimeType  string
    URL       string
    FileID    string
    Metadata  map[string]string
    CreatedAt string
}

type MultimodalProcessor interface {
    ContentType() string
    Process(ctx context.Context, asset MultimodalAsset) (string, error)
}

type MultimodalGenerator interface {
    OutputType() string
    Generate(ctx context.Context, prompt string, opts map[string]any) (*MultimodalAsset, error)
}

type AssetStore interface {
    Save(ctx context.Context, asset MultimodalAsset) (MultimodalAsset, error)
    Get(ctx context.Context, id string) (MultimodalAsset, error)
    GetBySession(ctx context.Context, sessionID string) ([]MultimodalAsset, error)
    Delete(ctx context.Context, id string) error
}

type MultimodalAssetReader interface {
    GetAsset(ctx context.Context, id string) (MultimodalAsset, error)
    ListAssetsBySession(ctx context.Context, sessionID string) ([]MultimodalAsset, error)
}

type MultimodalAssetWriter interface {
    SaveAsset(ctx context.Context, asset MultimodalAsset) (MultimodalAsset, error)
    DeleteAsset(ctx context.Context, id string) error
}

type MultimodalAssetRepository interface {
    MultimodalAssetReader
    MultimodalAssetWriter
}
```

**多模态用例**（`internal/biz/multimodal_usecase.go`）：

```go
type MultimodalUsecase struct {
    repo       MultimodalAssetRepository
    processors map[string]MultimodalProcessor
    generators map[string]MultimodalGenerator
    store      AssetStore
}

func NewMultimodalUsecase(
    repo MultimodalAssetRepository,
    processors []MultimodalProcessor,
    generators []MultimodalGenerator,
    store AssetStore,
) *MultimodalUsecase

func (u *MultimodalUsecase) ProcessAsset(ctx context.Context, asset MultimodalAsset) (string, error)
func (u *MultimodalUsecase) GenerateContent(ctx context.Context, outputType string, prompt string, opts map[string]any) (*MultimodalAsset, error)
func (u *MultimodalUsecase) SaveAsset(ctx context.Context, asset MultimodalAsset) (MultimodalAsset, error)
func (u *MultimodalUsecase) GetAsset(ctx context.Context, id string) (MultimodalAsset, error)
```

**多模态工具**（`internal/tools/multimodal/`）：

```go
func NewImageAnalysisTool(uc *biz.MultimodalUsecase) tool.Tool {
    return function.NewFunctionTool(
        func(ctx context.Context, input ImageAnalysisInput) (*ImageAnalysisOutput, error) {
            result, err := uc.ProcessAsset(ctx, biz.MultimodalAsset{Type: "image", URL: input.ImageURL})
            return &ImageAnalysisOutput{Description: result}, err
        },
        function.WithName("image_analysis"),
        function.WithDescription("分析图片内容并返回描述"),
    )
}

func NewImageGenerationTool(uc *biz.MultimodalUsecase) tool.Tool {
    return function.NewFunctionTool(
        func(ctx context.Context, input ImageGenerationInput) (*ImageGenerationOutput, error) {
            asset, err := uc.GenerateContent(ctx, "image", input.Prompt, nil)
            return &ImageGenerationOutput{ImageURL: asset.URL}, err
        },
        function.WithName("image_generation"),
        function.WithDescription("根据文字描述生成图片"),
    )
}

func NewSpeechToTextTool(uc *biz.MultimodalUsecase) tool.Tool
func NewTextToSpeechTool(uc *biz.MultimodalUsecase) tool.Tool
```

#### 数据流图

```
用户上传图片
    │
    ▼
ChatPage → POST /v1/chat/turn (multipart)
    │
    ▼
ChatService → 解析 ContentPart{Type: "image", Image: {Data: ...}}
    │
    ▼
MultimodalUsecase.ProcessAsset()
    ├── ImageProcessor.Process() → 压缩/格式转换
    ├── AssetStore.Save() → 存储图片资产
    └── 返回资产 URL
    │
    ▼
BuildTRPCLLMAgent → Message.AddImageData(data, detail, format)
    │
    ▼
LLM Model (OpenAI Vision / Gemini) → 理解图片内容
    │
    ▼
Agent 回复包含图片理解结果

Agent 生成图片
    │
    ▼
LLM 调用 image_generation 工具
    │
    ▼
ImageGenerationTool → MultimodalUsecase.GenerateContent("image", prompt, opts)
    │
    ▼
ImageGenerator.Generate() → 调用 DALL-E API
    │
    ▼
AssetStore.Save() → 存储生成图片
    │
    ▼
返回 ImageURL → Agent 回复包含图片
```

### 2.4 与框架的集成方式

| 集成点 | 框架组件 | 集成方式 |
|--------|----------|----------|
| 多模态消息构建 | `model.Message.AddImageData` / `AddAudioData` / `AddFileData` | Chat Service 解析前端上传后调用这些方法构建 `ContentPart` |
| 多模态模型调用 | `openai.New` / `gemini.New` | Provider 层已支持，Agent 通过 `WithModel` 选择多模态模型 |
| 多模态工具注册 | `function.NewFunctionTool[I, O]` | 图片分析/生成/STT/TTS 工具在 `internal/tools/multimodal/` 注册 |
| 工具装配 | `internal/agent/trpc_build.go` → `buildToolDeps` | 多模态工具加入 Agent ToolSet |
| 事件发射 | `agent.EmitEvent(ctx, inv, ch, evt)` | 多模态资产生成后发射事件通知前端 |
| 资产存储 | `trpcartifact` 框架 | 可选使用框架 `artifact` 包管理多模态资产 |

### 2.5 错误处理

| 场景 | 错误码 | 处理 |
|------|--------|------|
| 图片格式不支持 | `BadRequest("MULTIMODAL", "unsupported image format")` | 返回 400 |
| 文件大小超限 | `BadRequest("MULTIMODAL", "file size exceeds limit")` | 返回 400 |
| 图片生成 API 失败 | `InternalServer("MULTIMODAL", "image generation failed")` | 返回 500 |
| STT 转写失败 | `InternalServer("MULTIMODAL", "speech to text failed")` | 降级为文本提示 |
| 资产存储失败 | `InternalServer("MULTIMODAL", "asset storage failed")` | 返回 500 |
| 多模态模型不可用 | `InternalServer("MULTIMODAL", "multimodal model unavailable")` | 降级为纯文本 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|------------|
| T1 | 定义 `MultimodalAsset` 领域模型 + 端口接口 | 无 | S |
| T2 | 实现 `AssetStore`（本地存储 + S3 可选） | T1 | M |
| T3 | 实现 `ImageProcessor`（压缩/格式转换） | T1 | M |
| T4 | 实现 `FileProcessor`（PDF/DOCX 文本提取） | T1 | M |
| T5 | 实现 `AudioProcessor`（STT 集成） | T1 | L |
| T6 | 实现 `ImageGenerator`（DALL-E API 集成） | T1 | L |
| T7 | 实现 `SpeechGenerator`（TTS 集成） | T1 | M |
| T8 | 实现多模态工具（4 个 FunctionTool） | T1, T3, T5, T6, T7 | M |
| T9 | 扩展 Chat Proto 支持多模态消息 | 无 | S |
| T10 | 扩展 Chat Service 解析多模态输入 | T9, T3, T4 | L |
| T11 | 实现 `MultimodalUsecase` | T1, T2, T3, T5, T6 | M |
| T12 | 实现 `MultimodalAssetRepository`（Ent） | T1 | M |
| T13 | Wire 注入 + 集成测试 | T8, T10, T11, T12 | S |
| T14 | 前端多模态消息渲染 | T9 | L |
| T15 | 前端文件上传组件 | T9 | M |
| T16 | 端到端验证 | T13, T14, T15 | M |

### 3.2 开发顺序

```
Phase 1（核心模型）：T1 → T2 → T12 → T11
Phase 2（处理器）：T3 → T4 → T5（可并行）
Phase 3（生成器）：T6 → T7（可并行）
Phase 4（工具+服务）：T8 → T9 → T10 → T13
Phase 5（前端）：T14 → T15
Phase 6（验证）：T16
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 图片上传理解 | 上传图片 + 发送"描述这张图片" | Agent 返回图片内容描述 |
| PDF 文件理解 | 上传 PDF + 发送相关问题 | Agent 基于文件内容回答 |
| 图片生成 | 发送"生成一张猫的图片" | Agent 调用 image_generation 工具返回图片 URL |
| 语音输入 | 上传音频文件 | Agent 理解音频内容并回复 |
| 前端渲染 | 发送多模态消息 | 图片/音频/文件正确渲染 |
| API 契约 | `make api && go build ./...` | 编译通过 |
| Wire 注入 | `make wire && go build ./cmd/admin` | 编译通过 |
