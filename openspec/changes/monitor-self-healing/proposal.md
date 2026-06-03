## Why

Monitor 模块的自检修复功能当前存在架构分层问题：诊断能力完整（DiagBundle + RootCauseEngine + 10 条规则），但修复动作在 biz 层无法实际控制运行时行为（DefaultHealActionHandler 只打日志不执行修复）。需要将修复动作下沉到 trpc-agent-go 运行时层，让 Monitor 层专注于"观测自愈效果 + 识别无法自愈的问题 + 升级告警"，同时将 HealRecord 持久化、根因分析增强运行时自愈状态维度、冷却期按规则分级。

## What Changes

- 将 FixAction 的实际执行从 biz 层 HealActionHandler 下沉到 trpc-agent-go 运行时（MCP 重连、Tool 重试、上下文压缩重试）
- 运行时自愈结果作为结构化 FlowLog 事件上报（新增 `auto_healed` / `heal_attempts` / `heal_strategy` 字段）
- SelfHealUsecase 重新定位为 SelfHealObserver：订阅事件 → 统计自愈成功率 → 识别反复自愈失败 → 升级告警
- RootCauseCondition 增加 `AutoHealed` / `HealAttempts` 维度，区分"运行时已自愈"和"运行时未自愈"的错误
- HealRecord 持久化到 SQLite（新增 Ent Schema `heal_records`）
- 冷却期从全局 5 分钟改为按规则严重级别分级（critical=30min, high=10min, medium=5min, low=2min）
- read_flow_logs 工具增加自愈上下文字段

## Capabilities

### New Capabilities
- `runtime-auto-heal`: trpc-agent-go 运行时内嵌自愈能力（MCP 重连、Tool 重试、上下文压缩重试），自愈结果作为结构化事件上报
- `self-heal-observer`: Monitor 层自愈效果观测器，统计自愈成功率、识别反复自愈失败模式、升级告警、持久化 HealRecord

### Modified Capabilities
- `root-cause-engine`: RootCauseCondition 增加 AutoHealed/HealAttempts 维度，冷却期按严重级别分级
- `diag-bundle`: DiagBundle 增加运行时自愈状态数据

## Impact

- **运行时层**（pkg/trpc-agent-go）: MCP broker 增加重连触发接口、Tool 执行器增加重试策略、LLM flow 增加上下文压缩重试
- **Biz 层**（internal/biz/monitor）: SelfHealUsecase 重构为 Observer 模式、HealRecord 持久化、RootCauseEngine 增加维度
- **Data 层**（internal/data）: 新增 heal_records Ent Schema + Repo
- **Service 层**（internal/service）: DiagnoseAndHeal API 调整返回值、新增 ListHealRecords API
- **Proto**（api/kratos/monitor/v1）: 新增 ListHealRecords RPC、DiagnoseAndHealResponse 增加字段
- **前端**（web）: Monitor 页面增加自愈效果仪表盘
