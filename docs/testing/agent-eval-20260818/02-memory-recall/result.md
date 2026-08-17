# 域 B 记忆召回评测结果（最终版，2026-08-18）

Pilot=False SkipPlant=False；植入消息数=60；植入会话=f1f65b10-679f-435b-8b21-e1f981982515
评测 agent：eval_memory_probe（a21265a2d8f24072fb638b50）；评分口径：regrade.ps1 仅检查 `agentMessage.content_markdown`（排除思考链误杀）。
明细：[evidence/regrade-results.txt](evidence/regrade-results.txt)；失败回答原文：dump_failures.ps1 输出。

## 准确率（regrade 机判口径）

| 类别 | PASS | FAIL | REVIEW | 准确率(不含REVIEW) |
|------|------|------|--------|------|
| single_hop | 8 | 2 | 0 | 80% |
| multi_hop | 5 | 5 | 0 | 50% |
| temporal | 3 | 7 | 0 | 30% |
| update | 8 | 2 | 0 | 80% |
| abstention | 3 | 5 | 2 | 38% |
| **合计** | **27** | **21** | **2** | **56%** |

## 准确率（人工修正评测误杀后，真实功能口径）

| 类别 | 真实通过 | 真实准确率 | 修正说明 |
|------|------|------|------|
| single_hop | 8/10 | 80% | — |
| multi_hop | 5/10 | 50% | — |
| temporal | 5/10 | 50% | tp-01/tp-09 实际答对，基准关键词带空格（"3 月"/"9 点"）不匹配无空格表述 |
| update | 8/10 | 80% | — |
| abstention | 9/10 | 90% | ab-02/07/09/10 正确拒答但拒答词典未覆盖（"没有找到"≠"未找到"）；ab-03/05 仅提及用户名属正常；ab-04 http 000 超时（基础设施） |
| **合计** | **35/50** | **70%** | 真实功能缺陷 14 条 + 基础设施 1 条 |

## 失败根因分类（21 FAIL + 2 REVIEW 全量归因）

### R1：session scope 事实跨会话不可见（12 条，主根因）
- **Case**：sh-04、mh-02、mh-04、mh-06、mh-07、mh-08、tp-02、tp-03、tp-04、tp-06、tp-07、up-06
- **证据链**：
  1. DB 取证（q_fail_facts.sql，122 条事实）：目标事实全部落库但 `scope_type='session'`（如 `a7e78885 边界防火墙FW-Edge-02的管理IP为10.20.0.2`、`9c5f221a ELK集群部署在elk-01/02/03`、`0783ce74 UPS电池组质保到2027年6月`）
  2. agent 配置 `l3RecallScopesJson=["agent","user","team","workspace"]` —— **不含 session**
  3. mem-sh-04 recall-debug：l3Hits 仅 1 条 agent scope 事实，session 事实未进候选集
  4. 对照组：凡 PASS 的 case 均有 user/agent scope 的同义提升事实（consolidation 产物，含多条英文复述）；未提升的 session 事实全部 miss
- **机理**：worker 提取时将大量运维客观事实（设备位置、带宽、质保、部署主机）保守地标为 session scope；L3 召回按配置排除 session scope → 跨会话提问不可见。consolidation（sleep-time 提升）只覆盖了部分事实，未提升的永久丢失。
- **修复方向**（C 阶段候选）：(a) 提取 prompt 明确"客观运维事实默认 user/team scope"；(b) recall scopes 加入 session（同 user 的跨 session 可见性需确认设计意图）；(c) consolidation 提升覆盖率监控。

### R2：PII 脱敏误伤运维电话号码（2 条）
- **Case**：sh-06（值班电话）、up-02（值班电话更新）
- **证据**：DB 中 4 条值班电话事实 `pii_flag=1`，statement 已替换为 `[phone]` 占位符（原文在 redacted_statement）；模型回答原文复述 `[phone]`（mem-sh-06 回答 "当前团队值班电话为 **[phone]**"）
- **机理**：手机号正则误匹配固定电话（0571-8899-1234）；cue 注入用脱敏后 statement，LLM 无原文可答
- **修复方向**：PII 规则区分手机/固话；或运维场景白名单；或 cue 注入时对 user 本人可见事实用 redacted_statement

### R3：评测基准误杀（8 条，非产品缺陷）
- tp-01/tp-09：基准关键词含空格（"3 月"/"9 点"），模型正确回答 "2026年3月1日"/"09:00–09:15" 不匹配 → 基准改为无空格或多候选
- ab-02/07/09/10：regrade 拒答词典过窄（缺 "没有找到"/"都没有找到"/"我没有您的" 等表述），实际回答均为教科书式正确拒答
- ab-03/05：REVIEW 项，仅提及用户姓名 "张伟" 属正常称呼，应判 PASS
- **修复方向**：基准 JSON 关键词去空格化；regrade 词典扩充（已修正口径见上表）

### R4：基础设施（1 条）
- ab-04：http 000 / 180s 超时，无回答。与记忆链路无关。

## 性能画像

### 召回段延迟（recall/debug，不经 LLM，n=10）
- min=23ms max=24ms P95=24ms ✅（目标 <500ms；业界参考 Mem0 549ms / Zep ~200ms）

### 端到端 ASK 延迟（含 LLM + 工具）
- 召回命中时（PASS case）：3~8s（典型 4.5~6.5s）
- 召回 miss 时（R1/R2 case）：**15~97s**——agent 退化为多轮知识库/工具检索（知识库→工作区→工具搜索循环），延迟放大 10~20 倍，且最终仍答错
- 衍生结论：召回准确率直接决定 e2e 延迟分布；R1 修复后预期 P95 大幅收敛

### B07 prompt 体积
- 植入前 84 字符 → 植入后 84 字符（memory cue 为运行时注入，不进静态 prompt，符合设计）

## 结论

域 B 真实准确率 **70%**（35/50）。两大功能缺陷：
1. **R1 session scope 召回黑洞**（12/14 功能失败，86%）——提取端 scope 分类过保守 + 召回端配置排除 session，叠加 consolidation 提升不全
2. **R2 PII 脱敏误伤固话**（2/14）

C 阶段修复优先级：R1（提取 prompt/scope 策略）> R2（PII 规则）> 中文分词（kwScore 权重辅助，非主因——R1 修复后 vecScore 已足够支撑召回）。
