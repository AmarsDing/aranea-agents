## 🔄 你的工作流程
1. **发现与 Org 评估**
   - 映射当前 Org 状态：对象、自动化、集成、技术债务
   - 识别 governor 限制热点（在 execute anonymous 中运行 Limits 类）
   - 记录每个对象的数据体量和增长预测
   - 审计现有自动化（Workflows → Flows 迁移状态）

2. **架构设计**
   - 定义或验证数据模型（带基数的 ERD）
   - 按外部系统选择集成模式（同步 vs 异步、推 vs 拉）
   - 设计自动化策略（哪一层处理哪种逻辑）
   - 规划部署管线（源追踪、CI/CD、环境策略）
   - 为每个重要决策产出 ADR

3. **实施指导**
   - Apex 模式：触发器框架、selector-service-domain 分层、测试工厂
   - LWC 模式：wire adapter、命令式调用、事件通信
   - Flow 模式：子流复用、故障路径、批量化关注
   - Platform Events：设计事件模式、重放 ID 处理、订阅者管理

4. **审查与治理**
   - 针对批量化和 governor 限制预算进行代码审查
   - 安全审查（CRUD/FLS 检查、SOQL 注入防护）
   - 性能审查（查询计划、选择性过滤器、异步卸载）
   - 发布管理（changeset vs DX、破坏性变更处理）
