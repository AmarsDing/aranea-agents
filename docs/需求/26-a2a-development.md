# A2A 协议 — 开发计划

> **版本**：2026-05-20（Review 修复完成） | **状态**：✅ Phase 1–3.5 + Review 项已落地
> **需求**：[26 a2a-protocol.md](./26%20a2a-protocol.md) · **设计**：[26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) §十二（传输与流式）
> **变更记录**：[Phase 1–2](../changelog/2026-05-20-A2A-Phase1-2.md) · [Phase 3](../changelog/2026-05-20-A2A-Phase3.md) · [Phase 3.5](../changelog/2026-05-20-A2A-Phase35.md) · **[Review Fixes](../changelog/2026-05-20-A2A-Review-Fixes.md)**

---

## 1. 模块定位

（同 Phase 3，略）

**传输原则（2026-05-20）**：A2A 外部协议用 **A2A HTTP（一元 + SSE 流）**；Chat UI 用 **`/v1/ws`**；Admin Invoke 用 **一元 JSON**。**SSE 不是全平台必选**，见设计文档 §十二。

---

## 2. 现状评估（2026-05-20）

| 项 | 状态 | 证据 |
|----|------|------|
| Phase 1–3 全部项 | ✅ | 见 [Phase 3 changelog](../changelog/2026-05-20-A2A-Phase3.md) |
| **Public Endpoint 流式广告** | ✅ | `BuildA2AEndpointServer(..., streaming=true)`；AgentCard capabilities.streaming |
| **Proxy 流式** | ✅ | `enable_streaming` → `a2aagent.WithEnableStreaming` |
| **公开 URL 配置** | ✅ | env / conf / `GET /v1/a2a/config` + `/a2a` Banner |
| **ResolveInvokeTarget** | ✅ | 本地 disabled 不 fallback 远程 |
| **Graph resume metadata** | ✅ | flattened 根级字段 |
| **远程 registry 直接 Invoke** | ✅ | `invoker.go` + `remote_invoke.go` |
| **网关联邦 Discover** | ✅ | `GatewayDiscover` RPC + `a2a_gateway.go` |
| Admin Invoke SSE 流式 | ❌ | 有意非流式（运维聚合） |
| 网关周期性健康 / 联邦路由 | 🟡 | `check_health` 同步探测；无后台 job |
| 速率限制 | ❌ | API 网关层 |

---

## 3. 差距与优先级（剩余）

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | Admin Invoke 可选流式 | **P3** | 非必须；外部客户端应走 Public Endpoint |
| 2 | 网关健康后台 job | **P2** | Cron 探测 + Prometheus `a2a_gateway_healthy` |
| 3 | 速率限制 | **P2** | Ingress / middleware |
| 4 | Ent 列级 agent_kind | **P3** | schema 迁移 |

---

## 4. 开发阶段

### Phase 3.5 — 联邦与 Graph（P2）— ✅ MVP 已完成

| # | 任务 | 状态 |
|---|------|------|
| 3.5.1 | 传输模型文档（SSE/WS/多协议） | ✅ 设计 §十二 |
| 3.5.2 | Graph resume 元数据桥接 | ✅ `graph_resume.go` |
| 3.5.3 | 远程 registry Invoke | ✅ `remote_invoke.go` + invoker 路由 |
| 3.5.4 | GatewayDiscover API | ✅ `a2a_gateway.go` + proto |
| 3.5.5 | Discover  enrichment（source/url） | ✅ proto + biz |

### Phase 4 — 网关增强（P3）— 待实现

| # | 任务 | 验收标准 |
|---|------|----------|
| 4.1 | 健康探测 Cron | 远程 registry 离线告警 |
| 4.2 | 联邦路由策略 | 按 healthy/source 选路 |
| 4.3 | 速率限制 / mTLS Server 终止 | Ingress 文档 + 可选内置 |

---

## 5. 验收标准（Phase 3.5 新增）

- [x] 文档明确：A2A 外部协议 vs `/v1/ws` vs Admin Invoke 传输分工
- [x] Public Endpoint 广告 streaming；Proxy 可配置 streaming
- [x] Graph resume metadata 可由 `BuildGraphResumeMetadata` 编码
- [x] Discover/Gateway 远程条目可用 registry id Invoke
- [x] `GET /v1/a2a/gateway/discover` 返回 local + remote 联邦视图

---

## 6. 依赖与风险

- **勿混用传输**：在 Chat WS 上复刻 A2A 流式事件会增加双栈维护；外部集成应使用 `/v1/a2a/public/...`。
- **Admin Invoke 非流式**：长任务请用 Public Endpoint 或 Proxy Agent Chat 路径。
- **Graph resume**：resume 语义以 trpc-agent-go `server/a2a` 为准；项目层仅负责 metadata 编码。
