## Context

aranea-review 全栈代码审查（7 个归档变更后）发现 9 项建议级问题，全部为后端代码质量改进。当前状态：

- `SkillIntelligenceRepo` 组合接口 7 方法，超过 ≤5 规范
- `NewSkillIntelligenceUsecase` 接收组合接口但内部拆为 3 个字段
- `NewSelfHealObserver` 返回 nil 而非 error
- `SelfHealObserver.ObserveFlowLogEvent` 两次独立锁操作可合并
- `GenerateReport` 用 `fmt.Sprintf` 生成 ID
- `skill_intelligence.go` 多处魔法数字
- `SeverityCooldown` 为可变包级 var
- `data/skill_intelligence.go` 使用 `interface{}` 而非 `any`

## Goals / Non-Goals

**Goals:**
- 拆分 SkillIntelligenceRepo 为 3 个窄接口（≤5 方法），构造函数分别注入
- 改善 SelfHealObserver nil 处理（返回 error）
- 优化 SelfHealObserver 锁粒度
- 提取魔法数字为命名常量
- 统一 ID 生成方式
- SeverityCooldown 只读化
- `interface{}` → `any` 现代化

**Non-Goals:**
- 不重构 SelfHealUsecase（deprecated fallback）
- 不修改前端代码
- 不新增 API/Proto
- 不修改 Ent Schema

## Decisions

### D1: SkillIntelligenceRepo 拆分策略

**选择**: 保留 `SkillIntelligenceRepo` 作为组合接口（供 Wire 绑定），但 `NewSkillIntelligenceUsecase` 改为接收 3 个窄接口参数。

**替代方案**: 完全删除 `SkillIntelligenceRepo` 组合接口 → Wire 需 3 个独立 bind → 增加配置复杂度。

**理由**: 组合接口作为 Wire 绑定锚点，data 层只需实现一个 struct；biz 层通过构造函数窄接口声明真实依赖，两全其美。

### D2: NewSelfHealObserver nil 处理

**选择**: 返回 `(*SelfHealObserver, error)` 而非 `nil`。

**理由**: nil 返回值要求调用方手动检查，容易遗漏。返回 error 让编译器和 Wire 框架协助处理。

### D3: 锁粒度优化

**选择**: 将 `ObserveFlowLogEvent` 中 success 分支的 `delete(failCounts)` 和 fail 分支的 `failCounts[stepID]++` 合并到同一个锁区间。

**理由**: 两次独立锁操作增加开销且语义上属于同一观察周期，合并后无功能变化但减少锁竞争。

### D4: ID 生成方式

**选择**: 使用 `uuid.New().String()` 替代 `fmt.Sprintf("rpt_%d_%s", ...)`。

**理由**: 项目其他处（如 `generateHealID`）已使用 uuid，保持一致性。`fmt.Sprintf` 格式 ID 在高并发下有碰撞风险。

### D5: SeverityCooldown 只读化

**选择**: 改为 unexported map + exported accessor 函数 `GetSeverityCooldown(severity string) time.Duration`。

**理由**: 包级 var 可被外部包修改，accessor 提供只读视图。

## Risks / Trade-offs

- **[Wire 签名变更]** → `provideSkillIntelligenceUsecase` 和 `NewSkillIntelligenceUsecase` 参数变更，需同步更新 `wire.go` 和 `wire_gen.go`（通过 `make wire` 自动生成）
- **[构造函数返回值变更]** → `NewSelfHealObserver` 从 `*SelfHealObserver` 改为 `(*SelfHealObserver, error)`，Wire provider 需适配
- **[低风险]** 常量提取和 `interface{}` → `any` 为纯机械变更，无行为影响
