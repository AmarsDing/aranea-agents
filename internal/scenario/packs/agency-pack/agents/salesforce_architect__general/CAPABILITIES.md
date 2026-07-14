## 🚀 高级能力
### 何时使用 Platform Events vs Change Data Capture

| 因素 | Platform Events | CDC |
|--------|----------------|-----|
| 自定义负载 | 是——定义自己的模式 | 否——镜像 sObject 字段 |
| 跨系统集成 | 首选——解耦生产者/消费者 | 有限——仅 Salesforce 原生事件 |
| 字段级追踪 | 否 | 是——捕获哪些字段变更 |
| 重放 | 72 小时重放窗口 | 3 天保留 |
| 体量 | 标准高体量（10 万/天） | 与对象事务体量挂钩 |
| 用例 | "某事发生了"（业务事件） | "某事变了"（数据同步） |

### 多云数据架构

在跨 Sales Cloud、Service Cloud、Marketing Cloud 和 Data Cloud 设计时：
- **单一真相源：** 定义哪个云拥有哪个数据域
- **身份解析：** Data Cloud 用于统一画像，Marketing Cloud 用于分群
- **同意管理：** 按云按渠道追踪 opt-in/opt-out
- **API 预算：** Marketing Cloud API 有独立于核心平台的限制

### Agentforce 架构

- Agent 在 Salesforce governor 限制内运行——设计在 CPU/SOQL 预算内完成的操作
- 提示模板：版本控制系统提示，使用自定义元数据进行 A/B 测试
- 接地：使用 Data Cloud 检索进行 RAG 模式，而非 agent 操作中的 SOQL
- 护栏：Einstein Trust Layer 用于 PII 掩码，主题分类用于路由
- 测试：使用 AgentForce 测试框架，而非手动对话测试
