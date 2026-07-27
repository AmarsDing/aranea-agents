# P1 Delta 协议与归因观测 — 实施审查报告

> 日期：2026-07-26
> 审查依据：`aranea-review` SKILL + `docs/review-dimension-checklists.md`
> 审查对象：[07-P1-delta协议与归因观测.design.md](../development/phase3-进化能力/07-P1-delta协议与归因观测.design.md) 的全部实施代码
> 结论：**通过（无阻断项）**，2 个 🟡 建议项已当场修复，1 个 🟡 设计语义备注，1 个 🟢 提示

---

## 一、审查范围

| 文件 | 变更 | 内容 |
|------|------|------|
| `internal/biz/skill_delta_protocol.go` | 新增 | Delta 更新协议：规则块解析/渲染、ops 解析/应用、计数归账 |
| `internal/biz/skill_attribution.go` | 新增 | 计数归因：最近一次 applied 进化的有效性裁决 + 回写 |
| `internal/biz/llm_skill_evolver.go` | 修改 | 双模式 Curator（delta / 全量重写 + 协议降级） |
| `internal/biz/skill_evolution_loop.go` | 修改 | Gate 第五维（effectiveness）+ 数据集回放进功能维 |
| `internal/biz/skill_intelligence.go` | 修改 | generateDraft 组装归因/trace/delta_ops 并落账 |
| `internal/biz/skill_evolution_unified.go` | 修改 | EvoMetaBaselineSuccessRate / EvoMetaDeltaOps / EvoMetaEffectiveness 常量 |
| `internal/service/skill_replay_runner.go` | 新增 | Solve 接线：评测数据集回放（含审查后窄接口化） |
| `cmd/admin/wire.go` | 修改 | provideSkillReplayRunner / provideSkillGateVerifier |
| `internal/biz/skill_gate_p1_test.go` | 新增（本次审查补充） | Gate 两个新维度 9 个用例 |
| `internal/service/skill_replay_runner_test.go` | 新增（本次审查补充） | 回放器 7 个用例 |

**维度加载**：1（架构）+ 2（质量）+ 3（正确性）+ 8（错误处理）+ 4（DB：metadata 落库）+ 6/11/14（Usecase）+ 7/12（跨模块：biz/service/evaluation/wire）+ 15（测试）。

## 二、架构与分层合规（维度 1）——全部通过

| 检查项 | 结果 |
|--------|------|
| BA2 biz 禁止 import `pkg/trpc-agent-go` | ✅ 全部新/改文件仅依赖 stdlib + apierror + loggateway |
| BA3 biz 禁止 import proto | ✅ |
| BA6 跨模块经窄接口 | ✅ 审查中修复：`SkillReplayRunner` 原持有具体类型 `*evaluation.Usecase`，已窄化为 `evalDatasetReader`（2 方法）+ 编译期断言 `var _ evalDatasetReader = (*evaluation.Usecase)(nil)` |
| BA9 Graph 运行时类型泄漏 | ✅ 不涉及 |
| BL2 Service 错误经 apierror | ✅ `SKILL_REPLAY` 域，BadRequest/Unavailable/Wrap |
| BL5 连接访问器 | ✅ 无新增 DB 连接（复用 unifiedStore / evaluation repo） |
| Wire | ✅ 两个 provider 注册进 ProviderSet，`go build ./cmd/admin` 通过，无手动编辑 wire_gen |

## 三、正确性与错误处理（维度 3/8/14）——全部通过

- **BE1/BE4**：业务错误全部 `apierror`；best-effort 路径（persistDeltaOps / 归因回写 / trace 收集）失败均 Warn 日志降级，无静默吞错
- **BE6 nil 检查**：`applied[0]` 前置 `len==0` 检查；`replay result == nil` 跳过；`observation == nil` 性能维跳过；`Attribution == nil` 跳过归账
- **协议严格性**：`ParseDeltaOpsJSON` 逐项校验 + `ApplyDeltaOps` 严格语义（modify/merge/remove 必须存在、add 必须新 id），任一违规整体拒绝 → 调用方回退全量重写，无半应用状态
- **幂等**：归因裁决已存在则直接复用（`EvoMetaEffectiveness` 非空短路）；重复归账依赖版本不可变语义（计数只在新版本注册时传递）
- **不变量**：规则 id 跨版本稳定（全量重写 system prompt 要求保留既有 id）；`GateHarmfulRuleRejectThreshold=3` 常量化（CS-B8 ✅）

## 四、审查发现与处置

| # | 级别 | 发现 | 处置 |
|---|------|------|------|
| F1 | 🟡 | Gate 两个新维度（verifyReplay / verifyEffectiveness）无单元测试（BP13/BP14） | **已修复**：新增 `skill_gate_p1_test.go` 9 个用例（阈值拒绝/通过、跳过语义三分支、harmful 规则保留拒绝/重写/移除/未达阈值、nil-safe 三分支） |
| F2 | 🟡 | `SkillReplayRunner` 无单元测试且依赖具体 `*evaluation.Usecase` 不可测（BA6/TS） | **已修复**：窄接口化 + 新增 `skill_replay_runner_test.go` 7 个用例（寻址约定、大小写不敏感评分、maxCases 截断、LLM 未配置 Unavailable、单 case 失败不中断、空用例、空期望） |
| F3 | 🟡 | **设计语义备注**：verifyEffectiveness 读取的是当前已发布版本的计数（不含本周期刚归账的 bump）。规则 harmful=2 + 本周期 harmful 裁决 → 需下一周期才触发拒绝（一周期滞后）。语义可防为（基于已结算计数裁决），已在设计文档注明，不修改 | 记录，无需代码变更 |
| F4 | 🟢 | verifyReplay 返回的 `GateCheckResult.Name` 同为 `"functional"`，与 base check 同名。当前实现 verifyFunctional 二选一返回，无实际问题 | 记录备忘 |
| F5 | 🟢 | 回放串行执行最坏 5 case × 30s = 150s，Gate 同步链路延迟。仅当 skill 绑定数据集时触发，可接受；后续可考虑 case 级并发 | 记录备忘（P2 候选） |

## 五、验证证据（FR1–FR3）

| 命令 | 结果 |
|------|------|
| `go build ./...` | exit 0 |
| `go vet ./internal/biz/ ./internal/service/` | exit 0 |
| `gofmt -l`（全部 P1 文件，含会话早期创建的 3 个测试文件补格式化） | 无输出 |
| `go test ./internal/biz/ -count=1` | ok 17.6s（含新增 9 用例全绿） |
| `go test ./internal/service/ -count=1` | ok 11.0s（含新增 7 用例全绿） |

## 六、结论

P1 四项能力（Delta 更新协议、计数归因、trace 级观测、Solve 回放接线）实施与设计文档一致，架构红线零违规，nil-safe 降级路径完整。F1/F2 测试缺口已当场补齐并全绿。P1 实施**审查通过，可归档**。
