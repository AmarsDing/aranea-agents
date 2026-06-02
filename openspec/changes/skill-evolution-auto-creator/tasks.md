# Skill Evolution Auto Creator — 任务清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Agent 运行时检测重复模式并自动提议创建新 Skill 的完整闭环：检测 → 生成 → 审批 → 注册。

**Architecture:** 四层架构（Server → Service → Biz → Data）+ Wire DI + trpc-agent-go 框架集成。

**Design Doc:** [design.md](./design.md)

---

## Phase 1 — 数据基础

### Task 1: 定义 SkillProposal 领域模型

**Files:**
- Create: `internal/biz/skill_evolution_types.go`

- [ ] **Step 1:** 创建 `skill_evolution_types.go`，定义 `SkillProposal` 结构体（ID/AgentID/PatternHash/PatternDesc/SkillName/SkillMD/Status/ApprovedBy/RejectedBy/CreatedAt/ApprovedAt）和状态常量（pending/approved/rejected/registered/expired）

**DoD:** `SkillProposal` 结构体和 5 个状态常量定义完成，`go build ./internal/biz/...` 通过

---

### Task 2: 定义 Repo 端口接口

**Files:**
- Create: `internal/biz/skill_evolution_repo.go`

- [ ] **Step 1:** 创建 `skill_evolution_repo.go`，定义 `SkillProposalReader`（ListByAgent/GetByID/GetByPatternHash）和 `SkillProposalWriter`（Create/UpdateStatus）接口

**DoD:** `SkillProposalReader` 和 `SkillProposalWriter` 接口定义完成，`go build ./internal/biz/...` 通过

---

### Task 3: 创建 Ent Schema skill_proposal

**Files:**
- Create: `internal/data/ent/schema/skill_proposal.go`

- [ ] **Step 1:** 创建 `skill_proposal.go` Ent Schema，字段：id/string/agent_id/pattern_hash/pattern_desc/skill_name/skill_md/status/approved_by/rejected_by/created_at/approved_at，status 默认 `pending`，pattern_hash + agent_id 唯一索引

- [ ] **Step 2:** 运行 `go generate ./internal/data/ent/...`

**DoD:** Ent Schema 创建完成，代码生成无错误，`go build ./internal/data/...` 通过

---

### Task 4: 实现 Data 层 Repo

**Files:**
- Create: `internal/data/skill_evolution.go`
- Modify: `internal/data/data.go`（Wire 绑定）

- [ ] **Step 1:** 创建 `skill_evolution.go`，实现 `SkillProposalReader` 和 `SkillProposalWriter` 接口，包含 `entSkillProposalToBiz` 转换函数

- [ ] **Step 2:** 在 `data.go` 中添加 Wire 绑定

**DoD:** `SkillProposalReader`/`SkillProposalWriter` 实现完成，`go build ./internal/data/...` 通过

**Phase 1 验证：** `go generate ./internal/data/ent/... && go build ./...`

---

## Phase 2 — Skill 生成与注册

### Task 5: 实现 SkillAutoCreator.GenerateSKILLMD

**Files:**
- Create: `internal/skill/auto_creator.go`

- [ ] **Step 1:** 创建 `auto_creator.go`，实现 `SkillAutoCreator` 结构体（modelProvider + rootDir），`GenerateSKILLMD` 方法调用 LLM 生成 SKILL.md（含 YAML front matter + Markdown body），30s 超时控制

**DoD:** `GenerateSKILLMD` 方法实现完成，LLM 调用含超时控制，`go build ./internal/skill/...` 通过

---

### Task 6: 实现 SkillAutoCreator.ValidateSKILLMD

**Files:**
- Modify: `internal/skill/auto_creator.go`

- [ ] **Step 1:** 实现 `ValidateSKILLMD` 方法，验证 SKILL.md 格式合规（YAML front matter 解析 + Markdown body 非空 + `skill.Repository.Get()` 解析验证）

**DoD:** `ValidateSKILLMD` 方法实现完成，能检测格式错误的 SKILL.md，`go build ./internal/skill/...` 通过

---

### Task 7: 实现 FileSystemSkillRegistrar

**Files:**
- Modify: `internal/skill/auto_creator.go`

- [ ] **Step 1:** 实现 `FileSystemSkillRegistrar` 结构体（rootDir + FSRepository），`RegisterSkill` 写入 `<rootDir>/<skillName>/SKILL.md` 并调用 `FSRepository.Refresh()`，`SkillExists` 检查名称冲突

**DoD:** `FileSystemSkillRegistrar` 实现完成，`RegisterSkill` 写入文件 + 刷新索引，`SkillExists` 检查冲突，`go build ./internal/skill/...` 通过

**Phase 2 验证：** `go test ./internal/skill/... -run TestSkillAutoCreator -count=1`

---

## Phase 3 — 业务闭环

### Task 8: 实现 SkillEvolutionUsecase.DetectAndPropose

**Files:**
- Create: `internal/biz/skill_evolution.go`

- [ ] **Step 1:** 创建 `skill_evolution.go`，实现 `SkillEvolutionUsecase` 结构体，`DetectAndPropose` 方法：查询 `biz.ToolInvocationReader` 获取工具调用历史 → 分析频次 + 参数相似度 → 计算 pattern hash → 去重检查 → 调用 `SkillAutoCreator.GenerateSKILLMD` → 创建 SkillProposal

**DoD:** `DetectAndPropose` 方法实现完成，能检测重复模式并生成提议，`go build ./internal/biz/...` 通过

---

### Task 9: 实现 SkillEvolutionUsecase.ApproveProposal / RejectProposal

**Files:**
- Modify: `internal/biz/skill_evolution.go`

- [ ] **Step 1:** 实现 `ApproveProposal` 方法：校验 status=pending → 更新为 approved → 记录 ApprovedBy/ApprovedAt

- [ ] **Step 2:** 实现 `RejectProposal` 方法：校验 status=pending → 更新为 rejected → 记录 RejectedBy

**DoD:** `ApproveProposal`/`RejectProposal` 方法实现完成，状态流转正确，`go build ./internal/biz/...` 通过

---

### Task 10: 实现 SkillEvolutionUsecase.RegisterApproved

**Files:**
- Modify: `internal/biz/skill_evolution.go`

- [ ] **Step 1:** 实现 `RegisterApproved` 方法：校验 status=approved → 调用 `SkillRegistrationPort.RegisterSkill` → 更新 status=registered

**DoD:** `RegisterApproved` 方法实现完成，审批通过的 Skill 可注册到仓库，`go build ./internal/biz/...` 通过

**Phase 3 验证：** `go test ./internal/biz/... -run TestSkillEvolution -count=1`

---

## Phase 4 — 接入层

### Task 11: 新增 skill_evolution.proto + Service 层

**Files:**
- Create: `api/aranea/v1/skill_evolution.proto`
- Create: `internal/service/skill_evolution.go`

- [ ] **Step 1:** 定义 proto：`ListProposals`/`GetProposal`/`ApproveProposal`/`RejectProposal`/`TriggerDetection` RPC

- [ ] **Step 2:** 运行 `make api` 生成代码

- [ ] **Step 3:** 实现 Service 层，proto↔biz 映射

**DoD:** proto 定义 + Service 层实现完成，`make api && go build ./...` 通过

---

### Task 12: 新增 cronrunner 定时任务

**Files:**
- Create: `internal/cronrunner/jobs/skill_evolution.go`

- [ ] **Step 1:** 创建 `skill_evolution.go`，实现定时任务：遍历活跃 Agent → 调用 `SkillEvolutionUsecase.DetectAndPropose`

**DoD:** 定时任务实现完成，`go build ./internal/cronrunner/...` 通过

---

### Task 13: Wire DI 装配

**Files:**
- Modify: `internal/biz/biz.go`（ProviderSet）
- Modify: `cmd/admin/wire.go`（依赖注入）

- [ ] **Step 1:** 在 `biz.go` ProviderSet 中添加 `NewSkillEvolutionUsecase`

- [ ] **Step 2:** 在 `wire.go` 中添加 `SkillEvolutionUsecase` 构造参数

- [ ] **Step 3:** 运行 `make wire && make build`

**DoD:** Wire DI 装配完成，`make wire && make build` 通过

**Phase 4 验证：** `make api && make wire && make build`

---

## Phase 5 — 验证

### Task 14: 单元测试

**Files:**
- Create: `internal/biz/skill_evolution_test.go`
- Create: `internal/skill/auto_creator_test.go`

- [ ] **Step 1:** 编写 `SkillEvolutionUsecase` 单元测试：DetectAndPropose 去重、ApproveProposal 状态校验、RejectProposal 状态校验、RegisterApproved 调用 RegisterSkill

- [ ] **Step 2:** 编写 `SkillAutoCreator` 单元测试：GenerateSKILLMD 格式验证、ValidateSKILLMD 合规/不合规用例、FileSystemSkillRegistrar 写入+刷新

**DoD:** 所有单元测试通过，`go test ./internal/biz/... ./internal/skill/... -run TestSkillEvolution -count=1` 绿色

---

### Task 15: 集成测试

**Files:**
- Create: `internal/service/skill_evolution_test.go`

- [ ] **Step 1:** 编写端到端集成测试：触发模式检测 → 生成 SKILL.md → 审批 → 注册 → `skill.Repository.Summaries()` 可发现新 Skill

- [ ] **Step 2:** 验证 `skill_load` 可加载、`skill_run` 可执行新注册的 Skill

**DoD:** 集成测试通过，新 Skill 可被框架工具链正常加载和执行

**Phase 5 验证：** 端到端验证 + `make api && make wire && make build && make test && make lint`
