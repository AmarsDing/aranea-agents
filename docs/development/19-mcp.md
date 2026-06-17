# MCP 服务器管理

> 需求文档。设计见 [19-mcp.design.md](./19-mcp.design.md)，开发计划见 [19-mcp.development.md](./19-mcp.development.md)。

本文档描述 **Model Context Protocol（MCP）服务器** 在控制台中管理的 **用户故事、功能需求、验收标准与非功能需求**（用户视角）。前端采用 **Quasar（Vue 3）**，与 Monitor 控制台风格一致。

> 实现进度/状态、代码锚点见 [19-mcp.development.md](./19-mcp.development.md)；架构、Proto/API 契约、数据模型、UX 规范、前端组件设计见 [19-mcp.design.md](./19-mcp.design.md)。

---

## 1. 用户故事

- **作为平台管理员**，我希望注册外部 MCP 服务器（stdio / SSE / Streamable HTTP），以便 Agent 可以挂载并调用其工具。
- **作为平台管理员**，我希望在列表页查看所有 MCP 服务器的健康状态（状态灯），以便快速识别异常。
- **作为平台管理员**，我希望在添加/编辑表单中测试连接，以便在保存前验证配置是否可用。
- **作为平台管理员**，我希望对启用了 `require_user_credentials` 的服务器配置我自己的凭据，以便 Agent 代表我调用工具时使用我的身份。
- **作为平台管理员**，我希望对 URL 进行预检（不持久化），以便在表单填写阶段就发现 SSRF / 网络问题。

---

## 2. 功能需求

### 2.1 列表页

| 需求 | 说明 |
|------|------|
| 列表展示 | 展示所有 MCP 服务器（每项一个 MCP 组件），含名称、传输类型、地址/命令、工具前缀、超时、启用状态 |
| 搜索 | 按 `name`、`display_name` 过滤 |
| 状态灯 | 灰=未检测、绿=健康、红=失败、黄=降级；Tooltip 展示最近错误或成功时间 |
| 空态 | 中央插头图标 + 主文案「暂无 MCP 服务器」+ 副文案 + 添加按钮 |
| 刷新 | 手动刷新列表 |
| 添加入口 | 右上「+ 添加服务器」按钮 |

### 2.2 CRUD

| 操作 | 行为 |
|------|------|
| 创建 | 打开对话框表单，`key` + `name` 必填 |
| 编辑 | 打开对话框表单预填 |
| 删除 | 确认对话框：「删除后依赖该服务器的工具将不可用」；软删除 |
| 测试连接 | 探活并写入健康元数据，结果 Notify 提示 |

### 2.3 添加/编辑表单字段

| 字段（逻辑名） | 必填 | 校验 / 说明 |
|----------------|------|-------------|
| `name`（表单）→ API `key` | 是 | Slug：仅小写字母、数字、连字符；平台内唯一 |
| `display_name`（表单）→ API `name` | 否 | 展示用 |
| `transport` | 是 | `stdio` \| `sse` \| `streamable_http` |
| `url` | `sse`/`streamable_http` 必填 | HTTP(S) URL；服务端 SSRF 校验 |
| `command` / `args` | `stdio` 必填 | 可执行路径与参数 |
| `headers` | 否 | 动态键值行 |
| `env` | 否 | 动态键值行 |
| `tool_prefix` | 否 | 空则服务端从 `name` 派生 |
| `timeout_sec` | 否 | 默认 60 |
| `enabled` | 否 | 默认开 |
| `require_user_credentials` | 否 | 为真时需用户级凭据 |
| `probe_mode` | 否 | `connectivity`（默认）\| `auth_aware` |

**传输与字段显隐**：`streamable_http`/`sse` 展示 URL；`stdio` 展示 command/args。

> 字段控件映射（Quasar 组件）、表单对话框 UX 细节见设计文档 §10.3。

### 2.4 用户凭据管理

| 需求 | 说明 |
|------|------|
| 列出凭据 | 列出当前用户在某 MCP 服务器上的凭据 |
| 新增/更新凭据 | 提交 `credential_key` + `secret`，密钥加密存储 |
| 删除凭据 | 按 `credential_key` 删除 |
| 触发条件 | 仅 `require_user_credentials=true` 的服务器需要 |

### 2.5 URL 预检

| 需求 | 说明 |
|------|------|
| 预检 API | `POST /v1/mcp-servers/validate`，不持久化，仅校验连通性 |
| 表单集成 | 表单填写阶段可触发预检，失败时保留错误在对话框内 |

---

## 3. 非功能需求

- **SSRF 防护**：对 `url` 做解析与白名单/内网限制；错误信息不泄露内网拓扑。
- **敏感头保护**：`Authorization`、`token` 等不落日志明文；列表接口可对 `headers` 脱敏。
- **stdio 注入防护**：校验 `command` 路径与参数，防止注入。
- **健康可观测**：后台定时探活更新健康元数据；持续错误触发告警。

---

## 4. 验收标准

- [x] 列表：搜索、状态灯、空态、刷新、添加入口齐全。
- [x] CRUD：创建、编辑、删除确认；`name` slug 校验。
- [x] 传输切换：`url` vs `command`/`args` 显隐正确。
- [x] 测试连接与错误文案（含 SSRF/网络失败）可感知。
- [x] `require_user_credentials` 为真时有用户凭据入口。
- [x] URL 预检 API 可用。
- [x] MCP 调用统计闭环。
- [x] 健康持续告警。

---

## 5. 参考截图（本地）

线稿与空态、添加对话框可参考工作区 `assets/` 下对应 `image-*.png`。

---

## 6. 相关文档

- 设计文档：[19-mcp.design.md](./19-mcp.design.md) — 架构、Proto/API 契约、数据模型、UX 规范、前端组件设计
- 开发计划：[19-mcp.development.md](./19-mcp.development.md) — 代码锚点、现状评估、任务清单、进度状态

---

*文档版本：3.0 — 2026-06-17 按三件套边界重组：UX 规范/数据模型/API 契约/实现状态迁至设计与开发文档。*
