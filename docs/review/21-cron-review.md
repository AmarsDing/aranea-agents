# 21 Cron Review

> **评分**：82 / 100 | **风险等级**：P2  
> **文档**：[21-cron-development.md](../需求/21-cron-development.md)  
> **代码锚点**：`internal/cronrunner/` · `internal/service/cron.go` · `internal/biz/cron.go` · `internal/cronrunner/jobs/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | 定时触发 Agent/Team + 执行记录 + RecordSkipped 统一 ✅；高级 Cron 语法/多时区较弱 |
| 架构一致性 | 22 | 25 | `cronrunner` 独立运行时 ✅；经 `RunGateway` 进入 Chat 主链路 ✅；jobs 层次清晰 |
| 后端实现质量 | 18 | 20 | `execute.go` + `retry.go` + `auto_memory` 等内置 job ✅；RecordSkipped 统一 ✅ |
| 前端实现质量 | 13 | 15 | `/cron` + `/cron/runs` 两页 ✅；Cron 表达式 + 绑定 Agent + 启用状态管理 ✅ |
| 测试与验证 | 6 | 10 | `execute_test.go`、`retry_test.go` ✅；内置 job 单测待补 |
| 文档一致性 | 6 | 10 | `21-cron-development.md` 对齐；RecordSkipped 统一 changelog 已同步 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Cron 任务 CRUD | ✅ |
| Cron 表达式（5 段）| ✅ |
| 绑定 Agent/Team | ✅ |
| 启用/禁用 | ✅ |
| 执行历史（`cron_task_run`）| ✅ |
| 最近执行摘要 | ✅ |
| RecordSkipped 统一 | ✅ changelog |
| 内置 Job：`auto_memory` | ✅ |
| 内置 Job：其他（错误重试等）| ✅ |
| 多时区支持 | 🟡 |
| Cron 表达式可视化编辑器 | 🟡 |

---

## 主要风险

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| CRON-P2-01 | 内置 job（`auto_memory` 等）无单测 | 补内置 job 单测 |
| CRON-P2-02 | Cron 任务在进程重启时若正在执行，恢复/重试策略不明确 | 文档化重启策略；补重启恢复测试 |
| CRON-P2-03 | 多时区支持（UTC + 用户本地时区）仅部分实现 | 规划时区配置功能 |

---

## 内置 Job 类型

| Job | 功能 | 状态 |
|-----|------|------|
| `auto_memory` | 自动写入 L4 记忆图 | ✅ |
| 其他调度 job | 错误重试、清理等 | ✅ |

---

## 建议优化路径

1. 补内置 job 单测（P2）。
2. 文档化重启恢复策略（P2）。
3. 规划多时区支持（P2）。
