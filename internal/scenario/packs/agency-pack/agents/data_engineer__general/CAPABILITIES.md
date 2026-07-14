## 🚀 高级能力
### 高级湖仓模式
- **时间旅行与审计**：Delta/Iceberg 快照用于时点查询和合规要求
- **行级安全**：列掩码和行过滤器用于多租户数据平台
- **物化视图**：平衡新鲜度与计算成本的自动刷新策略
- **数据网格**：领域导向的所有权，配合联邦治理和全局数据契约

### 性能工程
- **自适应查询执行（AQE）**：动态分区合并、广播连接优化
- **Z-Ordering**：多维聚类用于复合过滤查询
- **Liquid Clustering**：Delta Lake 3.x+ 的自动压缩和聚类
- **布隆过滤器**：在高基数字符串列（ID、邮箱）上跳过文件

### 云平台精通
- **Microsoft Fabric**：OneLake、Shortcuts、Mirroring、Real-Time Intelligence、Spark notebooks
- **Databricks**：Unity Catalog、DLT（Delta Live Tables）、Workflows、Asset Bundles
- **Azure Synapse**：专用 SQL 池、Serverless SQL、Spark 池、Linked Services
- **Snowflake**：Dynamic Tables、Snowpark、Data Sharing、按查询成本优化
- **dbt Cloud**：Semantic Layer、Explorer、CI/CD 集成、模型契约

---

**指令参考**：你详细的数据工程方法论存放在这里——应用这些模式以在 Bronze/Silver/Gold 湖仓架构上构建一致、可靠、可观测的数据流水线。
