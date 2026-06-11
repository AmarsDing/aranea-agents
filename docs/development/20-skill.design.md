# Skill 技能模块 — 实现设计文档

> 对应需求：`20 skill.md`
> 架构参考：`20 skill struct design.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Skill 是可安装、可版本化的能力包，由文件资产（SKILL.md + 附件）组成，通过上下文注入方式增强 Agent 能力。Skill 不替代 Tool 执行语义，而是作为 Tool 的上游——在运行时由 Assembler 选出并注入 Agent 工具链。

核心链路：**注册（CRUD / 导入）→ 发布 → 启用 → 运行时路由 → trpc-agent-go Skill 工具链 → 执行追踪**。

---

## 二、Proto 层

### 2.1 已实现 Proto（20 RPC）

文件：`api/kratos/skill/v1/skill.proto`

```protobuf
service SkillService {
  rpc ListSkills(ListSkillsRequest) returns (ListSkillsResponse) {
    option (google.api.http) = { get: "/v1/skills" };
  }
  rpc GetSkill(GetSkillRequest) returns (Skill) {
    option (google.api.http) = { get: "/v1/skills/{id}" };
  }
  rpc CreateSkill(CreateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills" body: "*" };
  }
  rpc UpdateSkill(UpdateSkillRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}" body: "*" };
  }
  rpc PublishSkill(PublishSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/publish" };
  }
  rpc ToggleSkillEnabled(ToggleSkillEnabledRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}/enabled" body: "*" };
  }
  rpc DuplicateSkill(DuplicateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/duplicate" };
  }
  rpc DeleteSkill(DeleteSkillRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/skills/{id}" };
  }
  rpc ListSkillFiles(ListSkillFilesRequest) returns (ListSkillFilesResponse) {
    option (google.api.http) = { get: "/v1/skills/{id}/files" };
  }
  rpc GetSkillFile(GetSkillFileRequest) returns (SkillFile) {
    option (google.api.http) = { get: "/v1/skills/{id}/file" };
  }
  rpc UpdateSkillFile(UpdateSkillFileRequest) returns (SkillFile) {
    option (google.api.http) = { put: "/v1/skills/{id}/file" body: "*" };
  }
  rpc DeleteSkillFile(DeleteSkillFileRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/skills/{id}/files:delete" body: "*" };
  }
  rpc PreviewSkillRuntime(PreviewSkillRuntimeRequest) returns (PreviewSkillRuntimeResponse) {
    option (google.api.http) = { get: "/v1/skill-runtime-preview" };
  }
  rpc ListSkillRuns(ListSkillRunsRequest) returns (ListSkillRunsResponse) {
    option (google.api.http) = { get: "/v1/skill-runs" };
  }
  // ImportSkillZip: multipart POST /v1/skills/import via RegisterSkillImportMultipart
  rpc ImportSkillZip(google.protobuf.Empty) returns (ImportSkillZipResponse);
  rpc GetSkillImportJob(GetSkillImportJobRequest) returns (SkillImportJob) {
    option (google.api.http) = { get: "/v1/skills/import/{job_id}" };
  }
  rpc ApplySkillImport(ApplySkillImportRequest) returns (SkillImportApplyResult) {
    option (google.api.http) = { post: "/v1/skills/import/{job_id}/apply" body: "*" };
  }
  rpc RefineSkillImportConflict(RefineSkillImportConflictRequest) returns (SkillRefineResult) {
    option (google.api.http) = {
      post: "/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine"
      body: "*"
    };
  }
}
```

### 2.2 ZIP 导入 multipart 端点

文件：`internal/service/skill_import_http.go`（由 `internal/server/http.go` 挂载）

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v1/skills/import` | POST | 上传 ZIP（multipart；`ImportSkillZip` 无 HTTP annotation） |
| `/v1/skills/import/{job_id}` | GET | 轮询导入状态 |
| `/v1/skills/import/{job_id}/apply` | POST | 应用导入结果 |
| `/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` | POST | AI 炼化冲突组 |

### 2.3 已实现版本 RPC

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetSkillVersions` | `GET /v1/skills/{id}/versions` | 版本历史列表（分页） |
| `RollbackSkillVersion` | `POST /v1/skills/{id}/versions/{version}/rollback` | 版本回滚（不可变策略：新建版本 + patch 递增） |

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/skill.go

type Skill struct {
    ID                 string
    SkillKey           string
    Name               string
    Description        string
    Status             string
    Enabled            bool
    Kind               string
    RiskLevel          string
    EntryPath          string
    FilesystemMissing  bool
    ConfigJSON         string
    MetadataJSON       string
    BodyMarkdown       string
    Version            string
    Tags               []SkillTag
    Permissions        *SkillPermissions
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type SkillTag struct {
    Name   string
    Source string
}

type SkillPermissions struct {
    CanEdit   bool
    CanDelete bool
    CanView   bool
}

type SkillVersionSummary struct {
    ID        string
    Version   string
    Status    string
    CreatedAt time.Time
}
```

### 3.2 SkillRepo 接口

`Repo` 已拆分为 `SkillReader` + `SkillWriter` 窄接口（见 §5.1），完整方法签名见 `internal/biz/skill/skill.go`。

### 3.3 SkillUsecase

```go
// internal/biz/skill.go

type SkillUsecase struct {
    repo SkillRepo
    // ...
}

func (uc *SkillUsecase) ListSkills(ctx, query) (SkillListResult, error)
func (uc *SkillUsecase) GetSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) CreateSkill(ctx, input) (Skill, error)
func (uc *SkillUsecase) UpdateSkill(ctx, id, draft) (Skill, error)
func (uc *SkillUsecase) PublishSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) ToggleSkillEnabled(ctx, id, enabled) (Skill, error)
func (uc *SkillUsecase) DuplicateSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) DeleteSkill(ctx, id) error
func (uc *SkillUsecase) ListSkillFiles(ctx, id) ([]SkillFileEntry, error)
func (uc *SkillUsecase) GetSkillFile(ctx, id, path) (SkillFileContent, error)
func (uc *SkillUsecase) UpdateSkillFile(ctx, id, path, content) error
func (uc *SkillUsecase) DeleteSkillFile(ctx, id, path) error
func (uc *SkillUsecase) PreviewRuntime(ctx) (SkillRuntimePreview, error)
func (uc *SkillUsecase) ListSkillRuns(ctx, query) (SkillRunResult, error)
func (uc *SkillUsecase) ListEnabledPublishedSkillKeys(ctx) ([]string, error)
func (uc *SkillUsecase) ListEnabledPublishedSkillCandidates(ctx) ([]SkillRuntimeCandidate, error)
func (uc *SkillUsecase) RecordSkillInvocation(ctx, write) error
func (uc *SkillUsecase) UpsertSkillFromDisk(ctx, input) (Skill, error)
func (uc *SkillUsecase) MarkSkillFilesystemMissing(ctx, slug, missing) error
func (uc *SkillUsecase) ListVersions(ctx, query) (VersionListResult, error)
func (uc *SkillUsecase) RollbackVersion(ctx, skillID, version) (Skill, error)
func (uc *SkillUsecase) BatchGetSkillGuidance(ctx, slugs) ([]SkillGuidanceEntry, error)
func (uc *SkillUsecase) ScoreByEmbedding(ctx, query, candidates) (map[string]float64, error)
func (uc *SkillUsecase) InvalidateEmbedCache()
```

### 3.4 运行时策略模型

```go
// internal/biz/skill_runtime.go

type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool
    IntentMaxPaths       int
    MaxSkillsInToolset   int
}

type SkillRuntimeCandidate struct {
    Slug          string
    Name          string
    Description   string
    Tags          []SkillTag
    TaxonomyPaths []string
}

func ParseSkillRuntimePolicy(jsonStr string) (SkillRuntimePolicy, error)
```

### 3.5 导入 DTO

```go
// internal/biz/skill_import.go

type SkillImportJob struct {
    JobID          string
    Status         string
    TotalCandidates int
    ConflictGroups []SkillImportConflictGroup
}

type SkillImportCandidate struct {
    Slug        string
    Name        string
    Description string
    Source      string
    Similarity  *SkillSimilarityMetrics
}

type SkillImportConflictGroup struct {
    GroupID     string
    Slug        string
    Candidates  []SkillImportCandidate
    Resolution  string
}

type SkillSimilarityMetrics struct {
    NameSimilarity       float64
    DescriptionSimilarity float64
    OverallSimilarity    float64
}

type SkillImportRefineRequest struct {
    JobID   string
    GroupID string
    Action  string
}

type SkillImportApplyRequest struct {
    JobID    string
    Resolutions map[string]string
}
```

### 3.6 SkillSimilarityEngine — 统一相似度引擎

文件：`internal/biz/skill_similarity.go`

统一平台级去重与导入流程的相似度计算，替代原先仅用 Name/Description 2 维 Jaccard 的简单算法。

**4 维相似度计算**：

| 维度 | 权重 | 说明 |
|------|------|------|
| Name | 0.4 | 名称 Jaccard 相似度 |
| Description | 0.25 | 描述 Jaccard 相似度 |
| Body | 0.25 | 正文段落标题 Jaccard 相似度 |
| Tag | 0.1 | 标签 Jaccard 相似度 |

**可选 Embedding 增强**：当注入 `DedupEmbedder` 时，将 Jaccard 分数与 Embedding 余弦相似度按 `embeddingBlendWeight=0.5` 混合。

**阈值常量**：

| 常量 | 值 | 用途 |
|------|-----|------|
| `similarityHighThreshold` | 0.8 | 总分 ≥ 此值 → `high` 风险，建议合并 |
| `similarityNameHighThreshold` | 0.9 | 名称分 ≥ 此值 → 极可能重复 |
| `similarityMediumThreshold` | 0.5 | 总分 ≥ 此值 → `medium` 风险，建议审查 |
| `embeddingBlendWeight` | 0.5 | Embedding 与 Jaccard 混合权重 |

**核心类型**：

```go
type SkillSimilarityEngine struct {
    embedder DedupEmbedder  // 可选，nil 时纯 Jaccard
    logger   loggateway.Logger
}

type SimilarityResult struct {
    TotalScore    float64
    NameScore     float64
    Dimensions    []DimensionScore
    ConflictRisk  string   // "high" / "medium" / "low"
    Recommendation string  // "merge" / "review" / "distinct"
}

func (e *SkillSimilarityEngine) Compare(ctx, target, candidate SkillDedupCandidate) (SimilarityResult, error)
```

**与导入流程的关系**：导入流程的 `SkillSimilarityMetrics`（6 维）仍独立运行于导入引擎中；平台级去重通过 `SkillSimilarityEngine` 统一为 4 维 + 可选 Embedding 增强。两套体系共享 Jaccard 核心算法，维度差异源于场景不同（导入需要更精细的来源/结构比对，平台级关注内容语义重叠）。

### 3.7 SkillMergeUsecase — 三阶段合并

文件：`internal/biz/skill_merge.go`、`internal/biz/skill_merge_ai_fuser.go`

替代原先 `SkillDedupUsecase.MergeSkills` 的粗暴合并（仅转移调用 + 废弃源 Skill），实现真正的三阶段合并流程。

**三阶段流程**：

```
阶段1: 内容融合 → 阶段2: Gate 验证 → 阶段3: 事务应用
```

**阶段 1 — 内容融合**（`SkillContentFuser` 接口）：

| 策略 | 说明 |
|------|------|
| `append` | 以 target body 为主体，从 source body 提取 target 没有的 `##` 段落追加 |
| `ai_fuse` | 预留 AI 融合扩展点（当前 fallback 到 append） |
| `manual_pick` | 手动选择保留哪一方内容 |

当前实现：`RuleBasedContentFuser`，基于 `##` 段落标题去重合并。

**阶段 2 — Gate 验证**：融合后的内容需通过校验（如非空、长度限制等），验证失败则拒绝合并。

**阶段 3 — 事务应用**（`SkillMergeWriter` 接口）：

在单个事务内执行 4 步操作：
1. 为 target Skill 创建新版本（含融合后 body）
2. 更新 target Skill 的 metadata/tags
3. 转移 source Skill 的调用记录到 target
4. 废弃源 Skill（状态 → `deprecated`）

**核心类型**：

```go
type SkillMergeUsecase struct {
    fuser  SkillContentFuser
    reader SkillMergeReader
    writer SkillMergeWriter
    logger loggateway.Logger
}

type SkillMergeRequest struct {
    TargetID  string
    SourceID  string
    Strategy  MergeStrategy  // append / ai_fuse / manual_pick
}

func (uc *SkillMergeUsecase) Merge(ctx, req SkillMergeRequest) (*SkillMergeResult, error)
```

**Data 层实现**：`internal/data/skill_merge.go` — `SkillMergeRepo` 实现 `SkillMergeReader` + `SkillMergeWriter`，事务内 4 步操作。

### 3.8 SkillEvolutionOrchestrator — 统一进化编排

文件：`internal/biz/skill_evolution_unified.go`、`internal/biz/skill_evolution_triggers.go`

统一原先三条进化管线（`SkillEvolutionUsecase` / `SkillIntelligenceUsecase` / `EvolutionUsecase`），解决 `EvolutionCoordinator` 的 TOCTOU 竞态问题。

**EvolutionTrigger 接口**（策略模式）：

| Trigger | 来源 | 检测逻辑 |
|---------|------|----------|
| `PatternTrigger` | 工具调用 Pattern | 从高频工具调用组合中检测新 Skill 需求，**返回所有匹配 pattern**（非仅第一个） |
| `HealthTrigger` | 健康指标 | 检测 30d 失败率 > 30% 或 score < 60；依赖 `SkillScorer` 窄接口（非具体类型） |
| `AgentConfigTrigger` | Agent 配置 | 预留扩展点（当前返回 nil），保留注册以支持未来扩展 |

**SkillEvolutionOrchestrator**：

- `RegisterTrigger`：**线程安全**（`sync.RWMutex` 保护 `triggers` 切片）
- `CheckAndCreate`：原子化检查 + 创建，解决 TOCTOU 竞态
  - 先调用 `UnifiedEvolutionReader.HasPendingForTarget` 检查
  - 遍历触发器时使用 **快照读取**（`RLock` → copy → `RUnlock`），避免长持锁
  - 每个 trigger 返回 `[]UnifiedEvolutionSuggestion`（支持多 pattern 同时触发）
  - 不存在则创建，DB UNIQUE 约束兜底（多实例并发安全）
  - 重复创建时返回 `nil, nil`（幂等）
- `Approve` / `Reject`：审批/拒绝进化建议，使用 `kerrors.BadRequest` 返回业务错误
- `ExpirePending`：**已实现**，调用 `UnifiedEvolutionWriter.ExpireOlderThan` 批量过期超过 7 天的 pending 建议

**接口拆分**（符合"接口方法 ≤ 5"规范）：

- `UnifiedEvolutionReader`（6 方法）：`HasPendingForTarget` / `GetLatestByTarget` / `ListByTarget` / `CountByTarget` / `GetByID` / `ListAllPending`
- `UnifiedEvolutionWriter`（6 方法）：`Create` / `UpdateStatus` / `UpdateDraftBody` / `UpdateLifecycleStatus` / `UpdateSandboxResult` / `ExpireOlderThan`

**Data 层实现**：`internal/data/unified_evolution.go` — `UnifiedEvolutionRepo` 同时实现 Reader + Writer，使用 raw SQL + 读写分离（与 `skill_evolution.go` 保持一致，待 Ent schema 补齐后迁移）。

**UnifiedEvolutionSuggestion**（统一数据模型）：

```go
type UnifiedEvolutionSuggestion struct {
    ID               string
    TargetType       string          // "skill" / "agent"
    TargetID         string
    ActionType       string          // "create" / "merge" / "evolve" / "deprecate"
    TriggerSource    string          // "pattern" / "health" / "agent_config"
    SuggestedName    string
    SuggestedBody    string
    Status           string          // "pending" / "approved" / "rejected" / "applied" / "expired"
    Metadata         json.RawMessage // 不同 ActionType 的扩展数据
    CreatedAt        time.Time
}
```

**EvolutionCoordinator 状态**：已标记 `deprecated`，`HasPendingEvolution` 优先委托 `SkillEvolutionOrchestrator`，失败时 fallback 到 legacy 逻辑。`SetCoordinator` 使用 `sync.Once` 保护，多次调用 panic。

**SkillDedupUsecase.MergeSkills**：已标记 `Deprecated`，应使用 `SkillMergeUsecase.Merge`（三阶段事务性合并）。Service 层不再回退到旧合并。

**SkillDedupUsecase.DetectDuplicateGroups**：添加 **10 分钟 TTL 内存缓存**，避免每次 API 调用全量 O(n²) 扫描。外部可通过 `InvalidateDedupCache()` 手动失效。

### 3.9 ScoreSkill 四维权重修复

文件：`internal/biz/skill_intelligence.go`

原先 `ScoreSkill` 的 4 维权重中 Token(0.2) 和 Feedback(0.15) 永远为 0，实际只有 SuccessRate(0.4) + Duration(0.25) 生效。

**修复后逻辑**：

| 维度 | 权重 | 启用条件 |
|------|------|----------|
| SuccessRate | 0.4 | 始终启用 |
| Duration | 0.25 | 始终启用 |
| Token | 0.2 | `AvgTokenUsage > 0` 时启用 |
| Feedback | 0.15 | `FeedbackScore > 0` 时启用 |

**Token 归一化**：`normalizeTokenUsage(avgTokenUsage)` — 以 `baselineTokens=2000` 为基准，计算 `1 - avg/baseline`，值域 [0, 1]。

**Feedback 启发式计算**（标注 `TEMPORARY`，待接入真实用户反馈）：

```go
func computeHeuristicFeedbackScore(successRate float64, avgDuration float64, avgTokenUsage int) float64
```

基于 SuccessRate/Duration/TokenUsage 启发式估算，作为真实 Feedback 数据缺失时的过渡方案。

**SkillHealthMetrics 扩展**：新增 `AvgTokenUsage int` 和 `FeedbackScore float64` 字段，Data 层从 `token_usage` JSON 中提取 `total` 字段计算平均值。

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/platform_skill.go` — Skill 主表
  - 关键字段：`skill_key`、`name`、`description`、`status`、`enabled`、`kind`、`risk_level`、`entry_path`、`filesystem_missing`、`config_json`、`metadata_json`
- `internal/data/ent/schema/skill_version.go` — 版本表
  - 关键字段：`skill_id`、`version`、`status`、`content_markdown`、`manifest_json`、`published_at`、`validation_status`
- `internal/data/ent/schema/skill_invocation.go` — 调用记录表
  - 关键字段：`skill_version`、`user_id`、`session_id`、`duration_ms`、`source`、`activation_id`

### 4.2 SkillRepo 实现

文件：`internal/data/skill.go`

关键方法：
- `SearchSkills`：支持搜索、标签、启用、状态筛选、分页；`published` 与历史 `active` 等同处理
- `CreateSkillWithVersion`：创建草稿 + 初始版本
- `PatchSkill`：可选字段更新（仅 `draft` 状态）
- `PublishSkill`：`draft` → `published`
- `UpsertSkillFromDisk`：磁盘同步（全量扫描 / fsnotify 增量）
- `ListEnabledPublishedSkillCandidates`：返回运行时候选（含 `TaxonomyPaths`）
- `MarkSkillFilesystemMissing`：标记/清除磁盘缺失状态

---

## 五、运行时层

### 5.1 装配入口

文件：`internal/agent/trpc_build.go` → `buildSkillDeps`

流程：
1. `ListEnabledPublishedSkillKeys()` 确认存在已启用 + 已发布 Skill
2. 优先 `SkillDBRepo`（`DBRepositoryAdapter`）；否则 `FSRepositoryAdapter`
3. `skillruntime.NewAgentVisibilityFilter(SkillUC, ag.Settings)` — Layer A/B，按 invocation 读取 turn query
4. `CodeExecutor`（local / docker，`CODE_EXECUTOR_BACKEND`；产出物经 `artifact_executor.go`）
5. `WithSkills` + `WithSkillFilter` + `WithSkillToolProfile(SkillToolProfileFull)`

Turn query 注入：`internal/service/trpc_turn.go` · `internal/team/runner_team_trpc.go` → `skillruntime.RunOptionWithTurnQuery`

### 5.2 运行时路由

文件：`internal/tools/skillruntime/resolve.go`

两级筛选：

**Layer A**（`applyLayerA`）：
- 按 `SkillRuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤

**Layer B**（`ResolveSkillSlugsDetailed`）：
- `skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配
- `filterByIntentPathsWithReasons()` → 按分类路径缩小候选
- `filterByAllTagsWithReasons()` → 按 `AllowedTags` + `ExtractTagHints()` 过滤
- `scoreCandidatesWithReasons()` → 按分类路径匹配度评分
- **Embedding 语义精排**（可选）：`SkillUsecase.ScoreByEmbedding(query, candidates)` → 余弦相似度融合评分
- 排序后取 `MaxSkillsInToolset`，返回 `ResolveResult{Slugs, Reasons}`

### 5.3 意图路由与分类

文件：`internal/tools/skillrouter/`

- `detect.go`：`DetectIntentPaths(userQuery, maxPaths)` → 关键词 → 分类路径
- `taxonomy.go`：`TaxonomyLeaves` 定义 + `ExtractTagHints(userQuery)` 提取 `file_type:*` / `domain:*` 提示

### 5.4 trpc-agent-go 桥接

文件：`internal/skill/trpc/`

- `repository.go`：`FSRepositoryAdapter` — 磁盘 FS → `trpcskill.Repository`
- `db_repository.go`：`DBRepositoryAdapter` — DB + TTL 缓存 → `trpcskill.Repository`
- `filter.go`：`NewFilteredRepository(base, allowedSlugs)` → `trpcskill.ContextRepository`
- `tools.go`：`BuildSkillTools()` 产出 4 个内置 Skill 工具（Load / Run / ListDocs / SelectDocs）
- `executor.go`：`CodeExecutor` 适配（local / docker）；`artifact_executor.go`：产出物 `WrapWithArtifactSave`

### 5.5 运行时策略存储

文件：`internal/biz/skill_runtime.go`

存储在 `agent_runtime_settings.skill_runtime_json`，字段：
- `allowed_slugs`、`denied_slugs`、`allowed_tags`
- `intent_routing_enabled`（默认 true）
- `intent_max_paths`（默认 3）
- `max_skills_in_toolset`（默认 32，上限 256）
- `embedding_scoring_enabled`（默认 false）— 启用 embedding 语义精排
- `embedding_score_weight`（默认 0.3，范围 0~1）— embedding 分权重

### 5.6 Prompt 注入（方式 C）

文件：`internal/agent/skill_guidance_inject.go`

在 `productCallbackChain` 中注册 `newSkillGuidanceBeforeHook`（priority=5），仅 `SkillsUseFullProfile` 模式下启用。

流程：
1. `ResolveSkillSlugsDetailed` 获取当前 turn 的 skill slugs
2. `BatchGetSkillGuidance(slugs)` 批量获取 skill markdown（2 条 SQL：按 skill_key 查 Skill + 按 skill_id 查最新 Version）
3. `manifest.Parse` 解析 frontmatter → `render.SkillGuidance` 渲染指导内容
4. 拼接为 system message 注入 `args.Request.Messages` 头部
5. 截断保护：`maxSkillGuidanceChars=4000`；`written==0` 时不注入

### 5.7 Embedding 语义精排

文件：`internal/biz/skill/skill.go`（`ScoreByEmbedding`/`refreshEmbedCache`/`cosineSimilarity32`）

评分融合公式：`final_score = keyword_score + cosine_similarity × 1000 × embedding_score_weight`

- 默认 `embedding_score_weight=0.3`，最大 embedding 贡献 300 分
- 低于 taxonomy 精确匹配（1000）和部分匹配（400），高于关键词匹配（100）
- 仅在 `embedding_scoring_enabled: true` 时启用
- 内存缓存：`embedCache map[string][]float32`，按 slug 缓存 embedding
- 缓存失效：Publish/ToggleEnabled/Delete/Duplicate 时 `InvalidateEmbedCache()`
- 优雅降级：embedding 不可用时回退到纯关键词评分，`event.SysLogWarn` 记录失败

### 5.8 SkillFilesystem 端口

接口定义在 `internal/biz/skill/skill.go`（`SkillFilesystem`），实现在 `internal/skill/storage/filesystem.go`。

Service 层通过 `SkillFilesystem` 端口访问文件系统，不直接操作 `os` 包。Wire 绑定：`ProvideSkillResolveRootFn`（提供 `func(ctx) string`）+ `storage.NewSkillFilesystem`。

端口方法：
- `ResolveRoot(ctx)` — 解析存储根目录（动态：优先 SystemSettingRepo.root_directory，回落环境变量/默认路径）
- `CreateSkillDir(slug, body)` — 创建 skill 目录 + SKILL.md
- `ListFiles(dir)` — 遍历目录返回文件列表
- `ReadFile(dir, relPath)` — 读取文件内容（含安全检查 + 大小限制）
- `WriteFile(dir, relPath, content)` — 写入文件
- `DeleteFile(dir, relPath)` — 删除文件
- `RootAccessible(ctx)` — 检查根目录是否可访问
- `DirExists(dir)` — 检查目录是否存在（替代 Service 层直接 `os.Stat`）
- `SafeFilePath(dir, relPath)` — 路径安全检查（防目录穿越）

### 5.9 Repo 窄接口拆分

`Repo` 接口按职责拆分为 `SkillReader` + `SkillWriter`，`Repo` 组合两者保持向后兼容：

- **SkillReader**（13 方法）：`SearchSkills`/`GetSkillByID`/`GetSkillBySkillKey`/`GetSkillStorageDir`/`GetLatestSkillMarkdown`/`BatchGetSkillMarkdownBySlugs`/`ListRegisteredSlugs`/`ListEnabledPublishedSkillKeys`/`ListEnabledPublishedSkillCandidates`/`ListSkillSimilaritySources`/`FilesystemHealthStats`/`SearchSkillInvocations`/`ListSkillVersions`
- **SkillWriter**（10 方法）：`CreateSkillWithVersion`/`UpdateSkillEnabled`/`DuplicateSkill`/`DeleteSkill`/`PatchSkill`/`PublishSkill`/`UpsertSkillFromDisk`/`MarkSkillFilesystemMissing`/`RecordSkillInvocation`/`RollbackSkillVersion`

新消费者应优先依赖窄接口（`SkillReader` 或 `SkillWriter`），仅同时需要读写时才使用 `Repo`。

---

## 六、磁盘监听与同步

### 6.1 文件系统监听

文件：`internal/skill/watch/runner.go`

- 启动全量扫描 + `fsnotify` 增量监听（debounce 2s）
- 环境变量 `SKILL_WATCH_DISABLED=1` 可关闭
- 支持 `event.Bus` 集成，同步成功后发布 `skill.reload` 事件
- 磁盘目录缺失时调用 `MarkSkillFilesystemMissing(slug, true)` 标记，恢复时清除

### 6.2 存储根解析

文件：`internal/skill/storage/root.go`

解析优先级（自上而下短路命中）：
1. `SKILL_ROOT` 环境变量
2. `SKILL_STORAGE_ROOT` 环境变量
3. `filepath.Join(Resolved(work_directory), "skills")`
4. 操作系统默认路径（`%AppData%\Aranea\skills` 等）

`ResolveRootWithPlatform(rootDirectory)` 实现完整回落链路。

---

## 七、ZIP 导入

### 7.1 导入引擎

文件：`internal/skill/importer/`

子包：
- `engine.go`：导入主流程（解压 → 校验 → 相似度检测 → 冲突分组）
- `validate.go`：ZIP 结构与 frontmatter 校验
- `helpers.go`：辅助函数
- `chat.go`：LLM 相似度检测与炼化
- `errors.go`：错误类型

### 7.2 HTTP 路由

业务逻辑在 `internal/service/skill_import.go`；multipart 挂载见 §2.2。

---

## 八、Service 层

文件：`internal/service/skill.go`

薄适配层，职责：
- Proto Request → Biz DTO 转换
- `resolvedStorageRoot()`：通过 `SystemSettingRepo.Get()` 读取 `work_directory` → `storage.ResolveRootWithPlatform()`
- `safeSkillFilePath()`：路径安全校验（禁止 `..` 跳出 Skill 根）

已实现方法：
```go
func (s *SkillService) ListSkills(ctx, req) (*ListSkillsResponse, error)
func (s *SkillService) GetSkill(ctx, req) (*Skill, error)
func (s *SkillService) CreateSkill(ctx, req) (*Skill, error)
func (s *SkillService) UpdateSkill(ctx, req) (*Skill, error)
func (s *SkillService) PublishSkill(ctx, req) (*Skill, error)
func (s *SkillService) ToggleSkillEnabled(ctx, req) (*Skill, error)
func (s *SkillService) DuplicateSkill(ctx, req) (*Skill, error)
func (s *SkillService) DeleteSkill(ctx, req) (*emptypb.Empty, error)
func (s *SkillService) ListSkillFiles(ctx, req) (*ListSkillFilesResponse, error)
func (s *SkillService) GetSkillFile(ctx, req) (*SkillFile, error)
func (s *SkillService) UpdateSkillFile(ctx, req) (*SkillFile, error)
func (s *SkillService) DeleteSkillFile(ctx, req) (*emptypb.Empty, error)
func (s *SkillService) PreviewSkillRuntime(ctx, req) (*PreviewSkillRuntimeResponse, error)
func (s *SkillService) ListSkillRuns(ctx, req) (*ListSkillRunsResponse, error)
func (s *SkillService) GetSkillVersions(ctx, req) (*GetSkillVersionsResponse, error)
func (s *SkillService) RollbackSkillVersion(ctx, req) (*Skill, error)
```

---

## 九、Wire 注入

已有，无需新增。Skill 相关依赖通过 `wire.NewSet` 注入：
- `SkillRepo` → `SkillUsecase` → `SkillService`
- `SkillUsecase` → `buildSkillDeps`（Agent 构建）

---

## 十、Web 前端设计

### 10.1 文件结构（与代码一致）

```
web/src/
├── pages/
│   ├── SkillsPage.vue              # 列表 + SkillUploadPlaceholder + SkillEditorDialog
│   └── SkillRunsPage.vue
├── pages/agent-settings/
│   └── AgentSettingsSkillsTab.vue  # skill_runtime_json
├── components/skills/
│   ├── SkillTable.vue · SkillFilterBar.vue · SkillStatsStrip.vue
│   ├── SkillEditorDialog.vue · SkillUploadPlaceholder.vue
│   ├── SkillDeleteDialog.vue · SkillRunsTable.vue · SkillPagination.vue
├── features/skills/
│   ├── api.ts · types.ts
└── stores/skills/index.ts
```

路由：`/skills` · `/skills/runs`（见 `frontend-pages.md` §4.6）

**SkillEditorDialog.vue** — 全屏 Dialog，左侧文件树 + 右侧内容编辑（`ListSkillFiles` / `GetSkillFile` / `UpdateSkillFile`）。

**SkillUploadPlaceholder.vue** — 上传 zip、轮询导入任务、冲突组炼化（调用 `features/skills/api` import 端点）。

### 10.3 API

```typescript
export async function listSkills(query: SkillListQuery): Promise<SkillListResult>
export async function getSkill(id: string): Promise<Skill>
export async function createSkill(req: CreateSkillRequest): Promise<Skill>
export async function updateSkill(id: string, req: UpdateSkillRequest): Promise<Skill>
export async function publishSkill(id: string): Promise<Skill>
export async function toggleSkillEnabled(id: string, enabled: boolean): Promise<Skill>
export async function duplicateSkill(id: string): Promise<Skill>
export async function deleteSkill(id: string): Promise<void>
export async function listSkillFiles(id: string): Promise<SkillFileEntry[]>
export async function getSkillFile(id: string, path: string): Promise<SkillFile>
export async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFile>
export async function deleteSkillFile(id: string, path: string): Promise<void>
export async function previewSkillRuntime(): Promise<SkillRuntimePreview>
export async function listSkillRuns(query: SkillRunQuery): Promise<SkillRunResult>
export async function importSkillZip(file: File): Promise<SkillImportJob>
export async function getImportStatus(jobId: string): Promise<SkillImportJob>
export async function applyImport(jobId: string, resolutions: Record<string, string>): Promise<void>
export async function refineConflictGroup(jobId: string, groupId: string): Promise<void>
```


---

## 子模块：Skill Struct 设计

本文档描述 **aranea-agents** 仓库中 Skill 能力的**现状基线**、**目标形态**与**演进路径**。产品交互与 API 字段细节以 [`20 skill.md`](./20%20skill.md) 为准；平台分层、Context 边界与 trpc-agent-go 集成红线以 [`architecture/platform-architecture.md`](../architecture/platform-architecture.md) 为准。

---

## 零、文档定位

| 维度 | 说明 |
|------|------|
| **Skill 是什么** | 可安装、可版本化、带文件资产的能力包：说明书、约束、示例与触发条件；默认通过 **上下文注入** 影响 Agent，而不是替代 Tool 的执行语义。 |
| **Tool 是什么** | 模型可调用的可执行单元：参数校验、执行、副作用、返回值。 |
| **本文不写** | Quasar 页面文案与验收清单（见 `20 skill.md`）；不把 Skill **等同于** trpc-agent-go 内置 Skill 工具的每一个——运行时可以是「提示注入 + 可选 toolset」。 |

**一句话**：管理面由 **Kratos + SQLite（Ent）+ Skill 根目录文件** 承载；运行时按 Agent/会话选 Skill → trpc-agent-go 装配 → 记录 usage。

---

## 一、概念边界（与平台架构一致）

- Skill 归属 **Capability Context**（与 Tool / MCP / Provider 并列），详见 `platform-architecture.md` 第一篇 §4、第三篇 §6「skill_bridge」示意。
- **跨 Context 规则**：Catalog / Conversation 只能通过 **`kernel/contracts`**（未来端口）查询 Skill 视图；按 [`platform-architecture.md`](../architecture/platform-architecture.md) **目标态**，**禁止**在非专用运行时适配包内 import `google.golang.org/adk`（当前仓库仍存在历史调用点如 `internal/tools/catalog`，迁移时应收敛）。
- **依赖方向（目标态）**：`proto → internal/service → internal/biz → internal/data`；导入等特殊路由可走同一服务的 HTTP 挂载，但业务逻辑应落在 **biz**，持久化在 **data**。
- Skill **默认不执行副作用**；需要读写文件、调用外部 API 时走 **Tool** 或显式「SkillAction」类扩展（若引入）。

---

## 二、现状基线（本仓库已实现）

### 2.1 代码与接口落点

| 层级 | 路径 | 职责 |
|------|------|------|
| API | `api/kratos/skill/v1/skill.proto` | SkillService：**20 RPC**（含 Import 4 个 + 版本 2 个） |
| 服务 | `internal/service/skill.go` | Proto ↔ biz 转换；`resolvedStorageRoot()` 对接系统设置 |
| 用例 | `internal/biz/skill/`（`skill.go` · `skill_import.go` · `skill_runtime.go`） | 列表校验、Skill CRUD 端口定义、导入请求类型、运行时策略与候选模型 |
| 数据 | `internal/data/skill.go` | Ent 实现的 `SkillRepo`：查询、聚合统计、存储目录、草稿/发布、磁盘同步等 |
| Schema | `internal/data/ent/schema/platform_skill.go`（表名 `skill`）、`skill_version.go`、`skill_invocation.go` | 与 DB 表映射 |
| 导入 | `internal/skill/importer/*` · `internal/service/skill_import.go` · `skill_import_http.go` | ZIP 导入；3/4 端点 proto codegen + multipart POST |
| 监听 | `internal/skill/watch/runner.go` | `fsnotify` + debounce 目录监听；启动全量扫描；发布 `skill.reload` 事件 |
| 存储 | `internal/skill/storage/root.go` | 解析 Skill 文件根目录；**已实现**环境变量 + 系统设置 `work_directory` + OS 默认三级回落 |
| trpc 桥接 | `internal/skill/trpc/*`（repository / db_repository / tools / executor / filter） | FS 适配器 + DB 适配器 → trpc-agent-go `skill.Repository`；Skill Tool 构建；FilteredRepository；CodeExecutor 适配 |
| 运行时路由 | `internal/tools/skillrouter/*`（detect / taxonomy） | 用户意图 → 分类路径匹配；关键词路由；标签提示提取 |
| 运行时装配 | `internal/tools/skillruntime/*` · `internal/agent/trpc_build.go` | Layer A/B → `NewAgentVisibilityFilter`；turn query 经 RuntimeState |
| Agent 集成 | `internal/agent/trpc_build.go` | `buildSkillDeps(ag, deps)` → `WithSkills` / `WithSkillFilter` / `WithCodeExecutor` |
| 系统设置 | `api/kratos/system_setting/v1/system_setting.proto`、`internal/service/system_setting.go` | 单例 **`work_directory`**（前端「系统设置」页）；已用作 Skill 根路径推导锚点 |

### 2.2 `SkillService`（proto）已暴露的能力

已实现 RPC（共 **20** 个）：

| RPC | HTTP | 说明 |
|-----|------|------|
| `ListSkills` | `GET /v1/skills` | 列表（搜索、标签、启用、状态筛选、分页） |
| `GetSkill` | `GET /v1/skills/{id}` | 详情（含 `body_markdown`） |
| `CreateSkill` | `POST /v1/skills` | 创建草稿（写 SKILL.md + 入库） |
| `UpdateSkill` | `PATCH /v1/skills/{id}` | 更新草稿（元数据 + 正文可选更新） |
| `PublishSkill` | `POST /v1/skills/{id}/publish` | 发布（`draft` → `published`） |
| `ToggleSkillEnabled` | `PATCH /v1/skills/{id}/enabled` | 启用/停用 |
| `DuplicateSkill` | `POST /v1/skills/{id}/duplicate` | 复制为草稿 |
| `DeleteSkill` | `DELETE /v1/skills/{id}` | 软删 |
| `ListSkillFiles` | `GET /v1/skills/{id}/files` | 文件列表 |
| `GetSkillFile` | `GET /v1/skills/{id}/file` | 文件内容 |
| `UpdateSkillFile` | `PUT /v1/skills/{id}/file` | 文件更新 |
| `DeleteSkillFile` | `POST /v1/skills/{id}/files:delete` | 文件删除（禁止删 SKILL.md） |
| `PreviewSkillRuntime` | `GET /v1/skill-runtime-preview` | 运行时装配预览 |
| `ListSkillRuns` | `GET /v1/skill-runs` | 运行记录分页 |
| `GetSkillVersions` | `GET /v1/skills/{id}/versions` | 版本历史列表 |
| `RollbackSkillVersion` | `POST /v1/skills/{id}/versions/{version}/rollback` | 版本回滚 |

此外，Import 链路：

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/skills/import` | POST | multipart ZIP（`RegisterSkillImportMultipart`；proto 占位 `ImportSkillZip`） |
| `/v1/skills/import/{job_id}` | GET | 轮询 |
| `/v1/skills/import/{job_id}/apply` | POST | 应用 |
| `/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` | POST | 炼化 |

### 2.3 与 `20 skill.md` 的差距（产品契约）

以下能力在需求文档中已约定，对照当前实现状态：

| 能力 | 状态 | 说明 |
|------|------|------|
| 创建草稿 | ✅ 已实现 | `CreateSkill` RPC + `CreateSkillWithVersion` data |
| 更新草稿 | ✅ 已实现 | `UpdateSkill` RPC + `PatchSkill` data（可选字段更新） |
| 发布 | ✅ 已实现 | `PublishSkill` RPC + `PublishSkill` data（`draft` → `published`） |
| 文件删除 | ✅ 已实现 | `DeleteSkillFile` RPC（禁止删 SKILL.md） |
| 运行时装配预览 | ✅ 已实现 | `PreviewSkillRuntime` RPC（返回已启用 slug 列表 + 存储根） |
| `SkillDetail` 级响应 | ✅ 已实现 | `GetSkill` 返回 `Skill` + `body_markdown` |
| 版本历史 / 回滚 | ✅ 已实现 | `GetSkillVersions` + `RollbackSkillVersion`；不可变策略（新建版本 + patch 递增 + 事务保护） |
| RBAC 权限 | ✅ 已实现 | `requireAdminAccess`（写操作门控）+ `applySkillPermission`（读操作权限掩码）；未认证零权限 |
| Ent 字段补齐 | ✅ 已实现 | `visibility`/`default_config_json`/`file_manifest_json`/`message_id` 均已落地 |
| Prompt 注入（方式 C） | ✅ 已实现 | BeforeModelHook + `BatchGetSkillGuidance` 批量获取 + 截断 + 空 guidance 防护 |
| Embedding 语义精排 | ✅ 已实现 | `SkillEmbedder` + `ScoreByEmbedding` + 内存缓存 + 评分融合 + 优雅降级 |
| Preview 选中原因 | ✅ 已实现 | `ResolveSkillSlugsDetailed` 返回 `Reasons map[string]string` + `agent_id` 关联 |
| manifest/render 包 | ✅ 已实现 | `internal/skill/manifest/` + `internal/skill/render/`；frontmatter 解析 + 变量替换 + prompt 渲染 |
| 自动负熵报告 | ❌ 未实现 | 本期只展示已有聚合指标（invoke/success/failure/avg_duration） |
| Budget 中间件 | ❌ 未实现 | token 上限裁剪（当前仅 `MaxSkillsInToolset` 数量限制） |
| Skill 依赖 / 冲突表 | ❌ 未实现 | 安装时检查 + 运行时互斥 |

导入链路见 §2.2；multipart 上传仍由 `RegisterSkillImportMultipart` 挂载（保留手动注册，已补齐 admin 校验 + 指标）。

### 2.4 运行时（Layer A + B 已接通）

**装配**（`internal/agent/trpc_build.go` → `buildSkillDeps`）：

1. 确认存在已启用 + 已发布 Skill。
2. Repository：`DBRepositoryAdapter` 优先，否则 `FSRepositoryAdapter`。
3. Filter：`skillruntime.NewAgentVisibilityFilter(SkillUC, ag.Settings)` — 每 invocation 调用 `ResolveSkillSlugs`。
4. Turn query：Chat/Team turn 通过 `skillruntime.RunOptionWithTurnQuery` 写入 RuntimeState。
5. `WithSkills` + `WithSkillFilter` + `WithSkillToolProfile(SkillToolProfileFull)`。

**Layer B**（`ResolveSkillSlugs` + `skillrouter`）：意图路径 · 标签合取 · 评分 · `MaxSkillsInToolset`。

**衔接方式**：方式 A（FS）+ 方式 B（DB）+ 方式 C（Prompt 注入）均已实现。

### 2.5 Skill 本地目录与系统设置「工作目录」

平台上 Skill **磁盘目录**应与用户在 **系统设置**里配置的 **工作目录**一致可追溯，避免「上传写到一处、运行时读到另一处」。

**系统设置契约（已实现）**

- Proto：`api/kratos/system_setting/v1/system_setting.proto` → 消息 `SystemSettings.work_directory`（注释：`Absolute or workspace-relative path used as the platform working directory`）。
- HTTP：`GET /v1/system-settings`、`PUT /v1/system-settings`。
- 持久化：`internal/data/ent/schema/system_setting.go` 单例行 **`work_directory`**。

**Skill 物理根路径（产品与实现对齐约定）**

| 项目 | 约定 |
|------|------|
| **Skill 根目录** | **`{ResolvedWorkDirectory}/skills`** —— 在工作目录下固定使用子目录名 **`skills`**，与 ZIP 导入解压目标、`skillstorage` 期望的根保持一致。 |
| **`ResolvedWorkDirectory`** | 服务端将用户填入的 `work_directory` 规范化为绝对路径（含 `~` 展开）；为空或未配置时的回落策略见下。 |

**解析优先级（已实现，自上而下短路命中）**

1. **`SKILL_ROOT`**（若设置 → 视作 Skill 根绝对路径）。
2. **`SKILL_STORAGE_ROOT`**（若设置 → 同上别名）。
3. **`filepath.Join(Resolved(work_directory), "skills")`** —— 仅当 **`work_directory` 非空** 且路径合法可读时使用。
4. **操作系统默认路径** —— 与 `storage.DefaultRoot(goos)` 一致（如 `%AppData%\Aranea\skills` 等）。

**已实现**：`storage.ResolveRootWithPlatform(rootDirectory)` 实现了 ①②③④ 完整回落链路。`SkillService.resolvedStorageRoot()`、`watch.Runner.resolveRoot()`、`importer.Engine.resolveRoot()` 均通过 `SystemSettingRepo.Get()` 读取 `work_directory` 后调用 `ResolveRootWithPlatform`。

运维或单机开发者仍可通过环境变量覆盖 ③④，便于 CI / 容器。

**注意事项**：前端「系统设置」保存成功后需提示：**更改工作目录后原有 `{旧路径}/skills` 下的 Skill 需迁移或重新索引**。ZIP 导入与文件读写均统一到同一解析函数。

### 2.6 目录监听 / 定时扫描 · 增量装载（磁盘 → 目录）

除 ZIP 导入与用户在前端编辑外，需要支持：**常驻监听 Skill 根目录**，在磁盘上出现新的合规 Skill 包时 **自动加载到系统目录登记**，无需重复手动导入。

**目标行为**

- Skill 根下每一个 **一级子目录**（或与现行导入一致的 slug 目录）若包含合规 **`SKILL.md`**（及规范允许的附带文件），应能通过后台同步进入 **`skill` / `skill_version`**（或与导入流水线共用同一校验与冲突策略）。
- 对已登记条目：**文件变更**可反映为新草稿版本或「待重新校验」；**目录删除**不建议硬删 DB，可标记 `archived` / `filesystem_missing`（字段名以迁移为准）。

**触发机制（可组合）**

| 方式 | 说明 |
|------|------|
| **定时扫描** | 服务端后台 **ticker**（间隔可配置，如 30s～5min）；比对子目录 **mtime / size / 内容 hash**，做增量 diff，避免每次全量暴力读大文件。 |
| **实时监听** | 在支持的 OS 上使用 **`fsnotify`**（或等价 API）监听 Skill 根及其一层或递归子树；对频繁保存 **debounce**（如 500ms～2s 合并事件）后再跑校验与入库。 |

**同步流水线（建议）**

```text
SkillRoot watcher/ticker
    → 枚举候选目录（跳过隐藏目录、临时前缀如 .）
    → 结构校验（与 20 skill.md / `internal/skill/importer` 一致）
    → slug/skill_key 幂等：已存在则比对 hash；仅变更则 upsert 版本或草稿
    → 可选：与「相似度 / 冲突组」策略对齐（磁盘导入可走轻量路径：同名目录视为同一 Skill）
    → 审计日志 / metrics：skill.fs.scan、skill.fs.synced、skill.fs.error
```

**与其它通路的关系**

- **ZIP 导入**：最终写入的仍是同一 Skill 根；扫描器应对同一 **`skill_key`/目录名** **幂等**，避免重复创建行。
- **权限与安全**：所有路径必须在 **解析后的 Skill 根** 之内，禁止 `..` 跳出；仅服务端进程执行扫描。

**现状**：`cmd/admin` 已集成 **`internal/skill/watch`**（启动全量扫描 + `fsnotify` debounce 2s；环境变量 **`SKILL_WATCH_DISABLED=1`** 可关闭）。`watch.Runner` 通过 **`NewRunnerWithBus`** 发布 `skill.filesystem.*` 与 `skill.reload`；磁盘目录缺失时 `MarkFilesystemMissing(slug, true)`；恢复时清除并 upsert。`metadata_json.sync_origin` 区分 `filesystem` / `import` / `manual`。目录名必须与 slug 一致（见 **`20 skill.md`** §2.4）。

**通知链路（P2.5，已实现 / 进行中）**

| 事件 | 触发 | 落点 |
|------|------|------|
| `skill.filesystem.imported` | 磁盘新建登记 | Monitor Events + EventBus |
| `skill.filesystem.updated` | 磁盘正文/metadata 变更 | 同上 |
| `skill.filesystem.missing` | 目录删除 | 同上 + Skill 页 Banner |
| `skill.filesystem.recovered` | 目录恢复 | 同上 |
| `skill.filesystem.rejected` | 校验失败（含 slug 目录名不一致） | 运行记录 + Monitor |

可选补充：**定时 reconcile ticker**（默认 5min，环境变量 `SKILL_FS_RECONCILE_INTERVAL`，`0/off` 关闭）；**Alert Rule** 外发 Webhook（`metric_key=skill.filesystem_missing_count`）。

**D5 行为（已实现）**

| 能力 | 说明 |
|------|------|
| Reconcile | 定时 scan 磁盘 + 对 DB 已登记 slug 补打 `filesystem_missing` |
| 回退 draft | 已发布 Skill 磁盘正文变更 → `draft` + `enabled=false` |
| 相似度 warn | 新磁盘 Skill 与已有 **同名** → `skill.filesystem.similarity_warn`（异步，非 LLM） |
| 告警 | Monitor 规则 `skill.filesystem_missing_count` ≥ threshold → Webhook/Channel |

---

## 三、目标分层架构（Capability · 对齐 Tools 思路）

在「平台目标态」下，Skill 与 Tool 共享同一种 **编排句式**：**registry → executor → middleware → backends**，差别在后端语义（加载/渲染 vs 执行）。

```text
┌─────────────────────────────────────────────────────────┐
│ Driving：HTTP(gRPC-Gateway) / CLI / Cron                   │
├─────────────────────────────────────────────────────────┤
│ Capability.application                                     │
│   · SkillCatalog（CRUD / version / import）               │
│   · SkillRuntimeAssembler（按回合选 Skill → 视图）        │
├──────────────────────────┬────────────────────────────────┤
│ Middleware               │ Executor                       │
│ · validation             │ · manifest / frontmatter 解析   │
│ · policy（enabled/status）│ · load files                  │
│ · budget（数量/token）    │ · render prompt block          │
│ · cache                  │ · record usage                 │
│ · tracing                │                                │
├──────────────────────────┴────────────────────────────────┤
│ Backends                                                   │
│ · MarkdownSkill（SKILL.md + assets）                        │
│ · PromptPack（多文件拼接）                                  │
│ · Workflow / Recipe（可选）                                 │
│ · Tool-backed（说明 + 触发 Tool，副作用仍在 Tool）          │
├────────────────────────────────────────────────────────────┤
│ Driven：Ent/SQLite · OS 文件 · LLM（相似度/炼化）· OTel     │
└────────────────────────────────────────────────────────────┘
```

**与 trpc-agent-go 的交界（A + B 已实现，C 待规划）**：

- **方式 A**：`FSRepositoryAdapter` → `WithSkills`
- **方式 B**：`DBRepositoryAdapter`（TTL 缓存）→ `WithSkills`
- **方式 C**（P4）：Prompt 块注入 system message；可选关闭内置 Skill 工具

装配在 `internal/agent/trpc_build.go`；`internal/skill/trpc/` 不 import 框架运行时。

---

## 四、Go 包布局（现状 + 规划）

当前仓库 Skill 相关包已基本成型，按增量演进原则维护：

```text
internal/
├── biz/
│   └── skill/                  # 用例子包
│       ├── skill.go            # 用例与端口（SkillReader/SkillWriter/Repo 接口、SkillUsecase、DTO、SkillFilesystem、SkillEmbedder）
│       ├── skill_import.go     # 导入 DTO（SkillImportJob/Candidate/ConflictGroup/SimilarityMetrics/RefineRequest/ApplyRequest）
│       ├── skill_runtime.go    # 运行时策略（SkillRuntimePolicy、SkillRuntimeCandidate、ParseSkillRuntimePolicy）
│       └── skill_test.go       # 单元测试
├── data/
│   └── skill.go                # Ent 仓储实现（skillRepo、enrichSkill、聚合查询、草稿/发布/磁盘同步/版本回滚）
├── skill/
│   ├── importer/               # ZIP 导入引擎（已实现：engine / validate / helpers / chat / errors）
│   ├── watch/                  # Skill 根目录监听与磁盘同步（已实现：runner，含 fsnotify + debounce + eventBus）
│   ├── storage/                # Skill 存储根解析 + SkillFilesystem 实现（已实现：root.go / filesystem.go）
│   ├── manifest/               # frontmatter / skill.json 解析与校验（已实现：manifest.go / manifest_test.go）
│   ├── render/                 # prompt 块渲染、截断策略（已实现：render.go / render_test.go）
│   └── trpc/                   # trpc-agent-go 桥接层（已实现：repository / db_repository / tools / executor / filter / artifact_executor）
├── tools/
│   ├── skillruntime/           # 运行时装配入口（已实现：toolset.go / resolve.go）
│   └── skillrouter/            # 意图路由与分类（已实现：detect.go / taxonomy.go）
├── service/
│   ├── skill.go                # 薄适配（16 RPC）
│   ├── skill_import.go         # 导入用例桥接
│   └── skill_import_http.go    # multipart POST /v1/skills/import
└── agent/
    ├── trpc_build.go           # Agent 构建中 Skill 装配（已实现：buildSkillDeps）
    └── skill_guidance_inject.go # Prompt 注入方式 C（BeforeModelHook + BatchGetSkillGuidance）
```

**规划新增**（不破坏现有包结构）：

| 包 | 规划内容 | 状态 |
|----|----------|------|
| `internal/skill/manifest/` | frontmatter / skill.json 解析与校验 | ✅ 已实现 |
| `internal/skill/render/` | prompt 块渲染、截断策略 | ✅ 已实现 |

若日后完整迁入「Capability Context」目录树，可将 `internal/skill/**` 整体映射为 `internal/capability/skill/**`（以迁移 playbook 为准）。

---

## 五、核心抽象（已实现 + 规划）

以下接口与类型用于统一「加载 → 路由 → 装配 → 记账」。

### 5.1 Skill 运行端口（已实现）

`Repo` 接口已按职责拆分为 `SkillReader` + `SkillWriter` 窄接口，`Repo` 组合两者保持向后兼容：

- **SkillReader**（组合 `SkillQueryReader` + `SkillLookupReader` + `SkillRuntimeReader`）：`SearchSkills`/`GetSkillByID`/`GetSkillBySkillKey`/`GetSkillStorageDir`/`GetLatestSkillMarkdown`/`BatchGetSkillMarkdownBySlugs`/`ListRegisteredSlugs`/`ListEnabledPublishedSkillKeys`/`ListEnabledPublishedSkillCandidates`/`ListSkillSimilaritySources`/`FilesystemHealthStats`/`SearchSkillInvocations`/`ListSkillVersions`
- **SkillWriter**（组合 `SkillMutationWriter` + `SkillSyncWriter`）：`CreateSkillWithVersion`/`UpdateSkillEnabled`/`DuplicateSkill`/`DeleteSkill`/`PatchSkill`/`PublishSkill`/`UpsertSkillFromDisk`/`MarkSkillFilesystemMissing`/`RecordSkillInvocation`/`RollbackSkillVersion`

新消费者应优先依赖窄接口（`SkillReader` 或 `SkillWriter`），仅同时需要读写时才使用 `Repo`。

完整方法签名见 `internal/biz/skill/skill.go`。

### 5.2 运行时策略与候选（已实现）

```go
// internal/biz/skill_runtime.go
type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool   // 默认 true
    IntentMaxPaths       int    // 默认 3
    MaxSkillsInToolset   int    // 默认 32，上限 256
}

type SkillRuntimeCandidate struct {
    Slug          string
    Name          string
    Description   string
    Tags          []SkillTag
    TaxonomyPaths []string
}
```

### 5.3 trpc-agent-go Repository 适配（已实现）

```go
// internal/skill/trpc/repository.go — FS 适配器
type FSRepositoryAdapter struct { ... }  // 实现 trpcskill.Repository

// internal/skill/trpc/db_repository.go — DB 适配器（TTL 缓存）
type DBRepositoryAdapter struct { ... }  // 实现 trpcskill.Repository

// internal/skill/trpc/filter.go — 白名单过滤
func NewFilteredRepository(base, allowedSlugs) trpcskill.ContextRepository
```

### 5.4 规划抽象

| 抽象 | 规划内容 | 状态 |
|------|----------|------|
| `SkillBackend` | 按 `kind`（markdown / prompt_pack / workflow / tool_backed）差异化加载与渲染 | ❌ 未实现 |
| `LoadedSkill` / `RenderedSkill` | 标准化加载结果与渲染结果，用于 Prompt 注入（方式 C） | ✅ 已实现（通过 `manifest.Parse` + `render.SkillGuidance`） |
| `SkillManifest` | 统一 frontmatter / skill.json / manifest.json 解析 | ✅ 已实现（`internal/skill/manifest/`） |

### 5.5 Manifest（逻辑模型）

与 `pkg/trpc-agent-go/tool/skilltoolset/skill` 的 frontmatter 习惯对齐，同时支持独立 `skill.json`：

| 字段 | 说明 |
|------|------|
| `name` / `slug` / `description` / `version` | 展示与版本 |
| `tags[]` | 检索与 Agent 策略 |
| `kind` | `markdown` \| `prompt_pack` \| `workflow` \| `tool_backed` |
| `entry` | 默认 `SKILL.md` |
| `requires[]` / `conflicts[]` | 依赖与冲突（见 §九） |
| `risk_level` | 策略中间件使用 |

**来源优先级**：`skill.json` / `manifest.json` → `SKILL.md` frontmatter → 导入 API 附加 metadata → 目录名推断。

---

## 六、中间件链（与 Tool 链对称）

Skill 链处理 **是否可选中、如何加载、如何裁剪、如何记录**，不做 Tool 的参数 schema 校验。

建议首批：

| 中间件 | 职责 |
|--------|------|
| Validation | manifest / entry 存在性 / slug |
| Policy | `enabled`、`status`、Agent allow/deny、tag |
| Budget | `MaxSkills`、注入 token 上限 |
| Cache | 按 `(skill_id, version)` 失效 |
| Tracing | `skills.assemble`、`skills.render` span |
| Usage | `activated` / `explicit_use` / `failed` 写入 `skill_invocation` 或事件总线 |

---

## 七、运行时 Assembler（已实现核心链路）

### 7.1 实际输入 / 输出

```go
// internal/tools/skillruntime/resolve.go
type SkillToolsetOptions struct {
    Runtime   *biz.AgentRuntimeSettings  // 含 skill_runtime_json
    UserQuery string                     // 当前用户输入
}

// ResolveSkillSlugs → []string（过滤后的 slug 列表）
```

### 7.2 实际流程（Layer A + Layer B）

1. **读库**：`ListEnabledPublishedSkillCandidates()` 获取 `enabled` + `published`（兼容 `active`）的候选 Skill。
2. **Layer A**（`applyLayerA`）：按 `SkillRuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤。
3. **Layer B**（`ResolveSkillSlugs`）：
   - **意图路由**：`skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配。
   - **标签过滤**：`filterByAllTags()` 按 `AllowedTags` + `ExtractTagHints()` 过滤。
   - **评分排序**：`scoreCandidates()` 按分类路径匹配度评分，排序后取 `MaxSkillsInToolset`。
4. **装配**：`NewAgentVisibilityFilter` + `WithSkills` / `WithSkillFilter`（trpc-agent-go 内置 Skill 工具）。

### 7.3 Skill Tool 产出

```go
// internal/skill/trpc/tools.go
func BuildSkillTools(cfg SkillToolsetConfig) []trpctool.Tool {
    // LoadTool: 加载 Skill 正文
    // RunTool: 执行 Skill 代码（需 CodeExecutor）
    // ListDocsTool: 列出 Skill 文档
    // SelectDocsTool: 选择 Skill 文档片段
}
```

### 7.4 规划扩展

| 扩展 | 说明 | 状态 |
|------|------|------|
| Prompt 注入（方式 C） | Assembler 产出 `## Available Skills` 文本块写入 system/developer message；`skilltoolset` 可选关闭 | ✅ 已实现 |
| embedding 相似度 | 候选筛选增加向量相似度匹配（当前仅关键词 + 标签） | ✅ 已实现 |
| Budget 中间件 | 注入 token 上限裁剪（当前仅 `MaxSkillsInToolset` 数量限制） | ❌ 未实现 |
| Preview API 增强 | 返回每个 Skill 的选中原因（`Reasons map[string]string`） | ✅ 已实现 |

---

## 八、持久化与 Ent / DB（现状 → 目标）

### 8.1 当前 Ent 字段摘要（与代码一致）

- **`skill`（`PlatformSkill`）**：`skill_key`、`name`、`description`、`status`、`enabled`、`config_json`、`metadata_json`（含 `tags` + `storage_dir` + `taxonomy_paths`）、软删 `deleted_at`，历史字段 `parent_id`、`level`、`agent_id`、`provider`、`model`，**已新增** `kind`（默认 `markdown`）、`risk_level`（默认 `low`）、`entry_path`（默认 `SKILL.md`）、`filesystem_missing`（默认 `false`）。
- **`skill_version`**：`skill_id`、`version`、`status`、`content_markdown`、`metadata_json`、时间戳，**已新增** `manifest_json`（默认 `{}`）、`published_at`、`validation_status`。
- **`skill_invocation`**：`skill_version`、`user_id`、`session_id`、`duration_ms`、`started_at`/`ended_at`、`preview`/`hash`、`error_code` 等；Ent 仍含 `input_json`/`output_json` 等广义字段，**已新增** `source`（默认 `runtime`，可选 `filesystem_scan` / `filesystem_watch`）、`activation_id`。

列表查询将 **`published` 与历史值 `active` 等同**（见 `skillListPredicates`），与迁移期数据共存。

### 8.2 已落地的新增字段（原 §8.2 建议演进 → 现状）

| 区域 | 原建议新增 | 当前状态 |
|------|-----------|----------|
| `skill` | `kind` | ✅ 已落地，默认 `markdown` |
| `skill` | `risk_level` | ✅ 已落地，默认 `low` |
| `skill` | `entry_path` | ✅ 已落地，默认 `SKILL.md` |
| `skill` | `filesystem_missing` | ✅ 已落地（原建议名 `runtime_status`，实际用 `filesystem_missing`） |
| `skill` | `current_version_id` | ❌ 未落地（通过 `skill_version` 查询最新版替代） |
| `skill` | `visibility` | ✅ 已落地，默认 `workspace` |
| `skill` | `default_config_json` | ✅ 已落地（Ent 字段名 `fallback_config_json`，StorageKey `default_config_json`） |
| `skill_version` | `manifest_json` | ✅ 已落地 |
| `skill_version` | `published_at` | ✅ 已落地 |
| `skill_version` | `validation_status` | ✅ 已落地 |
| `skill_version` | `file_manifest_json` | ✅ 已落地，默认 `[]` |
| `skill_invocation` | `source` | ✅ 已落地 |
| `skill_invocation` | `activation_id` | ✅ 已落地 |
| `skill_invocation` | `message_id` | ✅ 已落地 |

### 8.3 规划表（可选，重度依赖/权限开启时）

- `skill_permissions`（subject + 动作位）
- `skill_dependencies`
- `skill_conflicts`

具体 DDL 可参考下文「附录 B」中与上一版设计兼容的 SQL；落地前需与 **`接口与数据库开发规范`** 中的表前缀策略对齐（平台文档倾向 **`capability_*` 前缀**——若改名，需单独迁移任务）。

---

## 九、权限、可见性与冲突（浓缩）

### 9.1 可见性层级（目标）

`system` / `workspace` / `agent` / `private` / `public`——与 `20 skill.md` 的租户隔离、`permissions` 对象一起看；列表接口不返回无 `can_view` 的行。

### 9.2 运行时策略（已实现）

```go
// internal/biz/skill_runtime.go
type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool   // 默认 true
    IntentMaxPaths       int    // 默认 3
    MaxSkillsInToolset   int    // 默认 32，上限 256
    EmbeddingScoringEnabled bool   // 默认 false；启用 embedding 语义精排
    EmbeddingScoreWeight    float64 // 默认 0.3，范围 0~1；embedding 分权重
}
```

来源：`agent_runtime_settings.skill_runtime_json`（JSON 对象），由 `ParseSkillRuntimePolicy()` 解析。

### 9.3 依赖与冲突

- **依赖**：`required_skill`、`optional_skill`、`tool_capability`、`runtime_feature`。
- **冲突**：导入 slug 冲突、语义相似（导入流水线已实现相似度与炼化）、运行时互斥策略。
- **分级**：`info` / `warn` / `block`——与 `20 skill.md` 的 pass/warn/block 一致。

---

## 十、HTTP / Proto 面（统一清单）

| 能力 | 状态 | 备注 |
|------|------|------|
| `GET /v1/skills` | ✅ 已实现 | 分页 query：`page`/`page_size` + 搜索/标签/启用/状态筛选 |
| `GET /v1/skills/{id}` | ✅ 已实现 | 详情含 `body_markdown` |
| `POST /v1/skills` | ✅ 已实现 | 创建草稿 |
| `PATCH /v1/skills/{id}` | ✅ 已实现 | 更新草稿 |
| `POST /v1/skills/{id}/publish` | ✅ 已实现 | 发布 |
| `PATCH /v1/skills/{id}/enabled` | ✅ 已实现 | 启用/停用 |
| `POST /v1/skills/{id}/duplicate` | ✅ 已实现 | 复制为草稿 |
| `DELETE /v1/skills/{id}` | ✅ 已实现 | 软删 |
| `GET /v1/skills/{id}/files` | ✅ 已实现 | 文件列表 |
| `GET /v1/skills/{id}/file` | ✅ 已实现 | 文件内容 |
| `PUT /v1/skills/{id}/file` | ✅ 已实现 | 文件更新 |
| `POST /v1/skills/{id}/files:delete` | ✅ 已实现 | 文件删除（禁止删 SKILL.md） |
| `GET /v1/skill-runtime-preview` | ✅ 已实现 | 运行时装配预览 |
| `GET /v1/skill-runs` | ✅ 已实现 | 运行记录分页 |
| `GET /v1/skills/{id}/versions` | ✅ 已实现 | 版本历史列表（分页） |
| `POST /v1/skills/{id}/versions/{version}/rollback` | ✅ 已实现 | 版本回滚（不可变策略） |
| `POST /v1/skills/import*` | ✅ | 4 端点；multipart 由 Service 挂载 |
| `GET /v1/system-settings`、`PUT /v1/system-settings` | ✅ 已实现 | **`work_directory`**；Skill 磁盘根约定为 **`{work_directory}/skills`**（见 §2.5） |

前端类型与函数分组维持 `web/src/features/skills/api.ts` 与 proto 同源生成优先。**系统设置页**保存 `work_directory` 后，前端可在文案中提示 Skill 默认存放路径为 `{work_directory}/skills`（展示层可与后端返回的 **resolved skill root** 对齐，若后续 API 补充该只读字段）。

---

## 十一、可观测性

建议事件或 span：

- `skill.activated`、`skill.used`、`skill.failed`
- Span：`skills.assemble`、`skills.registry.search`、`skills.backend.load`、`skills.backend.render`、`skills.usage.record`

属性：`skill.id`、`skill.slug`、`skill.version`、`agent.id`、`session.id`、`activation.source`、`token.cost`。

磁盘同步（§2.6）建议补充：`skill.fs.scan`、`skill.fs.synced`、`skill.fs.error`（日志或指标），便于确认「文件夹新增已入库」。

## 十二、演进路线（2026-06-06 现状对齐）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P0** | `storage.ResolveRootWithPlatform()` 接通 `SystemSetting.work_directory`，约定 `{work_directory}/skills`；ZIP/文件 API/导入与工作目录一致 | ✅ 已完成 |
| **P1** | 补齐 proto：`CreateSkill`、`UpdateSkill`、`PublishSkill`、`GetSkill`、`DeleteSkillFile`、`PreviewSkillRuntime`；biz/data 贯通 | ✅ 已完成 |
| **P2** | Layer A/B + `buildSkillDeps` + turn query 注入 | ✅ 已完成 |
| **P2′** | Skill 根目录 `fsnotify` 监听 + debounce + eventBus + `filesystem_missing` 标记 | ✅ 已完成 |
| **P2.5** | 磁盘同步产品化：proto 字段、health API、目录 slug 约束、Monitor 通知、Skill 页 Banner | ✅ 已完成 |
| **P3** | 版本历史/回滚；RBAC（`requireAdminAccess` + `applySkillPermission`）；Ent 字段补齐（`visibility`/`default_config_json`/`file_manifest_json`/`message_id`） | ✅ 已完成 |
| **P4** | Prompt 注入（方式 C）；embedding 语义精排；Preview 选中原因；manifest/render 包 | ✅ 已完成 |
| **P4+** | Budget 中间件；Skill 依赖/冲突表；自动负熵报告；`SkillBackend` 多 kind 差异化；Context 目录迁移 | ❌ 待实现 |
| **P5** | 统一去重引擎（`SkillSimilarityEngine` 4 维 + 可选 Embedding）；三阶段合并（`SkillMergeUsecase` 内容融合→Gate→事务应用）；统一进化编排（`SkillEvolutionOrchestrator` + `EvolutionTrigger` 策略模式，DB UNIQUE 兜底 TOCTOU）；ScoreSkill 四维权重修复（Token/Feedback 条件启用 + 启发式 Feedback）；`EvolutionCoordinator` 标记 deprecated | ✅ 已完成 |

---

## 十三′、Skill 召回管线：分类树 · 多维标签 · 索引收窄 · 语义精排（规划）

本节描述端到端心智模型；**层 A（Agent 策略）与层 B（意图→候选收窄）已在运行时落地**，小范围语义精排仍为后续能力。

### 13′.1 详细分类树（示例）

意图可挂载到叶子路径（存储侧推荐写入 Skill `metadata_json.taxonomy_paths` 数组）：

- **数据获取与集成**
  - **内部数据源**
    - **文件系统读取（读取表格）** → 示例 Skill：`excel_reader`（读 xlsx）
- **分析与推理**
  - **自然语言理解（情感分析）** → 示例：`sentiment_analysis`
- **交互与执行**
  - **消息发送（发邮件）** → 示例：`email_sender`

### 13′.2 多维标签

标签推荐落在 **`SkillTag.name`**（与检索惯例一致），使用形如 `dim:value` 的扁平 token，例如：

- `file_type:xlsx`
- `domain:sales`

运行时：**同类约束为合取（AND）**——候选 Skill 必须同时具备本轮所需的每一个标签 token（Agent 策略中的 `allowed_tags` 与用户话术中带出的 hint 合并后再过滤）。

### 13′.3 索引收窄 → 标签过滤 →（规划中）语义精排

**.walkthrough（与用户示例对齐）**

1. **意图分类**：从用户话术中命中多条意图路径（实现上先做关键词/规则路由；语义意图模型可后续替换）。
2. **候选召回**：在多条路径下做 **OR** 并集（示意：约 23 个 Skill）。
3. **标签过滤**：例如要求 `file_type:xlsx` 且 `domain:sales`，将候选 **AND** 缩至更小集合（示意：7 个）。
4. **语义排序（待实现）**：对余下的少量 Skill 做 embedding / rerank，选出最优组合，例如：
   - `excel_reader`（读 xlsx）
   - `sentiment_analysis`（情感分析）
   - `email_sender`（发送邮件）
5. **执行规划（Planner / LLM，与本节数据结构独立）** 示意：
   - Skill 1: `excel_reader(file_path=?, sheet="客户反馈")`
   - Skill 2: `sentiment_analysis(text_col="反馈内容")`
   - Skill 3: `email_sender(to=当前用户邮箱, body=负面反馈汇总)`

### 13′.4 已实现：层 A — Agent `skill_runtime_json`

持久化字段：**`agent_runtime_settings.skill_runtime_json`**（JSON 对象）。

| 字段 | 含义 |
|------|------|
| `allowed_slugs` | 非空时仅保留列出的 slug（skill_key），与其它过滤 **相交** |
| `denied_slugs` | 优先剔除 |
| `allowed_tags` | 与话术抽取的标签 hint **合并** 后，对 Skill 标签做 **合取** 过滤 |
| `intent_routing_enabled` | 默认 `true`；显式 `false` 时跳过层 B 路径收窄 |
| `intent_max_paths` | 意图路径上限，默认 `3` |
| `max_skills_in_toolset` | 最终挂载到 trpc-agent-go Skill 工具链的 Skill 数量上限，默认 `32` |

### 13′.5 已实现：层 B — 意图路径关键词路由

- 内置若干 **taxonomy leaf** 与 **召回关键词**（`internal/tools/skillrouter`）。
- `DetectIntentPaths(user_query)` → 多条叶子路径；候选 Skill 只要在 **taxonomy_paths** 或 **slug/name/description** 上与任一命中路径相关，则进入并集。
- `ExtractTagHints(user_query)`：解析 `file_type:`、`domain:` 及常见 `xlsx` 暗示。

### 13′.6 Skill `metadata_json` 约定

```json
{
  "taxonomy_paths": [
    "数据获取与集成/内部数据源/文件系统读取（读取表格）"
  ],
  "tags": [{ "name": "file_type:xlsx", "source": "user" }]
}
```

### 13′.7 代码锚点

- `internal/tools/skillruntime`：`SkillToolsetOptions`、`ResolveSkillSlugs`、`NewAgentVisibilityFilter`
- `internal/tools/skillrouter`：`DetectIntentPaths`、`ExtractTagHints`、`TaxonomyLeaves`
- `internal/biz/skill_runtime.go`：`SkillRuntimePolicy`、`SkillRuntimeCandidate`、`ParseSkillRuntimePolicy`
- `internal/data/skill.go`：`ListEnabledPublishedSkillCandidates`
- `internal/agent/trpc_build.go`：`buildSkillDeps`

---

## 十三、测试关注点

- manifest 解析与校验（SKILL.md / skill.json）
- Assembler：policy / budget / 裁剪顺序
- 导入：结构校验、名称冲突、相似度、炼化、apply 幂等
- **`work_directory` 拼接 Skill 根**：变更设置前后路径解析、空值回落、env 覆盖优先
- **磁盘扫描 / watch**：新增目录入库、重复 slug 幂等、文件变更检测、删除目录与 DB 状态一致
- 数据：`published` vs `active` 兼容查询
- 前端：`features/skills` 与分页契约

---

## 附录 A · 与代码模块映射（替换旧「§十八」）

```text
api/kratos/skill/v1/skill.proto          → HTTP 契约（20 RPC）
internal/service/skill.go                → 适配层
internal/service/skill_import.go         → 导入 biz 桥接
internal/service/skill_import_http.go    → multipart POST /v1/skills/import
internal/biz/skill/skill.go              → 用例与 SkillReader/SkillWriter/Repo 端口
internal/biz/skill/skill_import.go       → 导入 DTO
internal/biz/skill/skill_runtime.go      → 运行时策略
internal/biz/skill_similarity.go         → 统一相似度引擎（4 维 + 可选 Embedding）
internal/biz/skill_merge.go              → 三阶段合并 Usecase
internal/biz/skill_merge_ai_fuser.go     → 基于规则的内容融合器
internal/biz/skill_evolution_unified.go  → 统一进化编排器 + UnifiedEvolutionSuggestion + UnifiedEvolutionReader/Writer 接口
internal/biz/skill_evolution_triggers.go → EvolutionTrigger 策略（Pattern/Health/AgentConfig）+ SkillScorer 窄接口
internal/biz/skill_intelligence.go       → ScoreSkill 四维权重（含 Token/Feedback 条件启用）；SetCoordinator sync.Once 保护
internal/biz/skill_evolution.go          → SkillEvolutionUsecase；SetCoordinator sync.Once 保护；SuggestionFromProposal 使用 EvoSuggestionCreateSkill
internal/biz/skill_dedup.go              → SkillDedupUsecase（DetectDuplicateGroups 带 10min TTL 缓存 + InvalidateDedupCache）；MergeSkills Deprecated
internal/biz/evolution_coordinator.go    → [deprecated] 旧进化协调器，委托 orchestrator
internal/data/skill.go                   → Ent 仓储与聚合
internal/data/skill_merge.go             → 合并 Data 层（事务内 4 步操作）
internal/data/skill_dedup.go             → 去重 Data 层（含 SkillSimilarityEngine 集成）
internal/data/skill_intelligence.go      → 健康指标聚合（含 AvgTokenUsage/FeedbackScore）
internal/data/unified_evolution.go       → 统一进化 Data 层（raw SQL + 读写分离，实现 UnifiedEvolutionReader + Writer）
internal/skill/importer/*                → ZIP 导入领域实现
internal/skill/watch/*                   → 磁盘监听与幂等 upsert
internal/skill/manifest/*                → frontmatter 解析与校验
internal/skill/render/*                  → prompt 渲染与截断
internal/tools/skillruntime/*            → ResolveSkillSlugs + AgentVisibilityFilter
internal/tools/skillrouter/*             → 意图路径与标签 hint（层 B）
internal/agent/trpc_build.go             → buildSkillDeps
internal/agent/skill_guidance_inject.go  → Prompt 注入方式 C
internal/skill/trpc/*                    → trpc-agent-go Repository 桥接
api/kratos/system_setting/v1/system_setting.proto → work_directory
```

---

## 附录 B · 规划表 DDL（参考，落地前须评审前缀与迁移）

与前一版设计兼容；**若启用 capability_ 前缀，仅追加映射层或迁移脚本**。

```sql
-- skill 本体增量列示例（名称仅供评审）
ALTER TABLE skill ADD COLUMN kind TEXT NOT NULL DEFAULT 'markdown';
ALTER TABLE skill ADD COLUMN risk_level TEXT NOT NULL DEFAULT 'low';
ALTER TABLE skill ADD COLUMN entry_path TEXT NOT NULL DEFAULT 'SKILL.md';
ALTER TABLE skill ADD COLUMN runtime_status TEXT NOT NULL DEFAULT 'catalog_only';
ALTER TABLE skill ADD COLUMN current_version_id TEXT NOT NULL DEFAULT '';
ALTER TABLE skill ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace';

-- skill_version 增量列示例
ALTER TABLE skill_version ADD COLUMN manifest_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE skill_version ADD COLUMN file_manifest_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE skill_version ADD COLUMN published_at TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_version ADD COLUMN validation_status TEXT NOT NULL DEFAULT '';

-- skill_invocation 增量列示例
ALTER TABLE skill_invocation ADD COLUMN activation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_invocation ADD COLUMN source TEXT NOT NULL DEFAULT 'runtime';
ALTER TABLE skill_invocation ADD COLUMN message_id TEXT NOT NULL DEFAULT '';

-- agent_runtime_settings：Skill 运行时策略 JSON（见正文「十三′」）
ALTER TABLE agent_runtime_settings ADD COLUMN skill_runtime_json TEXT NOT NULL DEFAULT '{}';
```

关系表 `skill_permissions` / `skill_dependencies` / `skill_conflicts` 及索引设计与前一版一致时可原文沿用；本文不再重复占篇幅，需要时从 Git 历史恢复全文 DDL。

---

## 附录 C · 落地原则（保留）

1. Skill 是知识/流程/提示包，Tool 是可执行函数；协作靠 Runtime Prompt + usage 关联。
2. **不得破坏**已安装 Skill 的存储目录与版本引用路径。
3. 运行时激活 **可解释**：每个 Skill 有选中原因与 token 估计。
4. Prompt 渲染 **可截断、可测试**，禁止无界注入大文件。
5. 副作用默认只走 Tool 或显式审批路径。
6. **Skill 物理根与系统设置工作目录一致可追溯**：解析规则唯一；变更工作目录须有迁移或再索引策略。
7. **磁盘增量同步须幂等、可观测**，且不得跨越 Skill 根路径访问文件系统。

---

*文档版本：4.5 — P5 对齐代码现状；统一去重引擎/三阶段合并/统一进化编排/ScoreSkill 四维权重修复；并发安全修复（RegisterTrigger RWMutex / SetCoordinator sync.Once / DedupCache TTL）；ExpirePending 实现；接口拆分（Reader/Writer）；SkillScorer 窄接口；PatternTrigger 多返回值；前端统一进化 API 错误区分与分页修复（2026-06-11）。*
