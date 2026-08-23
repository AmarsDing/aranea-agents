# 会话端到端测试方案（chat-e2e-20260823）

> 目标：按 `1-chat.md` / `1-chat.design.md` / `10-session.md` 设计，**真实发送消息**验证会话能否完成指定任务，并定位问题。
> 状态：**待评审** —— 评审通过后进入执行阶段。
> 前置结论：2026-08-17 realmachine 已验证 API CRUD 层（10 PASS / 1 FAIL，删除 BUG 已修复）。本次重点不在 CRUD，而在**任务完成能力**（三种对话模式 + 工具调用 + 控制面）。

---

## 一、测试环境

| 项 | 值 | 说明 |
|----|----|------|
| 后端 | Docker `aranea-admin`，HTTP `127.0.0.1:8810` / WS `127.0.0.1:8812` | 已确认 /healthz 200 |
| 前端 | quasar dev `http://127.0.0.1:9301`（代理 8810） | UI 阶段使用；若 9301 被占自动 +1，先确认实际端口 |
| 账号 | `dev / dev`（既有测试资产） | |
| 模型 | deepseek / deepseek-v4-flash（全量 agent 已统一） | |
| 证据目录 | `f:\myproject\test\chat-e2e-20260823\` | 响应体 / WS 事件流 / 日志片段全部落盘 |

## 二、测试对象选择策略

| 模式 | 对话对象 | 选择方式 |
|------|---------|---------|
| 模式 A（Simple） | 单个工具面明确的 agent（系统内置，如运维/开发岗） | 执行阶段先查 DB `agent_runtime_settings.tools_enabled` 选定 1 个 |
| 模式 B（Sub-Agent） | Spirit 主会话（`__spirit__`） | 多步骤任务触发拆解 |
| 模式 C（Team） | Spirit 组建 Team / 既有 Team 会话 | 复杂任务触发 graph_stage |

## 三、测试用例（P0→P5 顺序执行，逐级放行）

### P0 环境就绪（不通过则不进入后续）

| ID | 用例 | 操作 | 预期判定 |
|----|------|------|---------|
| ENV-01 | 后端健康 | `GET /healthz` | 200 且 body 含 ws_path/auth 标记 |
| ENV-02 | 登录 | `POST /v1/admins/login` dev/dev | 200 + token |
| ENV-03 | WS 握手 | 连 `ws://127.0.0.1:8812/v1/ws?session_id=...` | 收 `connected` 事件 |

### P1 基础对话链路（会话"能对话"）

| ID | 用例 | 输入 | 预期判定 |
|----|------|------|---------|
| BASIC-01 | 创建会话 | `POST /v1/sessions`（owner=agent） | 200 + sid |
| BASIC-02 | 单轮问答（HTTP unary） | "你好，请用一句话介绍你自己" | 200；`sessions/{id}/messages` 落库 user+assistant 各 1 条 |
| BASIC-03 | 多轮上下文 | ①"记住数字 42，只回复'已记住'" ②"我刚才让你记的数字是？" | 第 2 轮回复含 `42` |
| BASIC-04 | WS 流式事件 | WS 上行 `user_message` | 事件序列完整：`turn(created)` → `step(streaming, delta 递增)` → `step(completed)` → `turn(completed)`；无乱序/丢失 |
| BASIC-05 | 自动标题 | 首轮结束后查 session.title | 非空且非默认值（异步生成，轮询 ≤30s） |

### P2 任务完成能力（核心：会话"能完成指定任务"）

| ID | 用例 | 输入（任务指令） | 预期判定 |
|----|------|----------------|---------|
| TASK-A1 | 模式A · 简单推理 | "3+5 等于几？只回答数字" | 回复含 `8`；事件流无 action（纯 LLM） |
| TASK-A2 | 模式A · 工具调用 | 按对象工具面选任务（如"查一下当前时间"/文件/搜索类） | 事件流含 `action`（工具卡片，completed + 耗时）；最终回复内容与工具结果一致 |
| TASK-A3 | 指令遵循 · 结构化输出 | "用合法 JSON 返回苹果的三个优点，键为 pros，只输出 JSON" | 回复可 `JSON.parse` 且 `pros.length==3` |
| TASK-B | 模式B · 子 Agent 委派 | 向 Spirit 发明确多步骤任务（如"分三步分析 X，每步交给一个子代理"） | 事件流出现 `plan` + ≥2 个 `session`（子 Agent）Activity；子 session 可独立查询；父级 conclusion 汇总子结果 |
| TASK-C | 模式C · Team 编排 | 向 Spirit 发复杂任务触发组建 Team | 事件流出现 `graph_stage`→`team_stage`→`session` 三层嵌套；最终 conclusion |
| TASK-D | 技能/知识增强（可选） | 问知识库已有内容（如 AWOS 运维知识） | 回复命中知识内容，事件含 skill/knowledge 调用痕迹 |

> TASK-B/C 依赖 Spirit 编排决策，若 LLM 未触发对应模式，记录实际模式并分析 prompt/编排条件，不判 FAIL 而判「模式未触发」待分析。

### P3 控制面

| ID | 用例 | 操作 | 预期判定 |
|----|------|------|---------|
| CTRL-01 | 停止生成 | 发长文任务，流式中途 `POST /v1/chat/stop` | run 状态 → cancelled/interrupted；WS 收终止事件；无后续 delta |
| CTRL-02 | Follow-up 队列 | 生成中连发第 2 条（enqueue） | 入队 pending；当前 turn 完成后自动执行；顺序正确 |
| CTRL-03 | 运行状态 | `GET /v1/chat/run-status` | 200 + status 与实际一致 |

### P4 会话管理

| ID | 用例 | 操作 | 预期判定 |
|----|------|------|---------|
| MGMT-01 | 三视图一致 | messages / turns / timeline | 200 且数量/内容互洽 |
| MGMT-02 | 管理操作 | 检索 query / PATCH 标题 / pin·unpin | 全部 200 生效 |
| MGMT-03 | 上下文比例 | 查 session.context_used_ratio | >0 且与消息量匹配 |
| MGMT-04 | 删除会话 | `DELETE /v1/sessions/{id}` | 200（**回归验证** 2026-08-17 BUG-02 级联修复） |

### P5 UI 端到端（后置，agent-browser 独立 session `chat-e2e-0823`）

| ID | 用例 | 操作 | 预期判定 |
|----|------|------|---------|
| UI-01 | 浏览器发消息 | 9301 登录 → 聊天页发送 → 观察渲染 | 流式渲染正常；工具卡片显示能力名+耗时+状态色；默认折叠 |
| UI-02 | 刷新恢复 | F5 刷新 | 历史消息/卡片完整恢复（v2 REST hydrate） |

## 四、执行方式

1. **API 层**：PowerShell 脚本（`curl.exe`，非 PS curl 别名）+ Node WS 客户端脚本（node 环境已具备），统一放 `f:\myproject\test\chat-e2e-20260823\`，响应体/事件流落盘。
2. **日志排查路径**：HTTP 错误码 → 响应 body → `docker logs aranea-admin` → 容器内 `logs/aranea-pipeline.log`（K1-K7 关键节点：turn 入口/工具调用/错误）。
3. **UI 层**：`agent-browser --session chat-e2e-0823`（独立 profile，不与 mcp_playwright 混用）；中文一律 Unicode 转义注入；结果重定向文件后用 Read 工具读。
4. **报告产出**：`docs/testing/chat-e2e-20260823/result.md`（用例 × 结果 × 证据链接 × 问题清单）。

## 五、问题分级与处置

| 级别 | 定义 | 处置 |
|------|------|------|
| P1 | 主链路阻断（发不出消息/无回复/500） | 立即排查根因，整理证据向你请示后再修 |
| P2 | 功能不符设计（模式未触发/事件缺失/队列失效） | 记录复现步骤，分析后逐项请示 |
| P3 | 体验/边界问题 | 记录归档 |

**纪律**：发现框架（vendored trpc-agent-go）疑似 bug 时，只整理信息请示，不擅改（FW-R1~R3）。

## 六、待你确认的 3 个点

1. **范围**：P2 三种模式全测，还是本轮先聚焦模式 A（单 Agent + 工具）+ 基础链路，B/C 下轮再测？（建议：全测，但 B/C 允许「模式未触发」作为分析项）
2. **"指定的任务"**：你心中是否有具体任务场景（如 TwinMonitor 运维指令、知识库问答）？有的话告诉我，我把它固化成 P2 的 TASK 用例。
3. **UI 阶段**：P5 是否本轮必做，还是 API 层全绿后再补？
