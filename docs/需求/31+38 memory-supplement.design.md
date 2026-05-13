# Memory 补充设计 — 实现设计文档

> 对应需求：`31 memery.md` / `38 memory.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

> 注：核心五层记忆设计已覆盖在 `12-16 memory.design.md`，本文档补充 31/38 中额外描述的能力。

---

## 一、额外能力补充

### 1.1 记忆自动提取（31 memery.md）

Agent 运行后自动从对话中提取重要信息写入 L2/L3：

```go
// internal/memory/extractor.go
type MemoryExtractor struct {
    llm    model.LLM
    store  *sessionmemory.Store
}

func (e *MemoryExtractor) ExtractAfterTurn(ctx, sessionID, agentID string, messages []Message) error
```

提取策略：
- L2 情景记忆：重要事件（用户反馈、关键决策、错误恢复）
- L3 语义记忆：事实性知识（用户偏好、领域知识）

### 1.2 记忆检索增强（38 memory.md）

L3 向量检索与 Agent 运行时注入：

```go
// internal/memory/retriever.go
type MemoryRetriever struct {
    vector  *pgvector.VectorStore
    embed   model.LLM
}

func (r *MemoryRetriever) Retrieve(ctx, agentID, query string, topK int) ([]Memory, error)
```

### 1.3 记忆管理 API

```protobuf
rpc GetMemoryLayers(GetMemoryLayersRequest) returns (GetMemoryLayersResponse) {
  option (google.api.http) = { get: "/v1/memories/layers" };
}
rpc SearchMemories(SearchMemoriesRequest) returns (SearchMemoriesResponse) {
  option (google.api.http) = { post: "/v1/memories/search" body: "*" };
}
```

### 1.4 Web 前端补充

**MemoryLayerConfig.vue**：各层记忆配置面板（嵌入 Agent 设置页记忆 Tab）

| 层 | 控件 | 说明 |
|---|------|------|
| L0 | `QToggle` + `QInput` | 启用 + 窗口大小 |
| L1 | `QToggle` | 启用工作记忆 |
| L2 | `QToggle` + `QSlider` | 启用 + 重要性阈值 |
| L3 | `QToggle` + `QSelect` | 启用 + 嵌入模型 |
| L4 | `QToggle` | 启用身份记忆 |
