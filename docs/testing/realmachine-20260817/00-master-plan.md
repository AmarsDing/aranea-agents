# Aranea 真机功能测试主计划（2026-08-17）

> 测试形态：真机全链路（Docker 部署版 aranea-admin :8810/:9910/:8812 + TwinMonitor 全栈 + GNS3 仿真）
> 账号：dev/dev（workspace=default）| 执行：AI 无人值守
> 原则：①同模块用例一次对话内批量执行 ②每模块测完清理/归档日志 ③围绕监控场景 ④每用例有结果+原因分析+解决方案

## 环境基线

| 组件 | 地址 | 状态 |
|------|------|------|
| aranea-admin (HTTP) | http://localhost:8810 | ✅ /healthz ok |
| aranea-admin (gRPC/WS) | 9910 / 8812 | 待验 |
| aranea-postgres / redis | 容器内网 | ✅ healthy |
| TwinMonitor Gateway | http://localhost:8000 | ✅ 200 |
| GNS3 Agent | http://localhost:18081 | ✅ LISTENING |
| TwinMonitor 全栈 | 7788/80xx/81xx/90xx | ✅ 25h up |

## 模块与测试范围

| 文件夹 | 模块 | 核心用例数 | 优先级 |
|--------|------|-----------|--------|
| 00-env-readiness | 环境就绪 | 容器/端口/健康/登录/DB 迁移 | P0 |
| 01-agent-mgmt | Agent 管理 | 列表/详情/创建/更新/删除/收藏/prompt 预览/effective tools | P0 |
| 02-chat-session | 对话会话 | 发消息/流式/排队/取消/会话 CRUD/历史 | P0 |
| 03-team-orchestration | Team 编排 | 列表/创建/运行/步骤/取消/汇总 | P1 |
| 04-graph-orchestration | Graph 编排 | 图列表/执行/检查点/恢复/失败策略 | P1 |
| 05-spirit | Spirit 动态编排 | 精灵对话/任务分解/团队组装/综合 | P1 |
| 06-memory | 五层记忆 | L0-L4 状态/召回/worker/死信 | P0 |
| 07-knowledge | 知识库 | 词条/检索/写回/chunk 重放 | P1 |
| 08-tools-twinops | 工具/TwinOps | 工具清单/授权/有效工具/twinops 连通 | P0 |
| 09-mcp | MCP | server CRUD/探测/热加载 | P1 |
| 10-monitor-scenario | 监控运维闭环 | 告警→诊断→fault_inject/clear→复核（HITL）| P0★ |
| 11-observability | 可观测性 | trace/flowlog/根因/自愈/审计/runner 指标 | P0 |
| 12-usage-quota | 额度成本 | 用量总览/趋势/配额/预算告警 | P1 |
| 13-cron | 定时任务 | cron 列表/worker 状态 | P2 |
| 14-skill | 技能 | 列表/加载/进化提案/健康 | P2 |
| 15-provider-model | Provider/模型 | 模型目录/定价/校验 | P1 |
| 16-hook-webhook | 钩子 | hook CRUD/投递记录 | P2 |
| 17-frontend-ui | 前端 | 构建产物/页面可达（dev 或静态） | P2 |
| 18-performance | 性能 | 并发对话延迟/接口耗时/资源占用 | P1 |
| 19-cli | CLI | login/agent ls/chat/monitor | P2 |

## 测试产出约定

- 每模块文件夹：`cases.md`（用例）、`result.md`（结果+原因分析+解决方案）、`evidence/`（原始输出）
- 结果分级：PASS / FAIL / BLOCKED（环境缺）/ SKIP（需外部凭证）
- 日志纪律：每模块结束后归档 `docker/volumes/logs` 关键片段至 evidence，再截断容器日志（不删文件）
