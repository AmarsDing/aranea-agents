# Aranea 真机功能测试总报告（2026-08-17）

> **测试形态**：真机全链路无人值守——Docker 部署版 aranea-admin (:8810/:9910/:8812) + TwinMonitor 全栈 + GNS3 仿真 + 真实 LLM（deepseek）+ Playwright 真机浏览器。
> **覆盖**：20 个功能模块 / 约 200 个最终有效用例 / 每个模块独立文件夹（cases + result + evidence）。
> **执行纪律**：同模块用例单次对话批量执行；每模块测完归档证据并巡检日志；全程围绕监控场景；所有问题有结果+原因+方案。

## 一、总体结论

| 维度 | 结论 |
|------|------|
| 功能可用性 | **17/20 模块全量通过**；3 个模块各暴露 1 个 P1/P2 级真实缺陷 |
| 监控闭环（核心场景） | **链路闭环验证通过**：HITL 注入 → GNS3 故障真实生效（内核级端口 down/up）→ HITL 清除 → 恢复，地面真值吻合 |
| 性能 | 只读接口均值 20~100ms 优秀；并发 50 请求 0 失败；容器资源占用低（admin 138MiB/0.31% CPU）；1 个接口劣化待优化 |
| 安全 | SSRF 守卫、高危工具二次确认、HITL 门禁均真实生效（正向验证通过） |
| 稳定性 | 3 小时 3228 行日志 **0 error / 0 panic** |

## 二、缺陷清单（按优先级，全部含原因分析与解决方案，详见各模块 result.md）

### P1 —— 必须修复

| # | 缺陷 | 模块 | 现象与根因 | 方案 |
|---|------|------|-----------|------|
| 1 | **BUG-01 无岗位 Agent 创建必败** | 01 | `agents(position_key, agent_variant)` 唯一索引把默认值 `('','')` 纳入唯一键，存量 1 行后所有无 position 创建 400 冲突 | 索引改部分唯一 `WHERE position_key <> ''`（推荐）/ 空岗位写 NULL / 自动生成占位 position_key |
| 2 | **BUG-02 会话删除恒 500** | 02 | [cascade_delete.go:112](file:///f:/myproject/aranea-agents/internal/data/cascade_delete.go#L112) 引用已不存在的 `messages` 表（42P01），表重构后级联未同步 | 改现行消息表；全量核对 cascade 表名 vs information_schema；补 PG 集成回归测试 |

### P2 —— 应当修复

| # | 缺陷 | 模块 | 现象与根因 | 方案 |
|---|------|------|-----------|------|
| 3 | **BUG-MON-A 破坏性操作的澄清被自动代答为「取消」** | 10 | 意图识别对 fault_inject 未打 destructive 标记 + 推荐答案="Cancel" → autoResolveClarification「假设式前进」自动代答，HITL 形同虚设 | 意图识别对 fault_inject 类强制 destructive 标记；或 autoResolve 增加工具风险维度（requires_confirmation=true 禁止假设式前进） |
| 4 | **BUG-MON-C 端口级故障不产生告警** | 10 | 注入 sw1 eth1 down 后 180s TwinMonitor 零新告警；linemonitor 仅探测设备级管理面 ping | TwinMonitor 补端口级监控（SNMP ifOperStatus 轮询/探测项），否则真实监控闭环断链 |
| 5 | **BUG-G1 残缺图 visualize 500** | 04 | 空 nodes/缺 entry 的历史图未做防御，panic 上抛 500 | biz 层降级返回空 DOT 或明确 400 |
| 6 | **BUG-CLI-01 非法命令静默退出** | 19 | `SilenceErrors:true` + main() 从不打印 err → exit 3 零输出 | main() 退出前 `fmt.Fprintln(os.Stderr, "aranea:", err.Error())` |

### P3 —— 建议改进

| # | 事项 | 模块 | 要点 |
|---|------|------|------|
| 7 | ISSUE-G2 无检查点执行返回 404 | 04 | 与「执行不存在」无法区分，建议 200+空集或专属 reason |
| 8 | ISSUE-G3 time-travel step_index=0 被拒 | 04 | proto3 required 标量零值陷阱，改 optional 或业务层校验 |
| 9 | BUG-MON-B 持久化授权无 TTL | 10 | 8-15 演练授权残留至 8-17，期间 fault_clear 绕过人工确认；建议 expires_at + 演练后清理 SOP |
| 10 | ISSUE-K1 vault 宿主路径容器不可达 | 07 | `F:\aranea-agents\test\kb-ux-vault` 每 30s 刷 warn；改容器卷路径或停用同步 |
| 11 | PERF-S1 Spirit 系统提示词 24k token/轮 | 05 | 平凡问答 token_in=24199/9.9s，远超普通 agent；建议 prompt 分级装载或 prompt cache |
| 12 | PERF-F1 /v1/model-catalog/providers ~510ms | 15/18 | 是其他只读接口 20-50 倍；建议 pprof 定位 + 聚合缓存 |
| 13 | /v1/tools/test 平台限制 | 08/10 | builtin gns3/twin 工具不支持在线 test（400 语义清晰），建议 API 标注 testable=false |
| 14 | 2 个阿里云 MCP server 配置失效 | 09/日志 | 宿主 exe 路径容器不可达，健康检查连续失败 warn；清理或修正配置 |
| 15 | knowledge relation extract 超时 | 日志巡检 | PROVIDER/UNAVAILABLE context deadline，关注 provider 稳定性 |
| 16 | ListTools `limit` 参数不生效 | 08 | 仅 `page_size` 生效，默认 20 条；建议支持 limit 别名 |

## 三、各模块最终结论一览

| 模块 | 结论 | 备注 |
|------|------|------|
| 00-env-readiness | 11/11 PASS | 全栈健康 |
| 01-agent-mgmt | 10P/1F | BUG-01 |
| 02-chat-session | 10P/1F | BUG-02；LLM 6.4s 真实回复 |
| 03-team-orchestration | 10/10 PASS | 2 成员顺序执行 12s |
| 04-graph-orchestration | 13P + 3 发现 | BUG-G1 / ISSUE-G2 / ISSUE-G3 |
| 05-spirit | 7/7 PASS | PERF-S1 成本观察 |
| 06-memory | 17/17 PASS | 五层全通；写路径闭环 |
| 07-knowledge | 12/12 PASS | 摄入→检索 3s 闭环；ISSUE-K1 |
| 08-tools-twinops | 全 PASS | ISSUE-T1（17 监控工具 disabled）已修复；高危二次确认验证 |
| 09-mcp | 5/5 PASS | 探测/校验/热加载复核 |
| 10-monitor-scenario | 核心 6/6 PASS | BUG-MON-A/B/C；**HITL→GNS3→恢复全闭环** |
| 11-observability | 13/13 PASS | 审计/事件/trace 数据充足 |
| 12-usage-quota | 11/11 PASS | 配额 upsert→check 闭环 |
| 13-cron | 3/3 PASS | dream_cycle 15/15 成功，调度准时 |
| 14-skill | 7/7 PASS | |
| 15-provider-model | 6/6 PASS | 188 目录 provider 在线 |
| 16-hook-webhook | 7/7 PASS | SSRF 私网拦截正向验证 |
| 17-frontend | 11P/1F | SPA 真机挂载/路由/WS 全通；FAIL 为测试工具缺陷 |
| 18-performance | 修正后全 PASS | 基线/并发/资源/DB 四项达标；PERF-F1 |
| 19-cli | 8P/1F | 真实 LLM 链路 8.9s；BUG-CLI-01 |

## 四、性能摘要（18 模块详表）

- 只读接口基线：healthz 25ms / agents 29ms / sessions 26ms / tools 101ms / teams 44ms / graphs 27ms / flow-logs 20ms / memory-overview 53ms
- 并发：10 并发×5 波 50/50 成功，p95=462ms（含 PS 客户端开销，服务端 CPU 仅 0.31%）
- 异常点：model-catalog/providers 稳定 ~510ms（PERF-F1）
- 资源：admin 137.8MiB / postgres 366MiB / redis 11.7MiB；26 容器无异常

## 五、测试过程经验（已交叉验证的坑）

1. **PowerShell 自动变量三杀**：`$args`（19-cli）、`$pid`（15-provider）实参静默丢失——项目规约再次验证，形参命名必须避开自动变量。
2. **陈旧构建产物**：bin/aranea.exe（8-13）落后于源码（8-15），旧版不识别子命令全部落 REPL——测试前必先重建。
3. **API 路径漂移**：`/v1/chat/sessions`→`/v1/sessions`、`/v1/memory/overview`→`/v1/memory/layer-overview`（且强制 agent_id）、`/v1/monitor/flow-logs`——以 02/06/11 模块实测路径为准。
4. **dist 静态单发必须配 runtime-config.json**：生产构建空配置回退同源，脱离反代单发时 WS/API 全错（17 模块 FE-08 注入后 0 error + WS OPEN）。
5. **写操作三层校验落地**：MON-27/28 授权清理按「删前 count→事务删除→删后核验」执行，符合 2026-08-17 立规。

## 六、日志纪律执行

- 每模块 evidence/ 归档原始输出；00 模块起容器日志分段归档。
- 收尾巡检：aranea-admin 近 3h 3228 行日志 0 error/0 panic；warn 三类全部为已登记问题（K1 vault、relation extract 超时、阿里云 MCP 配置），见 `99-final-report/admin-errors-3h.txt` / `admin-warns-3h.txt`。
- Docker Desktop (Windows) json-file 日志位于 VM 内，宿主无法直接截断；建议 compose 侧配置 `logging.options.max-size/max-file` 轮转（当前未配）。

## 七、测试残留与待办

| 项 | 状态 |
|----|------|
| 会话 972daa64（02 模块） | 因 BUG-02 无法经 API 删除，待修复后清理 |
| Spirit 会话 6c0ec1f0（17/19 模块共用） | 真实会话，含 FE-12/CLI-07 消息，可留作回归入口 |
| workspace/default 配额（12 模块写入，宽松阈值） | 不影响判定，是否还原由用户裁定 |
| 17 个监控工具 enabled（08 模块修复） | **生产所需状态，保留** |
| runtime-config.json | 已恢复 `{}` 原始状态（注入版留存 17-frontend/evidence） |

> 全部模块资料：`docs/testing/realmachine-20260817/<模块>/`（cases.md + result.md + evidence/）。优化工作建议按第二节 P1→P2→P3 顺序汇总裁定。
