## 1. SkillIntelligence 接口拆分与常量提取

- [x] 1.1 在 `internal/biz/skill_intelligence.go` 中提取命名常量：`TimeoutThresholdMS=30000`、`ContextOverflowThreshold=5000`、`MinInvocationCount=5`、`DefaultNeutralScore=50`，替换所有魔法数字引用
- [x] 1.2 在 `internal/biz/skill_intelligence.go` 中修改 `NewSkillIntelligenceUsecase` 构造函数签名：接收 `ExperienceReportReader`、`ExperienceReportWriter`、`SkillHealthAggregator` 三个窄接口替代 `SkillIntelligenceRepo`，内部字段分别赋值
- [x] 1.3 在 `internal/biz/skill_intelligence.go` 中将 `GenerateReport` 的 ID 生成从 `fmt.Sprintf("rpt_%d_%s", ...)` 改为 `uuid.New().String()`，添加 `github.com/google/uuid` import
- [x] 1.4 在 `internal/data/skill_intelligence.go` 中将 `map[string]interface{}` 替换为 `map[string]any`（2 处：Create 和 BatchCreate）
- [x] 1.5 更新 `cmd/admin/wire.go` 中 `provideSkillIntelligenceUsecase` 参数：从 `repo biz.SkillIntelligenceRepo` 改为 `repo SkillIntelligenceRepo`（data 层类型），内部拆为 reader/writer/aggregator 传给构造函数
- [x] 1.6 验证：`make wire && go build ./...`

## 2. SelfHealObserver 构造函数与锁优化

- [x] 2.1 修改 `internal/biz/monitor/self_heal_observer.go` 中 `NewSelfHealObserver` 返回值从 `*SelfHealObserver` 改为 `(*SelfHealObserver, error)`，nil 参数时返回 `(nil, kerrors.InternalServer(...))`
- [x] 2.2 优化 `ObserveFlowLogEvent` 锁粒度：将 success 分支的 `delete(o.failCounts, stepID)` 和 fail 分支的 `o.failCounts[stepID]++` 合并到同一个 `o.mu.Lock()/Unlock()` 区间
- [x] 2.3 将 `SeverityCooldown` 从 exported `var` 改为 unexported `severityCooldown`，新增 exported 函数 `GetSeverityCooldown(severity string) time.Duration`
- [x] 2.4 更新 `cmd/admin/wire.go` 中 `provideSelfHealObserver` 适配新返回值 `(*SelfHealObserver, error)`
- [x] 2.5 验证：`make wire && go build ./...`

## 3. 全量验证

- [x] 3.1 运行 `go test ./internal/biz/... ./internal/data/... ./internal/service/... -count=1` 确认无测试回归
- [x] 3.2 运行 `make wire && make build` 确认构建通过
