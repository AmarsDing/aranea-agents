# 18 Monitor / Dashboard Review

> **评分**：78 / 100 | **风险等级**：P1  
> **文档**：[18 monitor.md](../需求/18%20monitor.md) · [18 monitor.design.md](../需求/18%20monitor.design.md) · [18-monitor-development.md](../需求/18-monitor-development.md) · [18 monitor-dashboard.md](../需求/18%20monitor-dashboard.md) · [18-monitor-dashboard-development.md](../需求/18-monitor-dashboard-development.md)  
> **代码锚点**：`internal/service/monitor.go` · `internal/biz/audit_record.go` · `internal/biz/event_store.go` · `web/src/pages/MonitorPage.vue` · `web/src/components/monitor/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 6 Tab 运维页 + Dashboard 概览均已落地；latency 聚合 / Phase 4 自动刷新 / Grafana 待补 |
| 架构一致性 | 21 | 25 | Monitor 正确消费 EventBus；LogStreamHub 共享单 WS ✅；Monitor 方案 C（Runs+Events）✅ |
| 后端实现质量 | 17 | 20 | audit_logs / monitor_events / runner.completion 落库 ✅；告警 Webhook/Channel + 冷却 ✅ |
| 前端实现质量 | 13 | 15 | 6 Tab 完整 ✅；`useMonitorPage.ts` composable ✅；ECharts 趋势 ✅ |
| 测试与验证 | 5 | 10 | Monitor 各 Tab 功能手动验证；自动测试薄弱 |
| 文档一致性 | 6 | 10 | 三件套 + Dashboard 三件套对齐；Usage Tab 去重、文件树与 Dashboard 分工 2026-05-21 已更新 |

---

## 双文档结构

| 文档组 | 范围 |
|--------|------|
| `18 monitor.md` + `18-monitor-development.md` | 运维页 `/monitor/logs`（6 Tab）|
| `18 monitor-dashboard.md` + `18-monitor-dashboard-development.md` | 概览 Dashboard `/overview` |

**注意**：同一模块编号 18 覆盖两个页面，是已知的文档结构特例。

---

## 运维页 6 Tab 状态

| Tab | 功能 | 状态 |
|-----|------|------|
| Usage | Runner 指标 + 跳转概览/明细 | ✅ |
| Alerts | 告警规则 + Webhook/Channel + 冷却 | ✅ MON-01 |
| Audit | 审计日志表（Tool/MCP/Agent CRUD）| ✅ |
| Events | 实时/持久化 Monitor 事件 | ✅ |
| Traces | LLM Trace 列表 + 瀑布图 + Span 树 | ✅ I6-TEL-02 |
| Logs | 流程日志（flow_log）+ 进程日志 | ✅ Phase 2 落库 + Logs Tab |

---

## Dashboard `/overview` 状态

| 组件 | 状态 |
|------|------|
| 指标卡（月预算使用率）| ✅ |
| ECharts 趋势（`UsageTrendChart`）| ✅ |
| Provider/模型占比（`UsageBreakdownCharts`）| ✅ |
| Top 模型/Agent | ✅ |
| 异常列表 | ✅ |
| Runner 条（`OverviewRunnerMetrics`）| ✅ I5-MON-01 |
| 快捷入口（`OverviewMonitorQuickLinks`）| ✅ |
| ECharts 趋势时间粒度（按天/按小时）| ✅ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| MON-P1-01 | `MonitorPage.vue` 直连 `features/monitor/api`，违反 Page→composable/store 分层规范 | 引入 `useMonitorPage.ts` composable |
| MON-P1-02 | FlowLogger Phase 2（落库历史查询）缺失，Monitor Logs Tab 只有实时流 | 见 [52-flowlogger-review.md](./52-flowlogger-review.md) |
| MON-P1-03 | 进程日志由 `server.monitor.process_log_enabled` 控制，文档化不足 | 在 Monitor 文档中明确配置项名称和默认值 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| MON-P2-01 | latency 聚合（全局/Agent 级延迟分布）未实现 | 规划 latency 聚合 API + 展示 |
| MON-P2-02 | Dashboard 自动刷新（Phase 4）未实现 | 规划前端轮询或 WS 推送刷新 |
| MON-P2-03 | Grafana dashboard 导出文件存在但与后端 Prometheus 指标路径的对齐需要验证 | 定期验证 Grafana JSON 与实际 metrics 路径 |
| MON-P2-04 | Monitor 告警多通道（Channel 下拉）正确性：Channel 配置与 Alert rule 绑定的 UX 待优化 | 补 Alert Channel 配置测试 |

---

## 建议优化路径

1. ~~`useMonitorPage.ts` composable~~ ✅
2. ~~FlowLogger Phase 2 落库~~ ✅
3. ~~latency `avg_duration_ms` + 30s 自动刷新~~ ✅ Phase C
4. **下迭代**：Grafana JSON 与 metrics 路径对齐；Alert Channel UX 测试
