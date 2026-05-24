# Review：Channel 外部参考借鉴手册

> **Review 日期**：2026-05-24  
> **对象文档**：[17-channel-external-reference-playbook.md](../需求/17-channel-external-reference-playbook.md)  
> **方法**：对照 `docs/README.md` 文档组织规则、M17/M55 现有文档链、代码锚点可达性、任务卡可执行性

---

## 1. 结论摘要

| 维度 | 评分 | 说明 |
|------|------|------|
| 文档组织与路由 | **9/10** | 已接入 `docs/README.md`、`README-development.md`、`17-channel-development.md §13`；与 Phase F（Hermes）职责分离清晰 |
| 内容完整性 | **9/10** | 覆盖 GoClaw + OpenClaw 双源、边界、对照表、14 项任务卡、验证命令 |
| 与代码一致性 | **8/10** | 锚点文件均存在；CH-BOR 均为 📋 未实现，与「参考手册」定位一致 |
| 架构红线符合度 | **10/10** | 明确禁止 biz import trpc、禁止 OpenClaw app 整包替换、保留 NativeTurnGateway |
| 可执行性 | **8/10** | P0 任务验收可测；P2/P3 需与 M55/Memory 排期对齐后再细化指标 |

**总评**：✅ **通过，可作为 Channel/Chat 外部借鉴的权威入口文档。**

**风险等级**：P3（文档级，无代码变更）

---

## 2. 优点

1. **双源合一**：GoClaw（调度/IM 工程）与 OpenClaw（接口/Gateway 模式）放在同一手册，避免 AI 只读其一而漏项。
2. **边界清晰**：§3「不建议照搬」与 Aranea 已有优势（revision、双 Bus、Ent session）对齐，降低误用 OpenClaw app 的概率。
3. **与 Phase F 分工**：Hermes = 飞书平台特化；Phase G / CH-BOR = 跨平台调度与网关模式，避免与 §11 重复。
4. **任务卡可追踪**：`CH-BOR-01`–`14` 带落点与验收，符合 M55 / execution-plan 任务 ID 习惯。
5. **路由完整**：从 AI 入口 `docs/README.md`、模块索引、Channel 开发计划、M55 方案、系统标杆均可一跳到达。

---

## 3. 待改进项（非阻塞）

| ID | 级别 | 项 | 建议 |
|----|------|-----|------|
| R-01 | P2 | GoClaw 代码路径为外部仓库 | 在 playbook 或 0-system-development 注明「GoClaw 非 vendored，对照只读」；实施 CH-BOR 时以本文任务卡为准，不依赖外部 repo 常驻 |
| R-02 | P2 | CH-BOR-02 群聊并发上限 | 实施前在 `17 channel.design.md` 补配置字段名（如 `session_max_concurrent`）避免与 M55 admission 字段冲突 |
| R-03 | P3 | CH-BOR-03 steer 语义 | 与 trpc-agent-go 是否支持 mid-run steer 对齐；若不支持，任务卡应降级为「queue + 合并上下文」 |
| R-04 | P3 | 55-chat-channel-cursor-solution §8 表格 typo | 修复 `\`n\` 损坏行（本次路由更新一并修复） |

---

## 4. 与现有实现差距核对

| 借鉴项 | 文档声称现状 | 代码核对 | 一致 |
|--------|-------------|----------|------|
| interrupt 模式 | ✅ | `channel_config_helpers.go` · `busy_input_mode` | ✅ |
| followup 合并 | ❌ | `PendingMessageQueue` 无 merge 逻辑 | ✅ |
| NativeTurnGateway | ✅ | `internal/biz/turn_input.go` · `chat_native.go` | ✅ |
| TurnPreview EventBus | ✅ | `channel_turn_preview.go` | ✅ |
| session_revision | ✅ | `internal/event/session_revision.go` | ✅ |
| OpenClaw surface 对齐 | ✅ | `internal/channel/surface.go` 注释 | ✅ |
| Provider taxonomy | 字符串匹配为主 | CH-BOR-09 | ✅ |
| OutboundMeta / local_key | 散落特判 | CH-BOR-10 | ✅ |
| Context admission | 无 | CH-BOR-11 | ✅ |
| Stream sanitize | thinking 泄漏 | CH-BOR-12 | ✅ |
| Lane scheduler | per-session 为主 | CH-BOR-13 | ✅ |
| Durable compact hook | Memory 有、Turn 前未统一 | CH-BOR-14 | ✅ |

---

## 5. 建议下一步

1. **DECO-01 手工 E2E**：飞书 Turn + Web 同 session revision hydrate（M55-SYNC-01/02）。
2. **Lane 压测**：Cron/Team 高负载下 Channel 响应延迟（CH-BOR-13 验收补全）。
3. **changelog**：重大 Channel 发布时摘要写入 `docs/changelog/`，不在 playbook 正文堆修复记录。

---

## 6. Review 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-24 | 初版 review，文档发布日 |
| 1.1 | 2026-05-24 | Phase G-c（CH-BOR-10–14）落地；设计回写 §5.3.1 |
