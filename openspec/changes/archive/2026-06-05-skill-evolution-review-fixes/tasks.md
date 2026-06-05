## 1. 阻断项修复 — LLM 适配器注入

- [x] 1.1 在 `internal/skill/auto_creator.go` 新增 `LLMCallerAdapter` 结构体，实现 `LLMSkillGenerator` 接口，内部持有 `biz.LLMCaller`
- [x] 1.2 修改 `cmd/admin/wire.go` 中 `provideSkillAutoCreator`，构造 `LLMCallerAdapter` + `NewSkillAutoCreator(adapter, lg)` 返回有效实例
- [x] 1.3 验证：`go build ./...` 通过，`SkillEvolutionUsecase.creator` 不为 nil

## 2. 阻断项修复 — SkillEvolutionScanner Wire 注册

- [x] 2.1 在 `cmd/admin/wire.go` 新增 `provideSkillEvolutionScanner` provider 函数，参照 `provideLearningLoopScanner` 模式，支持 `SKILL_EVOLUTION_DISABLED` 环境变量
- [x] 2.2 将 `SkillEvolutionScanner` 加入 wireOut 结构体字段
- [x] 2.3 在 Bootstrap 启动流程中调用 `scanner.Start(ctx)`
- [x] 2.4 将 `provideSkillEvolutionScanner` 加入 wire ProviderSet
- [x] 2.5 验证：编译通过

## 3. 阻断项修复 — UpdateStatus 事务一致性

- [x] 3.1 修改 `internal/data/skill_evolution.go` 的 `UpdateStatus` 方法，使用 `r.data.RWDB().WriteHandle().BeginTx()` 开启事务
- [x] 3.2 在事务内执行 UPDATE + SELECT，事务提交后返回结果
- [x] 3.3 验证：编译通过

## 4. nil 依赖处理统一

- [x] 4.1 修改 `internal/biz/skill_evolution.go` 的 `RegisterApproved` 方法，当 `registrar == nil` 时返回空结果 + Warn 日志（而非 `kerrors.InternalServer`）
- [x] 4.2 修改 `DetectAndPropose` 方法，当 `creator == nil` 时增加 Warn 日志
- [x] 4.3 验证：`go test ./internal/biz/... -run TestSkillEvolution -count=1` 通过

## 5. 分页实现

- [x] 5.1 修改 `internal/biz/skill_evolution_repo.go` 的 `SkillProposalReader.ListByAgent` 接口签名，增加 `limit int, offset int` 参数
- [x] 5.2 修改 `internal/data/skill_evolution.go` 的 `ListByAgent` 实现，SQL 添加 `LIMIT ? OFFSET ?`
- [x] 5.3 修改 `internal/biz/skill_evolution.go` 的 `ListProposals` 方法，传入 `limit, offset` 参数
- [x] 5.4 修改 `internal/service/skill_evolution.go` 的 `ListSkillProposals`，传入 limit/offset 到 biz 层
- [x] 5.5 更新所有 mock/stub 中的 `ListByAgent` 签名
- [x] 5.6 验证：`go test ./internal/biz/... ./internal/service/... -run TestSkillEvolution -count=1` 通过

## 6. RegisterApproved 校验 SkillMD 非空

- [x] 6.1 修改 `internal/biz/skill_evolution.go` 的 `RegisterApproved` 方法，在调用 `registrar.RegisterSkill` 前校验 `p.SkillMD != ""`
- [x] 6.2 校验失败返回 `kerrors.BadRequest("SKILL_EVO", "cannot register proposal with empty skill content")`
- [x] 6.3 新增单元测试覆盖空 SkillMD 场景
- [x] 6.4 验证：`go test ./internal/biz/... -run TestRegisterApproved_EmptyMD -count=1` 通过

## 7. 魔法数字提取为命名常量

- [x] 7.1 在 `internal/biz/skill_evolution.go` 定义常量 `const defaultScanAgentLimit = 500` 和 `const skillPatternMinConfidence = 0.15`
- [x] 7.2 替换 `ScanAndProposeAll` 中的 `500` 为 `defaultScanAgentLimit`
- [x] 7.3 替换 `findSkillPatterns` 中的 `0.15` 为 `skillPatternMinConfidence`
- [x] 7.4 在 `internal/data/skill_invocation_stats.go` 中将 `"success"` 提取为常量 `toolInvocationStatusSuccess`
- [x] 7.5 验证：`go build ./...` 通过

## 8. skillsButlerTools ctx 传递修复

- [x] 8.1 修改 `internal/service/cli_admin_tools.go` 的 `skillsButlerTools` 方法，将 `context.Background()` 替换为传入的 `ctx` 参数
- [x] 8.2 验证：`go build ./...` 通过

## 9. GetSkillInvocationStats SQL 聚合优化

- [x] 9.1 修改 `internal/data/skill_invocation_stats.go` 的 `GetSkillInvocationStats` 方法，替换 Ent 查询为原生 SQL 聚合查询
- [x] 9.2 SQL: `SELECT tool_key, COUNT(*), SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), COALESCE(SUM(duration_ms), 0) FROM tool_invocations WHERE agent_id=? AND created_at>=? GROUP BY tool_key`
- [x] 9.3 验证：编译通过

## 10. ScanAndProposeAll 分页遍历

- [x] 10.1 修改 `internal/biz/skill_evolution.go` 的 `ScanAndProposeAll` 方法，实现分页循环遍历（使用 `defaultScanAgentLimit` 作为每页大小，循环直到 `len(page.Items) < limit`）
- [x] 10.2 验证：`go test ./internal/biz/... -run TestSkillEvolution -count=1` 通过

## 11. 删除未使用常量

- [x] 11.1 删除 `internal/biz/skill_evolution_types.go` 中的 `SkillProposalStatusExpired` 常量
- [x] 11.2 验证：`go build ./...` 通过，无编译错误

## 12. 全量验证

- [x] 12.1 编译和单元测试通过
- [x] 12.2 确认所有新增/修改的测试用例通过
