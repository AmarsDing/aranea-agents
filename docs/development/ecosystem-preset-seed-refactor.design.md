# 种子入库重构 — 设计文档

## 1. 系统架构

### 1.1 种子数据分层模型

```
┌─────────────────────────────────────────────────────────┐
│                    种子数据分层                           │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  L1 系统内置层 (system_builtin)                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │ 精灵助手  │ │ 系统管家  │ │ 记忆管家  │ │ 技能管家  │  │
│  │ readonly  │ │ readonly  │ │ readonly  │ │ readonly  │  │
│  │ 不可删除  │ │ 不可删除  │ │ 不可删除  │ │ 不可删除  │  │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │
│  加载时机：P1 启动阶段，ON CONFLICT DO UPDATE            │
│                                                         │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  L2 附带生态层 (ecosystem_preset)                        │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │ 金融行业  │ │ 自媒体    │ │ 软件开发  │               │
│  │ 30 Agent │ │ 25 Agent │ │ 40 Agent │               │
│  │ 5 Team   │ │ 3 Team   │ │ 8 Team   │               │
│  │ 可编辑    │ │ 可编辑    │ │ 可编辑    │               │
│  │ 可删除    │ │ 可删除    │ │ 可删除    │               │
│  └──────────┘ └──────────┘ └──────────┘               │
│  加载时机：用户在系统设置页按需触发                        │
│  卸载：按行业卸载，弹框确认                               │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

### 1.2 种子管道架构

```
改造前（3 条管道并存）：
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│ 硬编码 SQL    │  │ YAML Loader  │  │ Pack 引擎    │
│ seed_system   │  │ industry_    │  │ seed_pack    │
│ admin.go      │  │ agent_seed   │  │ (Lazy)       │
│               │  │ .go          │  │              │
│ P1 阶段       │  │ 启动时自动    │  │ 延迟 3s      │
│ kind=system_  │  │ kind=默认     │  │ 版本门控      │
│ builtin       │  │ (user)       │  │              │
└──────────────┘  └──────────────┘  └──────────────┘
       ↓ 重复导入风险 ↓

改造后（2 条管道，职责清晰）：
┌──────────────────────┐  ┌──────────────────────┐
│ L1 启动管道           │  │ L2 API 触发管道       │
│ P1 阶段强制执行       │  │ 用户按需触发          │
│                      │  │                      │
│ SeedSystemAdminAgent │  │ POST /ecosystem/     │
│ SeedSpiritAgent      │  │   preset/load        │
│ SeedMemoryAgent      │  │                      │
│ SeedSkillsAgent      │  │ SeedPackIndustry     │
│ SeedBuiltinCLI...    │  │   (finance)          │
│ SeedPackBuiltin...   │  │ SeedPackIndustry     │
│ SeedSpiritPrompt...  │  │   (selfmedia)        │
│ SeedButlerPrompt...  │  │ SeedPackIndustry     │
│ SeedCronTasks        │  │   (softwaredev)      │
│                      │  │                      │
│ kind=system_builtin  │  │ kind=ecosystem_preset│
│ readonly=1           │  │ 可编辑可删除          │
│ 不可删除             │  │                      │
└──────────────────────┘  └──────────────────────┘
```

## 2. 数据模型

### 2.1 Agent Kind 枚举

```
改造前：user | system | system_builtin | industry_template | marketplace | certified
改造后：user | system_builtin | ecosystem_preset | marketplace | certified
```

| Kind | 含义 | 可编辑 | 可删除 | 徽章 |
|------|------|--------|--------|------|
| `user` | 用户自建 | 是 | 是 | 无 |
| `system_builtin` | 系统内置 | 是 | 否 | 内置(蓝) |
| `ecosystem_preset` | 附带生态 | 是 | 是 | 预设(绿) |
| `marketplace` | 商城导入 | 是 | 是 | 商城(紫) |
| `certified` | 认证 | 是 | 是 | 认证(橙) |

### 2.2 Team Kind 字段

```go
// internal/data/ent/schema/team.go 新增
field.Enum("kind").Values(
    "user", "system_builtin", "ecosystem_preset", "marketplace", "certified",
).Default("user").Comment("team kind: aligned with agent.kind for unified permission model")
```

**kind vs source 职责划分**：
- `kind`：权限分类（决定可编辑性/可删除性/徽章显示）
- `source`：来源追踪（`imported`/`system`/`user`，用于审计和统计）

### 2.3 ecosystem_loaded 状态存储

```json
// system_settings.ecosystem_loaded 字段（TEXT, JSON 格式）
{
  "finance": {
    "loaded": true,
    "loaded_at": "2026-06-05T10:00:00Z",
    "agents": 30,
    "teams": 5,
    "taxonomy_nodes": 40
  },
  "selfmedia": {
    "loaded": false
  },
  "softwaredev": {
    "loaded": false
  }
}
```

设计理由：
- JSON 格式支持按行业独立追踪加载状态
- 前端可直接读取展示各行业加载状态
- 支持部分加载失败场景
- 支持"重新加载"单个行业

## 3. API 设计

### 3.1 加载附带生态

```
POST /api/v1/admin/ecosystem/preset/load

Request:
{
  "industries": ["finance", "selfmedia", "softwaredev"],  // 可选，默认全部
  "force": false  // 可选，true 时重新加载已加载行业
}

Response 200:
{
  "results": {
    "finance": { "agents_created": 30, "teams_created": 5, "taxonomy_nodes": 40 },
    "selfmedia": { "agents_created": 25, "teams_created": 3, "taxonomy_nodes": 35 }
  },
  "already_loaded": ["softwaredev"],  // 仅 force=false 时存在
  "errors": {}  // 部分失败时包含错误信息
}
```

执行流程：
1. 读取 `system_settings.ecosystem_loaded` JSON
2. 对每个请求的行业：
   - 若 `loaded=true` 且 `force=false`，跳过并加入 `already_loaded`
   - 若 `loaded=true` 且 `force=true`，重置状态后重新加载
   - 若 `loaded=false`，执行加载
3. 调用 `SeedPackIndustry(ctx, scenarioDir, industryKey, WithKindOverride("ecosystem_preset"))`
4. 更新 `ecosystem_loaded` JSON
5. 返回结果

### 3.2 卸载附带生态

```
POST /api/v1/admin/ecosystem/preset/unload

Request:
{
  "industries": ["finance"]  // 必填，指定要卸载的行业
}

Response 200:
{
  "results": {
    "finance": {
      "agents_deleted": 30,
      "teams_deleted": 5,
      "taxonomy_nodes_deleted": 40,
      "teams_modified": 2  // 跨行业 Team 移除成员数
    }
  }
}

Response 400:
{
  "error": "industry not loaded",
  "industry": "finance"
}
```

执行流程：
1. 校验指定行业在 `ecosystem_loaded` 中为 `loaded: true`
2. 查找该行业下的所有分类节点（递归 industry → departments → positions），软删除
3. 查找 `kind = 'ecosystem_preset'` 且 `taxonomy_position_id` 属于该行业分类的 Agent，软删除
4. 查找 `kind = 'ecosystem_preset'` 的 Team：
   - 成员 Agent 全部属于该行业 → 软删除 Team
   - 成员 Agent 部分属于该行业 → 保留 Team，移除已删除 Agent 成员
5. 更新 `ecosystem_loaded` 中对应行业状态为 `loaded: false`
6. 返回删除统计

### 3.3 Agent/Team 删除保护

```
DELETE /api/v1/admin/agents/{id}

Response 403 (当 kind=system_builtin):
{
  "error": "cannot delete system_builtin agent",
  "agent_id": 123
}
```

## 4. 前端设计

### 4.1 系统设置 — 附带生态区块

```
┌──────────────────────────────────────────────────────┐
│  附带生态                                              │
│                                                      │
│  系统附带金融、软件开发、自媒体等行业的 Agent 模板和     │
│  分类数据。加载后可自由编辑和删除。                      │
│                                                      │
│  ┌────────────────────────────────────────────────┐  │
│  │ 金融          已加载  30 Agent · 5 Team  [卸载] │  │
│  │ 自媒体        未加载                    [加载]  │  │
│  │ 软件开发      未加载                    [加载]  │  │
│  └────────────────────────────────────────────────┘  │
│                                                      │
│  [加载全部附带生态]   ← 仅当有未加载行业时显示          │
│                                                      │
└──────────────────────────────────────────────────────┘
```

卸载确认对话框：
```
┌──────────────────────────────────────────────────────┐
│  ⚠ 确认卸载                                          │
│                                                      │
│  卸载将删除金融行业下所有数据：                         │
│  • 30 个 Agent                                       │
│  • 5 个 Team                                         │
│  • 40 个分类节点                                      │
│                                                      │
│  此操作不可撤销。确定要卸载吗？                         │
│                                                      │
│              [取消]  [确认卸载]                        │
└──────────────────────────────────────────────────────┘
```

### 4.2 行业分类树形布局

```
┌──────────────────────────────────────────────────────┐
│  行业分类管理                    [搜索] [仅看自建]      │
├──────────────────────────────────────────────────────┤
│                                                      │
│  ▼ 金融                              [编辑][+部门]   │
│    ▼ 量化交易                        [编辑][+岗位]   │
│      ┌─────────┐ ┌─────────┐ ┌─────────┐           │
│      │量化研究员│ │算法交易  │ │量化开发  │ ← 可拖拽  │
│      │ Alpha因子│ │ 工程师   │ │ 工程师   │           │
│      └─────────┘ └─────────┘ └─────────┘           │
│    ▶ 风控合规                                        │
│    ▶ 投资研究                                        │
│                                                      │
│  ▶ 自媒体                                            │
│  ▶ 软件开发                                          │
│                                                      │
└──────────────────────────────────────────────────────┘
```

组件结构：
```
TaxonomyPage.vue
  └── TaxonomyTree.vue（改造）
        ├── TaxonomyIndustryNode.vue（行业节点，QExpansionItem）
        │     └── TaxonomyDepartmentNode.vue（部门节点，QExpansionItem）
        │           └── TaxonomyPositionCard.vue（岗位卡片，QCard + vuedraggable）
        └── 操作按钮（新增/编辑/删除/启停）
```

### 4.3 Kind 徽章组件

```vue
<!-- KindBadge.vue -->
<template>
  <q-badge :color="badgeColor" :label="badgeLabel" />
</template>

<script setup>
const props = defineProps<{ kind: string }>()
const kindMap = {
  system_builtin: { label: '内置', color: 'blue' },
  ecosystem_preset: { label: '预设', color: 'green' },
  marketplace: { label: '商城', color: 'purple' },
  certified: { label: '认证', color: 'orange' },
}
</script>
```

## 5. 数据迁移

### 5.1 DDL 迁移

```sql
-- system_settings 新增 ecosystem_loaded 字段
ALTER TABLE system_settings ADD COLUMN ecosystem_loaded TEXT NOT NULL DEFAULT '{}';

-- teams 新增 kind 字段
ALTER TABLE teams ADD COLUMN kind TEXT NOT NULL DEFAULT 'user';
```

### 5.2 数据迁移

```sql
-- Agent Kind 迁移
UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system';
UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template';

-- Team Kind 初始化
UPDATE teams SET kind = 'ecosystem_preset' WHERE source = 'imported';
-- 其余 Team 保持 kind = 'user'（默认值）
```

### 5.3 回滚策略

- DDL 迁移仅新增列（无破坏性），可安全回滚
- Kind 枚举回滚需反向数据迁移

## 6. 风险与缓解

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| Kind 枚举变更兼容性 | 旧代码引用已删除枚举值 | 数据迁移脚本 + 全量编译验证 |
| 部分加载失败 | 某行业加载失败影响其他行业 | 按行业独立记录状态，互不影响 |
| 卸载操作误操作 | 用户误删大量数据 | 确认对话框 + 软删除 + 重新加载能力 |
| 卸载时跨行业 Team | Team 成员分属多个行业 | 保留跨行业 Team，仅移除被卸载行业的 Agent 成员 |
| Pack 引擎 Kind 覆盖影响现有调用 | 现有 Pack 导入行为变更 | `WithKindOverride` 为可选参数，不传时行为不变 |
