# 12 模型与定价

## 功能

统一管理 12+ Provider 与全球模型目录：模型规格自动同步、六维定价、能力标记——为[成本管控](09-cost.md)提供精准计量基础。

## 原理

### models.dev 同步

- 从 [models.dev](https://models.dev) 拉取全球 AI 模型规格，作为**模型参数和定价的外部真相源**；
- **定时同步**：每小时自动同步；
- **Provider 迁移**：旧 provider_code 自动迁移到 models.dev id，支持断点续传；
- **Runtime Overlay**：models.dev provider id → trpc-agent-go 运行时映射。

### 定价优先级

```text
manual（100）> model-inspect（50）> models.dev-sync（10）
```

低优先级来源永远不能覆盖高优先级——手动设置的定价不会被自动同步冲掉。

### 六维定价与双轨展示

| 维度 | 说明 |
|------|------|
| Input / Output | 输入/输出 token |
| CacheRead / CacheWrite | 缓存读/写 |
| Reasoning | 推理 token |
| Embedding | 嵌入 token |

- **MicroPricing**：内部精确计算（微美元精度）；
- **CostUSDPer1M**：对外展示（每百万 token 美元）。

### 能力标记

`text / vision / audio / file / tool_call / cache / thinking / text_only`——系统据此自动选择合适模型（如仅 vision 模型用于图像理解、仅 tool_call 模型进入编排）。

### 支持的 Provider（12+）

OpenAI、Anthropic、Gemini、DeepSeek、Qwen（阿里通义）、Moonshot（月之暗面）、OpenRouter、ZhipuAI（智谱）、Amazon Bedrock、Ollama、Hunyuan（腾讯混元）、HuggingFace。

## 设计要点

- **embedding 降级**：未配置 Embedding 端点时自动降级为关键词混合召回，契约保持可用（记忆挑战赛入口同样遵循）；
- **渠道（Channel）抽象**：模型访问经渠道统一管理，支持多端点与密钥轮换；
- **模型路由联动**：model_router 插件按规则在运行时切换模型（见 [11 安全与插件](11-security.md)）。

## 界面配置

- **模型页**：浏览模型目录（能力标记、六维定价、上下文窗口），手动覆盖定价（manual 优先级）；
- **渠道页**：配置 Provider 端点与密钥（加密存储 + masked preview），连通性测试；
- Agent 编辑页为每个 Agent 选择模型（如 `deepseek/deepseek-v4-flash`）。

## 深入阅读

- [65 模块交叉引用 · provider / pricing 章节](../../docs/development/65-module-cross-reference-full.md)
- [09 成本管控](09-cost.md)
