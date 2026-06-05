## Context

Phase1 Taxonomy Rename and Unify 变更已归档，但 aranea-review 深度复检发现多个遗留问题。当前状态：

- 后端 Go 代码中 `AgentCategory`/`agent_category` 零残留（已完全清理）
- 前端类型/路由名零残留（`AgentCategory`/`agent-category`/`agent-categories` 已清除）
- 但存在 2 个阻断级问题、13 个建议级问题、4 个提示级问题

关键遗留问题分类：
1. **数据正确性**：selfmedia variant 连字符、agent.proto 字段未重命名
2. **命名一致性**：前端 68 处 `category*` 变量名未统一
3. **死代码**：3 个前端文件 + 1 个后端 YAML 文件未删除
4. **功能缺失**：softwaredev Agent 不足（10 vs ~82）、finance 缺 variant/team、taxonomy.yaml 缺丰富字段
5. **架构遗漏**：IndustryTaxonomyService 未注册 gRPC

## Goals / Non-Goals

**Goals:**
- 修复所有阻断级问题（variant 连字符、proto 字段重命名）
- 清理所有死代码文件
- 统一前端变量命名为 `taxonomy*`
- 补全 taxonomy.yaml 丰富字段和部门 key 对齐
- 补全 finance/softwaredev Agent 定义
- 补全 IndustryTaxonomyService gRPC 注册

**Non-Goals:**
- 不改变 IndustryTaxonomyService 与 TaxonomyService 并存的架构（评估合并为后续独立变更）
- 不新增行业或岗位
- 不改变前端组件架构
- 不修改 `variantSafeRe` 正则（当前允许连字符，本次仅修复数据层）

## Decisions

### D1: agent.proto 字段重命名策略

**决策**：重命名 `category_position_id` → `taxonomy_position_id`

**理由**：
- Go/Biz 层已使用 `TaxonomyPositionID`，Proto 是唯一残留
- 前端 wireNormalize 的兼容映射可随之清理
- 属于 BREAKING 变更，但当前系统尚未对外发布

**替代方案**：保留 proto 旧名，仅靠前端兼容映射 → 拒绝，命名不一致会持续造成混淆

### D2: selfmedia variant 命名修复策略

**决策**：将 4 个连字符 variant 改为下划线（`data-driven` → `data_driven` 等），同步更新 agent_key 和 prompt 文件路径

**理由**：
- 设计规范要求 `^[a-z0-9_]+$`，连字符不合规
- 当前 `variantSafeRe` 实际允许连字符（`^[a-zA-Z0-9_-]+$`），但不应依赖实现偏差
- 修改后需 re-seed 数据库

### D3: 前端变量名批量重命名策略

**决策**：一次性批量重命名 ~68 处 `category*` → `taxonomy*`，不保留兼容别名

**理由**：
- 兼容别名已存在数月，无外部消费者依赖
- 逐步迁移增加维护负担，不如一次性清理
- 删除 `categoryTreeUtils.ts` 兼容桥接文件

### D4: taxonomy.yaml 丰富字段补全策略

**决策**：在 taxonomy.yaml 中补充 responsibilities/skills_required/seniority_level/variants 字段，同步更新 taxonomy_loader.go 和 seed 逻辑

**理由**：
- 设计文档 §5.1 明确要求这些字段
- 当前 metadata_json 为空，岗位描述信息无法通过 API 获取

### D5: softwaredev Agent 补全策略

**决策**：按 P1→P2→P3 顺序在 agents.yaml 中补全 Agent 定义，prompt 文件已存在无需重写

**理由**：
- taxonomy.yaml 岗位定义和 prompt 文件已就绪，仅缺 agents.yaml 条目
- 分批降低风险，P1 优先

## Risks / Trade-offs

- [Proto BREAKING 变更] → agent.proto 字段重命名影响所有客户端，需同步更新前端 → 通过 `make api` 重新生成 + 前端 wireNormalize 同步修改
- [variant 重命名导致 seed 数据不一致] → 已有数据库中的旧 variant key 需更新 → re-seed 或手动 SQL 更新
- [前端批量重命名范围大] → ~68 处修改可能引入遗漏 → 全局 grep 验证 + `pnpm build` 验证
- [softwaredev Agent 补全工作量大] → ~72 个 Agent 定义需编写 → 分批执行，P1 优先
