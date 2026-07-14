## 📋 你的技术交付物
### 重连安全的客户端协议

```typescript
// 契约：服务器为每个操作分配 seq；客户端确认已应用的操作；
// 恢复时重放间隙。重复按构造不可能（opId 去重）。
class SyncConnection {
  private lastServerSeq = 0;                    // 本地已应用的最高 seq
  private pending = new Map<string, Op>();      // 已发送，尚未确认
  private backoff = 500;

  connect() {
    this.ws = new WebSocket(`${WS_URL}?resumeFrom=${this.lastServerSeq}`);
    this.ws.onmessage = (e) => this.receive(JSON.parse(e.data));
    this.ws.onclose = () => this.scheduleReconnect();
    this.ws.onopen = () => {
      this.backoff = 500;
      this.pending.forEach((op) => this.ws.send(JSON.stringify(op))); // 安全：opId 去重
    };
  }

  send(op: Omit<Op, 'opId'>) {
    const stamped = { ...op, opId: crypto.randomUUID() };  // 客户端生成的标识
    this.pending.set(stamped.opId, stamped);
    this.queueLocally(stamped);                            // 乐观应用 + 离线队列
    if (this.ws.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(stamped));
  }

  private receive(msg: ServerMsg) {
    if (msg.type === 'op') {
      this.lastServerSeq = msg.seq;                        // 服务器排序是真相
      this.pending.delete(msg.opId);                       // 确认我们自己的操作，或...
      this.applyRemote(msg);                               // ...他人的操作，已变换
    }
  }

  private scheduleReconnect() {
    const jitter = Math.random() * this.backoff;           // 防群体效应
    setTimeout(() => this.connect(), this.backoff + jitter);
    this.backoff = Math.min(this.backoff * 2, 30_000);
  }
}
```

### 收敛模型决策表

| 数据类型 | 正确的机制 | 原因 |
|-----------|-----------------|-----|
| 协作富文本 | CRDT（Yjs/Loro）或 OT（服务器变换） | 同一范围内的并发插入必须交叉插入，而非覆盖 |
| 表单字段、设置、状态 | 服务器仲裁的最后写入胜出 + 版本检查 | 用户期望"最后保存胜出"；合并的下拉选项无意义 |
| 计数器（点赞、投票、配额） | CRDT 计数器 / 服务器递增操作 | 最后写入胜出会丢失递增；发送*操作*，而非计算后的总数 |
| 有序列表（看板） | 分数索引 + 服务器平局裁决 | 移动操作必须在每次拖拽时合并而非重新编号整个列表 |
| 光标、选区、状态 | 临时广播、TTL、最后状态胜出 | 没人需要光标抖动的持久、收敛历史 |

### 在线状态系统（临时、TTL 作用域、合并）

```typescript
// 基于 Redis 的状态：心跳刷新 TTL；沉默即离开。
// 每个房间最多扇出约 10 次/秒的状态更新——合并，最后写入胜出。
async function heartbeat(roomId: string, userId: string, state: PresenceState) {
  await redis.hset(`presence:${roomId}`, userId, JSON.stringify({
    ...state,                    // 光标、选区、视口
    updatedAt: Date.now(),
  }));
  await redis.expire(`presence:${roomId}`, 60);            // 房间 GC
  await redis.publish(`room:${roomId}:presence`, userId);  // 订阅者重新读取哈希
}
// 客户端规则：渲染 updatedAt 新鲜（< 30s）的对端；淡出其余的。
// 状态绝不写入文档日志——不同的通道，不同的保证。
```

### 扇出架构（一个房间，数千个套接字）

```text
clients ──ws──▶ gateway nodes (stateless, any node serves any room)
                   │  subscribe room:{id}
                   ▼
             pub/sub backplane (Redis/NATS)          ordering + durability
                   ▲                                   ┌──────────────────┐
                   │  publish op(seq)                  │ op log (append-  │
             room authority ──────assign seq──────────▶│ only, per room)  │
             (sharded by roomId — single writer        └──────────────────┘
              per room = trivially correct ordering)      └─▶ resumeFrom replay
```

每个房间单写入者使排序变得简单，通过分片房间而非解决每次按键的分布式共识来扩展。操作日志免费提供恢复、审计和时间旅行调试。

### 恶意网络测试清单

| 场景 | 必须保持 |
|----------|-----------|
| 操作中途断开套接字，重连 | 操作恰好应用一次；无间隙，无重复 |
| 离线 1 小时，排队 200 个操作，然后重连 | 队列按顺序重放；文档与并发的远程编辑收敛 |
| 两个客户端同时编辑同一个词 | 两者都收敛到相同字节；任何编辑都不会被静默丢失 |
| 活跃会话期间服务器部署 | 客户端在 5 秒内排空重连；零操作丢失；无惊群效应 |
| 热门房间的慢消费者 | 服务器内存有界；消费者获得合并状态，然后追赶 |
