## Why

aranea-review 全栈代码审查发现 9 项建议级问题（无阻断项），涉及 SkillIntelligenceRepo 接口方法数超标、魔法数字、锁粒度优化、构造函数接口设计等。这些问题虽不阻塞合并，但累积后影响可维护性和编码规范合规性，需在独立迭代中修复。

## What Changes

- **S1/S3**: 拆分 `SkillIntelligenceRepo` 组合接口（7 方法）为 `ExperienceReportReader`（3）、`ExperienceReportWriter`（2）、`SkillHealthAggregator`（2）三个窄接口，`NewSkillIntelligenceUsecase` 构造函数改为分别接收三个窄接口
- **S2**: `NewSelfHealObserver` 在 repo/engine 为 nil 时返回 nil，改为返回 error 让编译器辅助调用方处理
- **S4**: `SelfHealObserver.ObserveFlowLogEvent` 中 success/fail 分支的两次独立锁操作合并为一次锁区间
- **S5**: `GenerateReport` 中 `fmt.Sprintf("rpt_%d_%s", ...)` ID 生成改为 `uuid.New().String()`
- **S6/S7**: 提取 `skill_intelligence.go` 中魔法数字为命名常量（超时阈值 30000、context overflow 阈值 5000、最小调用次数 5、默认分数 50）
- **S8**: `SeverityCooldown` 从包级 `var` 改为 unexported + 只读 accessor
- **S9**: `data/skill_intelligence.go` 中 `map[string]interface{}` 改为 `map[string]any`

## Capabilities

### New Capabilities

（无新能力引入）

### Modified Capabilities

- `skill-intelligence`: SkillIntelligenceRepo 接口拆分 + 构造函数签名变更 + 魔法数字提取
- `monitor-self-heal`: SelfHealObserver 构造函数 nil 处理 + 锁粒度优化 + SeverityCooldown 只读化

## Impact

- **biz 层**: `skill_intelligence.go`（构造函数签名、常量提取）、`skill_intelligence_repo.go`（接口拆分后 SkillIntelligenceRepo 保留为组合别名）、`monitor/self_heal_observer.go`（构造函数、锁优化、SeverityCooldown）
- **data 层**: `skill_intelligence.go`（`interface{}` → `any`）
- **Wire**: `cmd/admin/wire.go` 中 `provideSkillIntelligenceUsecase` 参数调整 + `wire.Bind` 更新
- **测试**: 受影响构造函数的测试文件需同步更新参数

## Non-goals

- 不重构 SelfHealUsecase（已 deprecated，仅保留 fallback）
- 不修改前端代码
- 不新增 API/Proto 变更
