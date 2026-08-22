# ADR-KN-LINKINDEX：多副本读路径走 DB，不广播内存图

## 状态：已接受（2026-08-22）

## 背景

`LinkIndex`（`internal/biz/knowledge/link_index.go`）是进程内邻接表：启动从 `knowledge_block_refs` 全量重放，写路径 `ApplyDocDelta` 后可选推送 `knowledge.graph.delta`。交叉参考与 SP1 设计把多副本一致性写成「需事件广播，另立 ADR」。

若按字面做副本间全图广播：

1. **事件不可靠**：`knowledge.graph.delta` 已按 AS-EVT-01 定为 Informational（WS-only、可从边表重放）。把它升成 Critical 并做成副本同步总线，等于用不可靠通道承担正确性。
2. **与 WS 语义冲突**：该事件的消费者是前端图谱/反链 UI，不是兄弟进程。复用同一载荷做副本同步会把 UI 增量协议钉成集群协议。
3. **读路径已有 DB 兜底**：SP1-E 在 `LinkIndex.Loaded()==false` 时走 `BlockLinkReader` 落库。多副本上「本进程缓存未热」与「启动窗口」是同一问题。
4. **N 副本 × 全图内存** 随边数线性放大，收益只是少一次 SQL。

## 决策

### D1：`knowledge_block_refs` 是反链/dangling 的跨进程真相源

多副本部署下，**正确性读路径以 Postgres 边表为准**。`LinkIndex` 只允许作为**本进程可选缓存**（热路径加速），不得作为跨进程一致性原语。

### D2：禁止副本间广播内存图

不引入 `LinkIndex` 增量 pub/sub、不把 `knowledge.graph.delta` 升为 Critical、不在 replica 间重放 `ApplyDocDelta`。

`knowledge.graph.delta` 保持 Informational：**仅服务本进程已连接的 WS 客户端**。丢失可从边表重放；前端重连用既有 RPC 补全。

### D3：缓存失效策略（本进程）

| 场景 | 行为 |
|------|------|
| 本进程写路径 `ApplyDocDelta` / `RemoveDoc` | 继续更新本地 `LinkIndex`（零跨进程） |
| 他进程写入 | 本进程缓存可能短暂陈旧；反链 RPC 在缓存未加载或调用方要求强一致时走 DB |
| 进程启动 | `LoadAll` 从边表重建本地缓存；失败降级，读走 DB |
| 未来加速（非本 ADR 范围） | 可加短 TTL / `max(updated_at)` 版本校验的 read-through，仍不是广播 |

### D4：产品边界

- Knowledge 图谱是**可引用工作区的派生索引**，不是实时协作 CRDT。
- 需要强一致反链时读 DB；需要低延迟 UI 时读本进程缓存 + WS 增量。

## 后果

正面：

- 多副本部署无需新消息总线；水平扩展不复制全图内存。
- 与 AS-EVT-01 事件分级一致，避免 Informational 通道承担正确性。
- 读路径与 SP1-E 兜底同构，实现面小。

负面：

- 他进程刚写入的边，本进程缓存可能滞后到下次 Load / 本地写 / 或读穿透 DB。
- 3D 图谱若只订 WS、不轮询，他副本写入的边要等刷新或重连。可接受（Informational）。

## 替代方案（否决）

| 方案 | 否决理由 |
|------|----------|
| replica-wide `ApplyDocDelta` 广播 | 把 Informational WS 事件升格为集群协议；丢事件即图分裂 |
| 每副本常驻全图 + Redis 版本号 | 仍要全图内存；版本号只解决「知不知道过期」，最终仍得读 DB |
| 客户端 CRDT / 离线副本 | SP1-ADR-4 已否决；复杂度与产品边界不符 |

## 落地

- 代码注释与 `37-knowledge` 三件套 / 交叉参考改为「读 DB + 本进程缓存」，删除「多副本必须广播」表述。
- Wave 3 Usecase 拆分时，反链端口保持「cache-then-DB」，不引入 Graph 广播依赖。
