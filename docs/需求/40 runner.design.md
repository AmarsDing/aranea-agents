# Runner 运行器模块 — 实现设计文档

> 对应需求：`40 runner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 运行器完善：AgentFactory、PluginManager、ArtifactService、SessionIngestor、AwaitUserReplyRouting、Status/Cancel、AgentLookup。对标 trpc-agent-go `runner` 包。

---

## 二、Proto 层

无需独立 Proto，通过 Gateway 和 Chat Service 暴露。

---

## 三、Biz 层

### 3.1 领域模型

```go
type RunnerConfig struct {
    AgentFactory       AgentFactory
    PluginManager      PluginManager
    ArtifactService    ArtifactService
    SessionIngestor    SessionIngestor
    AwaitUserRouting   AwaitUserReplyRouting
    AgentLookup        AgentLookup
}
```

### 3.2 接口定义

```go
type AgentFactory interface {
    Create(ctx, agentID string) (agent.Agent, error)
}

type PluginManager interface {
    Load(ctx, pluginID string) error
    Unload(ctx, pluginID string) error
    List() []string
}

type SessionIngestor interface {
    Ingest(ctx, sessionID string, event AgentEvent) error
}

type AwaitUserReplyRouting interface {
    Route(ctx, sessionID string) (string, error)
}

type AgentLookup interface {
    Lookup(ctx, agentKey string) (agent.Agent, error)
}
```

---

## 四、运行时层

### 4.1 AgentFactory 实现

```go
// internal/agent/factory.go
type BizAgentFactory struct {
    agents  biz.AgentRepository
    builder func(ctx, biz.Agent) (agent.Agent, error)
}

func (f *BizAgentFactory) Create(ctx, agentID string) (agent.Agent, error) {
    a, err := f.agents.GetByID(ctx, agentID)
    if err != nil {
        return nil, err
    }
    return f.builder(ctx, a)
}
```

### 4.2 SessionIngestor 实现

```go
// internal/agent/ingestor.go
type BizSessionIngestor struct {
    sessions *biz.SessionUsecase
    broker   *biz.TeamRunEventBroker
}

func (ing *BizSessionIngestor) Ingest(ctx, sessionID string, event AgentEvent) error {
    // 更新 Session 上下文用量
    // 推送事件到 SSE
}
```

### 4.3 AgentLookup 实现

```go
// internal/agent/lookup.go
type BizAgentLookup struct {
    factory AgentFactory
    cache   map[string]agent.Agent
}

func (l *BizAgentLookup) Lookup(ctx, agentKey string) (agent.Agent, error)
```

---

## 五、Data 层

无需新增，通过 Biz 层接口实现。

---

## 六、Wire 注入

待新增：
```
agent.ProviderSet → NewBizAgentFactory, NewBizSessionIngestor, NewBizAgentLookup
```

---

## 七、Web 前端设计

Runner 为运行时基础设施，无独立前端页面。相关 UI 通过 Gateway 和 Chat 页面暴露。
