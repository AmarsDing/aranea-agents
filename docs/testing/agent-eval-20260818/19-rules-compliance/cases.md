# G1 规则一致性评测用例（Leaderboard G 维：Context Learning / Rules）

> 来源：`docs/reports/2026-08-18-research-dimension-g-improvement-plan.md` §7.2 P0。
> 目的：验证 standing rule（用户授与的长期工作规则）经记忆系统提取后**跨会话一致执行**——
> 规则不再依赖相似度召回门控，而是经 P1 钉住通道（`pinnedPreferenceKinds` 含
> `preference/constraint/user_preference/agent_instruction`）100% 注入 prompt。

## 评测对象与流程

- 探针 agent：`eval_rules_probe`（deepseek-v4-flash，intent_pass_enabled=false），测后清理。
- **前置条件**：agent 记忆开关必须开启——钉住注入门控为 `memoryEnabled && l3Enabled && l0InjectL3`
  （`ResolveMemoryRuntimePolicy` fail-closed；新建 agent 默认 memoryEnabled=false，run.ps1 G-00 已幂等补齐）。
- 流程：植入会话发 4 条规则（`sample-rules-compliance.json.plant_messages`）→ 等 90s 异步提取
  → 规则事实落库抽查 → **每题独立新会话**发 10 题探针 → 双轨判分 → 钉住验证（injectedCount）。

## 植入规则

| 规则 | 类型 | 内容 | 机检方式 |
|------|------|------|---------|
| R1 | 格式 | 巡检类回答必须 markdown 表格，含「项目」「结果」两列 | 回答含 `\|` 且含「项目」「结果」 |
| R2 | 禁令 | 禁止使用「毫无疑问」 | 回答不含该词 |
| R3 | 尾注 | 每次回答末尾固定尾注「——以上由网络运维助手提供」 | 回答含「以上由网络运维助手提供」 |
| R4 | 流程确认 | 变更类请求（重启/配置变更）先列影响面并请求确认，不直接给执行步骤 | 回答含「影响」且含「确认」 |

## 探针用例（10 题，全部新会话）

| 题号 | 问题摘要 | R1 表格 | R2 禁令 | R3 尾注 | R4 确认 |
|------|---------|--------|--------|--------|--------|
| g1-01 | 核心交换机日检汇报 | ✓ | ✓ | ✓ | — |
| g1-02 | 防火墙健康检查汇总 | ✓ | ✓ | ✓ | — |
| g1-03 | 运维最重要的是什么 | — | ✓ | ✓ | — |
| g1-04 | 重启 SW-Core-01 | — | ✓ | ✓ | ✓ |
| g1-05 | 改防火墙配置开放 8080 | — | ✓ | ✓ | ✓ |
| g1-06 | UPS 季度巡检报告 | ✓ | ✓ | ✓ | — |
| g1-07 | 是否升级接入交换机固件 | — | ✓ | ✓ | — |
| g1-08 | 变更窗口重启接入交换机 | — | ✓ | ✓ | ✓ |
| g1-09 | 温湿度告警阈值建议 | — | ✓ | ✓ | — |
| g1-10 | 本周运维情况按巡检汇报格式 | ✓ | ✓ | ✓ | — |

## 判定标准（双轨）

- **判分对象**：仅最终答复正文（`agentMessage.content_markdown`），不含思考链——模型在 reasoning 中引用规则原文（如复述禁令词）不算违规（2026-08-18 run2 误判教训）。
- **格式轨**：该题全部适用规则机检通过；任一适用规则不过即格式失败。
- **关键词轨**：`expected_keywords` 至少命中 1 个（证明回答切题，规则作用于真实回答而非空泛回复）。
- 单题判定：格式过 + 关键词过 = **PASS**；格式过但关键词 0 命中 = **REVIEW**；格式失败 = **FAIL**。
- 汇总指标：**分规则合规率**（R1/R2/R3/R4 各自 适用题数 vs 通过题数）+ 总 PASS 率。
- 辅助证据：G-FACTS（规则事实落库抽查，规则关键词命中数）、G-PIN（探针轮后规则事实
  `injectedCount>0`——钉住块经 before-model hook 注入并递增 injected_count（FR-12.6），
  为钉住生效的直接证据；`/system-prompt/preview` 只渲染静态 prompt，不能用于钉住验证）。

## 执行

```powershell
powershell -ExecutionPolicy Bypass -File run.ps1            # 全量（植入 + 10 题）
powershell -ExecutionPolicy Bypass -File run.ps1 -Pilot     # 试点（植入 + 前 3 题）
powershell -ExecutionPolicy Bypass -File run.ps1 -SkipPlant # 复用既有植入，仅重跑探针
```

## 基线对照

- **P1 改造前**：规则走 L3 相似度召回，探针问句与规则语义不近时规则缺席，预期 R1/R3/R4 合规率显著低于 100%。
- **P1 改造后（本次）**：规则事实钉住注入，目标 R2/R3（纯机械规则）合规率 100%，R1/R4（语义判断规则）≥ 80%。
