# M71: Agent 资源共享 — 实现设计

> 需求文档：[71-agent-resource-sharing.md](./71-agent-resource-sharing.md)
> 关联：M67（部门主管/借调）、M23（工具系统）、M10（会话）、M53（Team DAG）、SessionRun 模块（Turn 管道）

## 一、模块概述

### 1.1 设计定位

在 biz 层新增**受控资源访问层**（ResourceAccess），为三类跨 Agent 资源访问提供统一入口：

1. **memberfs**：部门主管只读访问本部门员工（含借入成员）工作目录
2. **deptmail**：部门主管之间的异步消息信箱（含 Turn 唤醒）
3. **sessionaccess**：精灵只读检索全工作区 session 会话内容

所有访问收敛到 biz 层统一执行 **权限校验 → 范围解析 → 审计落库** 三步；工具层为薄壳（与 `read_upstream_deliverable` 同构）。员工 Agent 的 file/shell 沙箱**零改动**。

**核心原则——传递与监管分离**：跨部门信息传递只走显式通道（DeliverableRef 交付物 + 主管信箱），目录/会话访问仅为监管手段（只读 + 审计）。

### 1.2 分层与依赖

```
internal/tools/memberfs/        ← 3 个工具薄壳（list/read/search）
internal/tools/deptmail/        ← 4 个工具薄壳（send/list_inbox/read/reply）
internal/tools/sessionaccess/   ← 3 个工具薄壳（search_messages/list_sessions/read_history）
        ↓ 依赖
internal/biz/resourceaccess/    ← ResourceAccessUsecase（权限+审计）、DeptMailboxUsecase、SessionSearchUsecase
        ↓ 依赖
internal/data/                  ← dept_lead_messages / resource_access_audits 表 + 复用 SessionRepo/MessageRepo
internal/service/               ← MailboxWaker 实现（经 NativeTurnGateway 发轻量 Turn）
internal/agent/                 ← 装配：dept_lead 挂 memberfs+deptmail；spirit 挂 sessionaccess
```

依赖红线：biz 不依赖 pkg/trpc-agent-go 以外禁忌不变；工具层只依赖 biz port；service 实现 biz 定义的 `MailboxWaker` port（防 biz→service 反向依赖）。

## 二、权限模型

### 2.1 身份识别（装配期绑定，不接受工具入参声明身份）

| 身份 | 判定来源 |
|------|---------|
| 部门主管 | 调用方 Agent 是某部门的 dept_lead（M67 DeptLeadManager 创建时写入的标记，经 OrganizationUsecase 反查 `agent_id → dept`） |
| 精灵 | 调用方 Agent 为 spirit（系统内置 Agent Key） |
| 员工 | 其他 Agent（三个工具组不装配给员工，即使被调用也拒绝） |

工具在**装配期**用调用方 Agent 身份构造（闭包捕获），LLM 入参中无 caller 字段，身份不可伪造（NFR-02）。

### 2.2 目录访问关系判定（AccessPolicy）

```
CanReadMemberDir(callerAgentID, targetAgentKey) → (relation, error)
  1. caller 必须是 dept_lead，取其部门 D
  2. org_home：target 的岗位归属部门 == D → allow
  3. team_owner：target ∈ 某 Team.cross_dept_member_ids 且 Team.department_id == D
     且 Team 状态非 archived → allow
  4. 否则 → deny
```

借调结束（成员从 Team 移除 / Team 归档）后 team_owner 关系自动失效（US-03 验收）。

### 2.3 权限矩阵（集中定义于 `biz/resourceaccess/policy.go`）

| 动作 | dept_lead | spirit | 员工 |
|------|-----------|--------|------|
| memberfs.* | org_home / team_owner 只读 | ❌ | ❌ |
| deptmail.send/reply | ✅（限 dept_lead 间） | ❌ | ❌ |
| deptmail.list/read | 仅本人收件箱 | ❌ | ❌ |
| sessionaccess.* | ❌ | ✅ 只读 + 限流 | ❌ |

## 三、数据模型

### 3.1 dept_lead_messages（主管信箱）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (uuid) | 主键 |
| from_agent_id / from_dept_id | string | 发送方主管/部门 |
| to_agent_id / to_dept_id | string | 接收方主管/部门（发送时按 to_dept_id 解析当前主管） |
| subject | string(200) | 主题 |
| body | text | 正文 |
| refs_json | text, 默认 `"[]"` | DeliverableRef 引用数组 JSON（{team_id, title, …}） |
| status | string(16) | `unread` / `read` / `replied`（3 态，无需显式状态机文件） |
| reply_to_id | string, 默认 `""` | 线程：被回复消息 id |
| created_at / read_at | time / *time | |

索引：`(to_agent_id, status)`、`(from_agent_id, created_at)`。`entsql.Annotation{Table: "dept_lead_messages"}`（DB-N4）。

### 3.2 resource_access_audits（统一审计）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string (uuid) | 主键 |
| actor_agent_id / actor_role | string | 访问者 / `dept_lead`/`spirit` |
| action | string(32) | `list_files`/`read_file`/`search_files`/`send_mail`/`read_mail`/`reply_mail`/`search_messages`/`list_sessions`/`read_session` |
| target_agent_id / target_dept_id | string | 目标（可空） |
| relation | string(16) | 文件访问关系：`org_home`/`team_owner`/`none` |
| resource_uri | string(512) | 文件路径 / session_id / message_id |
| result | string(8) | `allowed` / `denied` |
| deny_reason | string(256) | 拒绝原因（可空） |
| created_at | time | |

索引：`(actor_agent_id, created_at)`、`(target_agent_id, created_at)`。只增不改（US-07）。

### 3.3 迁移

两表进 Ent Schema（L1 Auto-Migration），索引经 DDL Migration Registry 注册（DB-R4/R10，版本号 YYYYMMDD）。

## 四、工具组设计

### 4.1 memberfs（装配给 dept_lead，3 个）

| 工具 | 入参 | 行为 |
|------|------|------|
| `list_member_files` | `agent_key`, `subdir?`, `depth?`（默认 2，上限 4） | 目录树（相对路径），超深截断 |
| `read_member_file` | `agent_key`, `path`, `max_bytes?`（默认/上限 200KB） | 文本内容；二进制按魔数嗅探拒绝；超限截断 + `truncated` 标记 |
| `search_member_files` | `agent_key`, `pattern`（glob） | 匹配相对路径清单（上限 200 条） |

### 4.2 deptmail（装配给 dept_lead，4 个）

| 工具 | 入参 | 行为 |
|------|------|------|
| `send_dept_message` | `to_dept_id`, `subject`, `body`, `refs?` | 落库 → 触发唤醒（见 §6） |
| `list_inbox` | `status?`, `limit?`（默认 20） | 本人收件箱列表（含已读状态） |
| `read_message` | `message_id` | 返回全文；属本人且 unread → 置 read + read_at |
| `reply_message` | `message_id`, `body` | 线程回复（reply_to_id），原消息置 replied；同样触发唤醒 |

### 4.3 sessionaccess（装配给 spirit，3 个）

| 工具 | 入参 | 行为 |
|------|------|------|
| `search_messages` | `query`, `agent_id?`, `limit?`（默认 20） | FTS5 检索 messages_fts，返回 snippet + session_id + message_id + 时间 |
| `list_agent_sessions` | `agent_id?`, `limit?`（默认 20） | 会话元数据（标题/agent/消息数/更新时间），按更新时间倒序 |
| `read_session_history` | `session_id`, `before_message_id?`, `limit?`, `max_chars?`（上限 200000，对齐 deliverable） | 消息历史（role/content/时间），截断标记 |

**限流**：SessionSearchUsecase 内令牌桶（每 spirit Agent 20 次/分钟，内存态），超限返回明确错误（FR-11）。

## 五、Biz 层设计

### 5.1 端口（窄接口 ≤5 方法，标注 Stability）

```go
// Stability:evolving
type MemberFileReader interface {          // 由 data/service 侧实现（os 文件读取）
    List(dir, subdir string, depth int) ([]FileEntry, error)
    ReadText(dir, rel string, maxBytes int64) (string, bool, error) // content, truncated
    Search(dir, pattern string, limit int) ([]string, error)
}

// Stability:evolving
type MemberDirResolver interface {         // 由 agent 装配侧实现（复用 §5.4 布局约定）
    ResolveDir(agentKey string) (string, error)
}

// Stability:evolving
type AccessAuditor interface {             // data 层实现，fail-closed
    Record(ctx context.Context, e AuditEntry) error
}

// Stability:evolving
type MailboxWaker interface {              // service 层实现（§6）
    WakeDeptLead(ctx context.Context, agentID, hint string) error
}

// Stability:evolving
type SessionSearchReader interface {       // data 层实现（复用 SessionRepo/MessageRepo/FTS5）
    SearchMessages(ctx context.Context, q MessageQuery) ([]MessageHit, error)
    ListAgentSessions(ctx context.Context, agentID string, limit int) ([]SessionMeta, error)
    ReadSessionHistory(ctx context.Context, sessionID, beforeID string, limit, maxChars int) ([]HistoryMessage, bool, error)
}
```

### 5.2 Usecase

| Usecase | 职责 | 方法数 |
|---------|------|--------|
| `ResourceAccessUsecase` | memberfs 三方法：权限（§2.2）→ 路径解析 → 审计（含拒绝） | 3 |
| `DeptMailboxUsecase` | 发信（解析 to_dept→当前主管）/收件箱/已读/回复 + 唤醒编排 + 审计 | 4 |
| `SessionSearchUsecase` | spirit 身份校验 → 限流 → 检索/读取 → 审计 | 3 |

审计失败 → 拒绝本次访问并返回错误（fail-closed，NFR-06）。

## 六、信箱唤醒机制（US-05）

```
send_dept_message / reply_message
  → DeptMailboxUsecase：消息落库（先库后唤醒，NFR-05）
  → 防抖判定：key=(from_agent,to_agent)，内存记录 lastWakeAt
       距上次唤醒 < 5min → 跳过（消息已在收件箱，主管当前 Turn 或下次激活可见）
       否则 → MailboxWaker.WakeDeptLead(toAgentID, hint)
  → service 层实现：构造系统 TurnInput（"你收到来自 {from_dept} 主管的消息《{subject}》，
     请用 list_inbox 查收并处理"），经 NativeTurnGateway 提交为**新 Turn**
  → 不打断主管进行中的 Turn（Turn 管道自身排队语义）
```

进程重启导致防抖状态丢失：可接受（最坏多唤醒一次）。唤醒失败仅记日志，消息本体不受影响。

## 七、路径解析与安全（NFR-01）

- 布局约定复用现有：`{base}/workspace/{workspaceID}/{agentKey}`（与 `resolveAgentFilesystemDir` 一致），base 来自同一配置源（`ARANEA_WORKSPACE_ROOT`/`WORKSPACE_ROOT` env → 配置兜底）
- 防护：相对路径拼接后 `filepath.Clean` + 前缀校验（必须在目标目录内）；拒绝绝对路径、`..` 逃逸；对最终路径 `EvalSymlinks` 后再做一次前缀校验（防符号链接逃逸）
- 只读：memberfs 只暴露 list/read/search，无写接口

## 八、装配设计

| Agent | 新增工具组 | 装配点 |
|-------|-----------|--------|
| dept_lead（M67 DeptLeadManager 创建） | memberfs + deptmail | DeptLeadManager 创建主管 Agent 时写入工具配置；存量主管 Agent 迁移补旗标 |
| spirit | sessionaccess | spirit Agent 装配路径 |
| 其他 Agent | 无 | — |

工具注册：`ToolsetConfig` 新增 `MemberFS`/`DeptMail`/`SessionAccess` 三个旗标，builder 分支构造对应工具集并注入调用方身份 + biz port（Wire 绑定）。

## 九、信息流向矩阵（传递与监管分离）

| 信息流向 | 通道 | 类型 |
|---------|------|------|
| 员工 → 本部门主管 | memberfs 只读 + 既有审批流 | 监管 |
| 员工 → 同 Team 成员 | Graph StateFields / 交付物注入（已有） | 传递（已有） |
| Team → Team（跨部门） | DeliverableRef + 契约校验 + 双主管审批（M67） | 传递（已有） |
| 主管 ↔ 主管 | deptmail 信箱（可挂 DeliverableRef）+ Turn 唤醒 | 传递（新增） |
| 主管 → 借调成员 | memberfs 只读（org_home + team_owner） | 监管 |
| 精灵 → 任意 session | sessionaccess 只读检索 + 限流 | 监管 |

铁律：跨部门信息流只允许走传递通道；禁止以"读对方部门目录"代替交付。

## 十、错误处理

| 场景 | 处理 |
|------|------|
| 权限拒绝 | `CodeForbidden`（apierror 无则新增）+ 审计（result=denied） |
| 路径穿越/二进制/超限 | `CodeBadRequest` + 明确 msg；截断属正常返回（truncated=true） |
| 目标部门无主管 / 部门不存在 | `CodeNotFound` + 提示 |
| 审计写入失败 | 拒绝访问（fail-closed），`CodeInternal` |
| 唤醒失败 | 仅 Warn 日志，消息不受影响 |
| 限流超限 | `CodeBadRequest` + 重试建议 |

## 十一、影响面（实现时需同步）

| 模块 | 改动 |
|------|------|
| M67 DeptLeadManager | 主管 Agent 创建时挂 memberfs+deptmail 旗标 |
| M23 工具系统 | ToolsetConfig + builder 分支 + 注册 |
| M10 会话 | SessionSearchReader 的 data 实现（FTS5 复用/补 repo 方法） |
| SessionRun 模块 | MailboxWaker 实现（NativeTurnGateway 提交系统 TurnInput） |
| Ent Schema + DDL Registry | 两张新表 |
| `65-module-cross-reference-full.md` / `0-system-diagram.md` | 新增 M71 模块卡片（实现时同步） |

## 十二、测试策略要点

- **policy 矩阵单测**：dept_lead/spirit/员工 × 三通道 × allow/deny 全组合（含借调 team_owner 失效）
- **路径安全单测**：`..`/绝对路径/符号链接逃逸用例
- **防抖单测**：5min 窗口合并、跨发送方独立
- **截断单测**：200KB 文件 / 200K 字符会话历史边界
- **信箱集成测试**（sqlite）：发→收→已读→回复→线程状态流转
- **fail-closed 测试**：审计写失败 → 访问被拒
- **工具契约测试**：mock biz port 校验三工具组入参/出参
