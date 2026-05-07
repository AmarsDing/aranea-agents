# Hook — SQLite 表结构、编辑界面与列表页

钩子处理器支持三种：**script（ES5.1 JavaScript）**、**http**、**prompt**。

---

## 1. 概念与约束

| 项目 | 说明 |
|------|------|
| **事件 `event`** | 生命周期触发点，与下拉一致：`session_start`、`user_prompt_submit`、`pre_tool_use`、`post_tool_use`、`stop`、`subagent_start`、`subagent_stop` |
| **范围 `scope`** | `global`（全局） / `tenant`（租户） / `agent`（指定 Agent） |
| **处理器 `processor_type`** | `script` \| `http` \| `prompt`；不同类型使用不同列（见下），未使用列为 `NULL` |
| **匹配器 `matcher_regex`** | 触发前作用于**工具名**；`prompt` 类型**必填**，其余可选 |
| **条件 `condition_cel`** | 对 `tool_input` 做 CEL 求值（cel-go 语法），可选 |
| **脚本** | `handle(event)` 返回 `{ decision: "allow"\|"block", reason?: string, updatedInput?: object }`，**≤ 32 KiB**（应用层校验） |
| **HTTP** | URL、方法、请求体模板（可含 `{{.Event}}` 等占位） |
| **Prompt** | 模板 + 可选模型 + 每轮最大调用次数 |
| **超时** | `timeout_ms`；`timeout_action`：`block`（及可扩展 `allow` / `ignore` 等，与实现对齐） |

---

## 2. SQLite DDL

```sql
-- 主表：钩子定义
CREATE TABLE hooks (
  id                  TEXT PRIMARY KEY
                      DEFAULT (lower(hex(randomblob(16)))),  -- 或 INTEGER PK AUTOINCREMENT，按项目统一
  name                TEXT NOT NULL,

  event               TEXT NOT NULL
                      CHECK (event IN (
                        'session_start',
                        'user_prompt_submit',
                        'pre_tool_use',
                        'post_tool_use',
                        'stop',
                        'subagent_start',
                        'subagent_stop'
                      )),

  scope               TEXT NOT NULL
                      CHECK (scope IN ('global', 'tenant', 'agent')),

  tenant_id           TEXT,          -- scope = tenant 时必填；global/agent 可 NULL（按实现约定）

  processor_type      TEXT NOT NULL
                      CHECK (processor_type IN ('script', 'http', 'prompt')),

  matcher_regex       TEXT,          -- prompt 必填；其余可选
  condition_cel       TEXT,          -- CEL，可选

  -- script：ES5.1，≤ 32 KiB（应用层校验）
  script_source       TEXT,

  -- http
  http_url            TEXT,
  http_method         TEXT DEFAULT 'POST'
                      CHECK (http_method IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
  http_body_template  TEXT,          -- JSON 模板等

  -- prompt
  prompt_template     TEXT,
  prompt_model        TEXT,          -- 模型 id，可选
  max_calls_per_round INTEGER DEFAULT 5
                      CHECK (max_calls_per_round IS NULL OR max_calls_per_round >= 0),

  timeout_ms          INTEGER NOT NULL DEFAULT 5000
                      CHECK (timeout_ms > 0),
  timeout_action      TEXT NOT NULL DEFAULT 'block'
                      CHECK (timeout_action IN ('block', 'allow', 'ignore')),  -- 以产品为准裁剪

  priority            INTEGER NOT NULL DEFAULT 100,
  is_enabled          INTEGER NOT NULL DEFAULT 1 CHECK (is_enabled IN (0, 1)),

  created_at          TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

-- 范围 = agent 时：钩子与多 Agent 多对多
CREATE TABLE hook_agents (
  hook_id   TEXT NOT NULL REFERENCES hooks(id) ON DELETE CASCADE,
  agent_id  TEXT NOT NULL,             -- 与 agents.id 或 agent_key 对齐，按项目 FK
  PRIMARY KEY (hook_id, agent_id)
);

CREATE INDEX idx_hooks_event_scope ON hooks(event, scope);
CREATE INDEX idx_hooks_enabled_priority ON hooks(is_enabled, priority DESC);
CREATE INDEX idx_hooks_tenant ON hooks(tenant_id) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_hook_agents_agent ON hook_agents(agent_id);
```

### 2.1 类型与列填充规则

| `processor_type` | 使用列 | 其余类型专属列 |
|------------------|--------|----------------|
| `script` | `script_source` 必填 | `http_*`、`prompt_*` 为 `NULL` |
| `http` | `http_url` 必填；`http_method`、`http_body_template` 可选 | `script_source`、`prompt_*` 为 `NULL` |
| `prompt` | `prompt_template` 必填；`matcher_regex` 必填；`prompt_model`、`max_calls_per_round` 可选 | `script_source`、`http_*` 为 `NULL` |

### 2.2 `scope` 与行数据

| `scope` | `tenant_id` | `hook_agents` |
|---------|---------------|----------------|
| `global` | `NULL` | 空（表示全站） |
| `tenant` | 必填 | 可为空或表示租户内全体 Agent，由产品约定 |
| `agent` | 可 `NULL` 或租户上下文 | **至少一行**，列出适用 `agent_id` |

应用层 / CHECK（可选迁移为触发器）：`scope = 'agent'` 时插入前校验 `hook_agents` 非空。

---

## 3. 编辑 / 添加界面（与「编辑钩子」弹窗对齐）

| 区块 | 字段 | 对应列 |
|------|------|--------|
| 基础 | 名称 | `name` |
| 基础 | 事件 | `event` |
| 基础 | 范围 | `scope`、`tenant_id`（若需） |
| 基础 | Agents 多选 | `hook_agents`（仅 `scope=agent` 展示或强相关） |
| 处理器 | script / http / prompt | `processor_type` |
| 匹配 | 匹配器（正则） | `matcher_regex` |
| 匹配 | 条件（CEL） | `condition_cel` |
| script | 脚本源码 | `script_source` |
| http | URL / 方法 / 请求体模板 | `http_url`、`http_method`、`http_body_template` |
| prompt | Prompt 模板 / 模型 / 每轮最大调用次数 | `prompt_template`、`prompt_model`、`max_calls_per_round` |
| 执行 | 超时(ms) / 超时处理 / 优先级 / 启用 | `timeout_ms`、`timeout_action`、`priority`、`is_enabled` |

底部：**取消**、**保存**（写 `hooks` + 同步 `hook_agents`）。

---

## 4. 列表页界面设计

**用途**：在 Agent 设置「钩子」Tab 或全局「钩子管理」中展示可排序、可启停的钩子列表。

### 4.1 表格列（建议）

| 列 | 数据来源 | 说明 |
|----|----------|------|
| **启用** | `is_enabled` | 行内 `QToggle`，PATCH 即时生效，二次确认 |
| **名称** | `name` | 主键跳转编辑 |
| **事件** | `event` | 短标签，如 `pre_tool_use` |
| **范围** | `scope` + 摘要 | `agent` 时显示 Agent 数量或首条名称 |
| **类型** | `processor_type` | 徽标：`script` / `http` / `prompt` |
| **匹配摘要** | `matcher_regex` 截断 + `condition_cel` 可选图标 | 悬停全文 |
| **动作摘要** | 按类型 | script 首行代码；http 显示 host；prompt 首行模板 |
| **优先级** | `priority` | 数字，支持列排序 |
| **操作** | — | **编辑**（打开同一弹窗）、**删除**（确认）、 **复制** |

### 4.2 交互与筛选

- 顶部：**搜索**（名称 / 事件 / 类型）、**事件**筛选、`scope` 筛选、**仅启用**开关。
- 排序：默认 `priority DESC`，其次 `name ASC`。
- 空态：无钩子时的说明 + **添加钩子**。

### 4.3 API 示意（非 SQLite，仅供前后端对齐）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/hooks` | 列表 + 筛选 + 分页 |
| GET | `/hooks/:id` | 详情（含 `agent_ids[]`） |
| POST | `/hooks` | 创建 |
| PATCH | `/hooks/:id` | 部分更新 |
| DELETE | `/hooks/:id` | 删除 |
| PATCH | `/hooks/:id/agents` | 仅更新 `hook_agents`（可选） |

---

