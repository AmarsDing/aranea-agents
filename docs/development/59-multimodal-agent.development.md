# M59: 多模态 Agent — 开发计划

> 版本：v1.0（2026-07-28）
> 需求见 [59-multimodal-agent.md](./59-multimodal-agent.md)；设计见 [59-multimodal-agent.design.md](./59-multimodal-agent.design.md)。

---

## 1. 模块定位

多模态 Agent 是 Chat 链路的多模态能力集合（非新 Agent 类型），建立在 27-artifact 资产底座之上。本模块只做**增量补齐**：图片治理、文档提取接入、STT、回放持久化，其余为既有功能的验收。

## 2. 代码锚点

| 层 | 路径 | 职责 |
|----|------|------|
| Proto | [chat.proto](../../api/kratos/chat/v1/chat.proto) | `AttachmentRef`（已有） |
| Service | [chat_attachments.go](../../internal/service/chat_attachments.go) | refs 解析、能力门控（已有） |
| Agent | [attachments.go](../../internal/agent/attachments.go) | `BuildUserMessageFromArtifacts`，图片→ContentPart（已有） |
| Biz | [internal/biz/artifact/](../../internal/biz/artifact/artifact.go) | 资产底座（已有，不改动） |
| Knowledge | [extractor.go](../../internal/knowledge/extractor.go)、[vision_extractor.go](../../internal/knowledge/vision_extractor.go) | 文档/图片提取（复用） |
| Media | [internal/provider/media/provider.go](../../internal/provider/media/provider.go)、[internal/tools/media/](../../internal/tools/media/generate_image.go) | 媒体生成（已有） |
| 前端 | web/src/components/chat/ChatMessageAttachments.vue | 附件渲染（已有，增量） |
| 【新增】 | internal/multimodal/imageguard/guard.go | 图片治理 |
| 【新增】 | internal/multimodal/stt/transcriber.go | STT 抽象 + OpenAI 兼容实现 |
| 【新增】 | internal/multimodal/docextract/bridge.go | knowledge Extractor → Chat 适配 |

## 3. 现状评估（2026-07-28 评审，含证据）

| 能力 | 状态 | 证据 |
|------|------|------|
| 附件上传/存储/预览 | ✅ | 27-artifact 全链路 |
| 图片输入 → Vision 模型 | ✅ 链路完整，待端到端验收 | [attachments.go:74-84](../../internal/agent/attachments.go) image 分支 |
| 模型能力闸前拦截 | ✅ | [chat_attachments.go:56-64](../../internal/service/chat_attachments.go) |
| 文本/超大附件 blob 治理 | ✅ | attachments.go gate 路径 |
| 图片/视频生成工具 | ✅ | tools/media 三工具 + provider/media |
| TTS 语音产出 | ✅ 待端到端验收 | 63-tts |
| 知识库图片理解 | ✅（知识场景） | knowledge VisionExtractor |
| 图片规格治理（压缩/转码） | ❌ | 无 |
| OOXML/PDF 提取接入 Chat | ❌ | 当前仅文本化/blob |
| 音频输入（STT） | ❌ | 无 |
| 历史消息附件回放 | 🟡 疑似部分存在（refs 已并入 turn options），待验收 | [chat_attachments.go:28-54](../../internal/service/chat_attachments.go) |
| 前端分类型渲染（audio/video 卡片） | 🟡 待核查 | ChatMessageAttachments |
| 视频输入 | ❌（P2 暂缓） | — |

## 4. Phase 划分与任务清单

> 替代 phase5 原 T1–T16（差异说明见设计文档 §五）。

### Phase 0：现状验收（先行，决定后续范围）

| 任务ID | 描述 | 状态 |
|--------|------|------|
| M0-1 | 端到端验收：上传图片 → Vision 理解 → 回复（含能力闸拒绝路径） | ⏳ |
| M0-2 | 端到端验收：generate_image/tts 产物在消息流中渲染与下载 | ⏳ |
| M0-3 | 回放验收：重开会话，确认历史消息附件引用是否完整恢复；输出结论（决定 M3 是否立项） | ⏳ |
| M0-4 | 前端渲染核查：audio/video/pdf 卡片分派与 i18n 键 | ⏳ |

### Phase 1：输入治理与文档理解（P0）

| 任务ID | 描述 | 依赖 | 状态 |
|--------|------|------|------|
| M1-1 | imageguard.Guard（压缩/转码/gif 首帧）+ 接入 attachments.go image 分支 + 单测 | 无 | ⏳ |
| M1-2 | docextract.Bridge 复用 knowledge.ExtractorRegistry，接入非图片附件分支 + 单测 | 无 | ⏳ |
| M1-3 | 上传白名单与单 Turn 数量/体积上限（闸前）+ 单测 | 无 | ⏳ |

### Phase 2：音频输入（P1）

| 任务ID | 描述 | 依赖 | 状态 |
|--------|------|------|------|
| M2-1 | stt.Transcriber 接口 + OpenAI 兼容实现（whisper 端点，30s 超时，未配置降级）+ 单测 | 无 | ⏳ |
| M2-2 | attachments.go 新增 audio/* 分支（转写文本注入）+ 配置装配 + 单测 | M2-1 | ⏳ |

### Phase 3：回放补齐（P0，视 M0-3 结论）

| 任务ID | 描述 | 依赖 | 状态 |
|--------|------|------|------|
| M3-1 | 历史加载路径透传附件 refs（优先复用 turn options；不足则幂等 DDL 加列） | M0-3 | ⏳ |
| M3-2 | 前端消息气泡按 refs 渲染 + artifact 已删除占位卡 | M3-1 | ⏳ |

### Phase 4：前端渲染与 i18n（P0/P1）

| 任务ID | 描述 | 依赖 | 状态 |
|--------|------|------|------|
| M4-1 | ChatMessageAttachments 按 PreviewKind 分派渲染（audio 播放条 / video poster / pdf 卡） | M0-4 | ⏳ |
| M4-2 | `multimodal.*` i18n 键（zh-CN/en-US）：类型名/错误/降级提示 | 无 | ⏳ |

### Phase 5：视频输入（P2，暂缓）

| 任务ID | 描述 | 依赖 | 状态 |
|--------|------|------|------|
| M5-1 | 视频关键帧提取 + 音轨 STT → 多图+文本注入 | Phase 2 | 📋 |

## 5. 验收标准

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 图片理解 | 上传图片问「描述这张图」 | Agent 正确描述；无视觉能力模型被闸前拒绝 |
| 图片治理 | 上传 8MB/5000px 图片 | 压缩后进入模型，回复正常 |
| 文档理解 | 上传 docx/pdf 提问 | 回复基于提取内容；提取失败降级不阻断 |
| 音频理解 | 上传 30s 录音提问 | 转写文本注入并正确回答；STT 未配置时降级提示 |
| 生成验收 | 「画一只猫」「朗读回复」 | 媒体卡片渲染、可播放/下载 |
| 回放 | 重开会话 | 历史附件完整可预览 |
| 回归 | `go build ./...` + `go test ./internal/agent/... ./internal/service/...` + `cd web && pnpm test` | 全绿 |

## 6. 改动文件清单（预估）

| 操作 | 文件 |
|------|------|
| 新增 | internal/multimodal/imageguard/guard.go + guard_test.go |
| 新增 | internal/multimodal/stt/transcriber.go + openai_transcriber.go + 测试 |
| 新增 | internal/multimodal/docextract/bridge.go + bridge_test.go |
| 修改 | internal/agent/attachments.go（治理/文档/音频分支） |
| 修改 | internal/service/chat_attachments.go（白名单/限额） |
| 修改 | cmd/admin 配置装配（STT 配置） |
| 修改 | web/src/components/chat/ChatMessageAttachments.vue + i18n locales |
| 可能 | DDL 迁移（回放加列，视 M0-3 结论） |
