# A2A 协议模块 — 实现设计文档

> 对应需求：`26 a2a-protocol.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent-to-Agent 通信协议：A2A Agent 构建、远程 Agent 发现、消息路由、Graph 恢复集成。对标 trpc-agent-go `a2aagent` + `internal/a2a`。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service A2AService {
  rpc RegisterA2AAgent(RegisterA2AAgentRequest) returns (A2AAgent) {
    option (google.api.http) = { post: "/v1/a2a/agents" body: "*" };
  }
  rpc ListA2AAgents(ListA2AAgentsRequest) returns (ListA2AAgentsResponse) {
    option (google.api.http) = { get: "/v1/a2a/agents" };
  }
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse) {
    option (google.api.http) = { post: "/v1/a2a/messages" body: "*" };
  }
  rpc DiscoverAgent(DiscoverAgentRequest) returns (AgentCard) {
    option (google.api.http) = { get: "/v1/a2a/discover" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type A2AAgentConfig struct {
    ID         string
    AgentID    string
    RemoteURL  string
    AgentCard  *AgentCard
    Status     string
}

type AgentCard struct {
    Name        string
    Description string
    URL         string
    Capabilities []string
    AuthType    string
}

type A2AMessage struct {
    ID        string
    FromAgent string
    ToAgent   string
    Type      string  // "task"/"message"/"artifact"
    Content   string
    Metadata  map[string]string
}
```

### 3.2 Usecase

```go
func (uc *A2AUsecase) RegisterAgent(ctx, cfg A2AAgentConfig) (A2AAgentConfig, error)
func (uc *A2AUsecase) DiscoverAgent(ctx, url string) (*AgentCard, error)
func (uc *A2AUsecase) SendMessage(ctx, msg A2AMessage) (A2AMessage, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/a2a_agent.go` — A2A Agent 配置

### 4.2 A2A Client

```go
// internal/a2a/client.go
type A2AClient struct {
    httpClient *http.Client
}

func (c *A2AClient) Discover(ctx, url string) (*AgentCard, error)
func (c *A2AClient) SendMessage(ctx, url string, msg A2AMessage) (A2AMessage, error)
```

---

## 五、运行时层

### 5.1 A2A Agent 构建

```go
// internal/agent/trpc_build.go
func BuildA2AAgent(ctx, cfg A2AAgentConfig, deps) (agent.Agent, error)
```

### 5.2 Graph 恢复集成

```go
// internal/a2a/graph_resume.go
func ResumeFromA2A(ctx, graph *trpcgraph.StateGraph, msg A2AMessage) error
```

---

## 六、Service 层

```go
func (s *A2AService) RegisterA2AAgent(ctx, req) (*A2AAgent, error)
func (s *A2AService) ListA2AAgents(ctx, req) (*ListA2AAgentsResponse, error)
func (s *A2AService) SendMessage(ctx, req) (*SendMessageResponse, error)
func (s *A2AService) DiscoverAgent(ctx, req) (*AgentCard, error)
```

---

## 七、Wire 注入

待新增：
```
data.ProviderSet → NewA2ARepo
biz.ProviderSet → NewA2AUsecase
service.ProviderSet → NewA2AService
```

---

## 八、Web 前端设计

### 8.1 组件

**A2AAgentListPage.vue**：A2A Agent 列表 + 注册/发现/测试

**A2AAgentRegisterDialog.vue**：注册远程 Agent，输入 URL → 自动发现 AgentCard

### 8.2 API

```typescript
export async function registerA2AAgent(req: RegisterA2ARequest): Promise<A2AAgent>
export async function listA2AAgents(): Promise<A2AAgent[]>
export async function discoverAgent(url: string): Promise<AgentCard>
export async function sendA2AMessage(req: SendMessageRequest): Promise<SendMessageResponse>
```
