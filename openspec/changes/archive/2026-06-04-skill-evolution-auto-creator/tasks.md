# Skill Evolution Auto Creator — 任务清单

**Goal:** 实现 Agent 运行时检测重复模式并自动提议创建新 Skill 的完整闭环：检测 → 生成 → 审批 → 注册。

**Architecture:** 四层架构（Server → Service → Biz → Data）+ Wire DI + trpc-agent-go 框架集成。

**Design Doc:** [design.md](./design.md)

---

## Phase 1 — 数据基础

### Task 1: 定义 SkillProposal 领域模型

- [x] `internal/biz/skill_evolution_types.go` 定义 `SkillProposal` 结构体和 5 个状态常量

### Task 2: 定义 Repo 端口接口

- [x] `internal/biz/skill_evolution_repo.go` 定义 `SkillProposalReader`/`SkillProposalWriter`/`SkillProposalReadWriter` 接口

### Task 3: 创建 skill_proposals 表 DDL

- [x] `internal/data/sql/skill_evolution.sql` DDL 定义
- [x] `internal/data/skill_evolution_schema.go` embed + EnsureSkillEvolutionSchema
- [x] DDL 注册到迁移框架（版本 20260706）

### Task 4: 实现 Data 层 Repo

- [x] `internal/data/skill_evolution.go` 实现 `skillProposalRepo`
- [x] `NewSkillProposalRepo` 加入 `data.ProviderSet`

**Phase 1 验证：** ✅ `go build ./...` 通过

---

## Phase 2 — Skill 生成与注册

### Task 5: 实现 SkillAutoCreator.GenerateSKILLMD

- [x] `internal/skill/auto_creator.go` 实现 `SkillAutoCreator` 结构体，`GenerateSKILLMD` 方法调用 LLM 生成 SKILL.md，30s 超时控制
- [x] `buildSkillPrompt` 和 `parseSkillOutput` 辅助函数

### Task 6: 实现 SkillAutoCreator.ValidateSKILLMD

- [x] `GenerateSKILLMD` 内含 YAML front matter 验证（`---` 开头检查）

### Task 7: 实现 SkillRegistrationPort

- [x] `biz.SkillRegistrationPort` 接口定义（RegisterSkill/SkillExists）
- [x] `service.NewSkillsButlerRegistrationAdapter` 适配 `SkillUsecase`
- [x] Wire 注入 `provideSkillRegistrationPort`

**Phase 2 验证：** ✅ `go build ./...` 通过

---

## Phase 3 — 业务闭环

### Task 8: 实现 SkillEvolutionUsecase.DetectAndPropose

- [x] `internal/biz/skill_evolution.go` 实现 `SkillEvolutionUsecase` 结构体
- [x] `DetectAndPropose` 方法：查询 PatternReader → 筛选 ToolCall 模式 → 去重检查 → 调用 SkillAutoCreator → 创建 SkillProposal
- [x] `findSkillPatterns` 和 `extractToolHistory` 辅助方法

### Task 9: 实现 SkillEvolutionUsecase.ApproveProposal / RejectProposal

- [x] `ApproveProposal`：校验 status=pending → 更新为 approved
- [x] `RejectProposal`：校验 status=pending → 更新为 rejected

### Task 10: 实现 SkillEvolutionUsecase.RegisterApproved

- [x] `RegisterApproved`：校验 status=approved → 调用 SkillRegistrationPort.RegisterSkill → 更新 status=registered
- [x] `ListProposals` 和 `CreateProposal` 方法
- [x] `ScanAndProposeAll` 批量扫描方法

**Phase 3 验证：** ✅ `go build ./...` 通过

---

## Phase 4 — 接入层

### Task 11: 新增 skill_evolution.proto + Service 层

- [x] 定义 proto：`ListProposals`/`GetProposal`/`ApproveProposal`/`RejectProposal`/`RegisterSkillProposal`/`TriggerDetection` RPC
- [x] 运行 `make api` 生成代码
- [x] 实现 Service 层 `skill_evolution.go`，proto↔biz 映射
- [x] 新增 `biz.SkillEvolutionUsecase.GetProposal` 方法
- [x] 注册到 ServiceRegistry + HTTP Server + gRPC Server
- [x] Wire 注入 `NewSkillEvolutionService`

### Task 12: 新增 cronrunner 定时任务

- [x] `internal/cronrunner/jobs/skill_evolution.go` 实现 `SkillEvolutionScanner`
- [x] 定时遍历活跃 Agent → 调用 `SkillEvolutionUsecase.ScanAndProposeAll`

### Task 13: Wire DI 装配

- [x] `NewSkillEvolutionUsecase` 在 `biz.ProviderSet` 中
- [x] `provideSkillAutoCreator` 和 `provideSkillRegistrationPort` 在 `wire.go` 中
- [x] `wire.Bind(new(biz.PatternReader), new(biz.PatternReadWriter))` 在 `wire.go` 中
- [x] `SkillEvolutionUsecase` 注入到 `ChatOrchestratorDeps.SkillEvo`
- [x] `make wire && make wire-clean` 通过

**Phase 4 验证：** ✅ Wire + Build + Proto + Service 层全部通过

---

## Phase 5 — 验证

### Task 14: 单元测试

- [x] 编写 `SkillEvolutionUsecase` 单元测试（14 个测试用例：Approve/Reject/Register/Get/List/Detect/Dedup/CreateProposal/Hash/ExtractToolNames）
- [x] 编写 `SkillAutoCreator` 单元测试（10 个测试用例：GenerateSKILLMD 成功/失败/无 front matter/自动命名 + buildSkillPrompt + parseSkillOutput）

### Task 15: 集成测试

- [x] 编写 Service 层集成测试（9 个测试用例：ListSkillProposals/GetSkillProposal/Approve/Reject/TriggerDetection + toProtoSkillProposal 映射）
- [x] 修复已有 `team_compile_view_test.go` 编译错误（函数签名变更导致的参数不匹配）

**Phase 5 验证：** ✅ 所有单元测试和集成测试通过

---

## 补充：Skills Butler 工具集成

### Task 16: Skills Butler 工具注入到 Agent 运行时

- [x] `internal/service/skills_butler_adapter.go` 三个端口适配器
- [x] `skillsButlerTools()` 方法，仅在 Agent 启用 `EvolutionSkillEvolve` 时返回工具
- [x] `chat_orchestrator_turn.go` 中 CustomTools 追加
- [x] `ChatOrchestratorDeps` 新增 `SkillEvo`/`Evolution`/`SkillStats` 字段
- [x] `internal/biz/skill_invocation_stats.go` 定义 `SkillInvocationStatsReader` 接口
- [x] `internal/data/skill_invocation_stats.go` 实现 `skillInvocationStatsRepo`
- [x] `NewSkillInvocationStatsRepo` 加入 `data.ProviderSet`
