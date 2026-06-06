# 行业模板库（Industry Template Library）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Aranea-Agents 平台构建 Industry → Department → Position → Agent(variant) 四层分类体系，并交付软件开发行业作为首个完整行业模板包。

**Architecture:** 新增 3 张 Ent 表（industries / departments / positions）+ agents 表扩展 3 字段（position_key / agent_variant / variant_description）。Biz 层新增 IndustryUsecase + Repo 接口，Data 层实现 Ent CRUD，Service 层暴露 HTTP API，前端新增行业市场页。首个行业包 `softwaredev` 以 `internal/scenario/softwaredev/` 场景安装包形式交付。

**Tech Stack:** Go / Ent ORM / SQLite / Kratos v2 (HTTP+gRPC) / Proto3 / Vue 3 + Quasar + Pinia + TypeScript

**Design Spec:** `docs/scenarios/industry-template-library.design.md`

---

## File Structure

### New Files (Backend)

```
internal/data/ent/schema/industry.go           — Ent Schema: industries 表
internal/data/ent/schema/department.go         — Ent Schema: departments 表
internal/data/ent/schema/position.go           — Ent Schema: positions 表
docs/sql/02_industry.sql                       — DDL: 3 张新表 + agents 表扩展
internal/biz/industry_types.go                 — Biz 类型定义
internal/biz/industry_usecase.go               — Biz Usecase
internal/biz/department_types.go               — Biz 类型定义
internal/biz/department_usecase.go             — Biz Usecase
internal/biz/position_types.go                 — Biz 类型定义
internal/biz/position_usecase.go               — Biz Usecase
internal/data/industry_repo.go                 — Data Repo 实现
internal/data/department_repo.go               — Data Repo 实现
internal/data/position_repo.go                 — Data Repo 实现
api/kratos/industry/v1/industry.proto          — Proto 定义
internal/service/industry.go                   — Service 层
internal/server/register_industry.go           — Server 路由注册
cmd/seed-industries/main.go                    — Seed CLI
```

### New Files (Softwaredev Scenario Package)

```
internal/scenario/softwaredev/install.go
internal/scenario/softwaredev/industry.yaml
internal/scenario/softwaredev/agents.yaml
internal/scenario/softwaredev/teams.yaml
internal/scenario/softwaredev/prompts/positions/go_senior_engineer/general.md
internal/scenario/softwaredev/prompts/positions/go_senior_engineer/code_review.md
internal/scenario/softwaredev/prompts/positions/go_senior_engineer/architect.md
internal/scenario/softwaredev/prompts/positions/vue3_senior_engineer/general.md
internal/scenario/softwaredev/prompts/positions/vue3_senior_engineer/code_review.md
internal/scenario/softwaredev/prompts/positions/vue3_senior_engineer/ux_auditor.md
internal/scenario/softwaredev/prompts/positions/ue_client_programmer/general.md
internal/scenario/softwaredev/prompts/positions/ue_client_programmer/gameplay.md
internal/scenario/softwaredev/prompts/positions/ue_client_programmer/performance.md
internal/scenario/softwaredev/prompts/positions/ue_client_programmer/network.md
internal/scenario/softwaredev/schemas/go_dev_output.schema.json
internal/scenario/softwaredev/schemas/go_review_output.schema.json
internal/scenario/softwaredev/schemas/go_arch_output.schema.json
internal/scenario/softwaredev/skills/softwaredev-go-best-practices/SKILL.md
internal/scenario/softwaredev/skills/softwaredev-clean-arch/SKILL.md
internal/scenario/softwaredev/skills/softwaredev-code-review-checklist/SKILL.md
internal/scenario/softwaredev/skills/softwaredev-ddd-tactical/SKILL.md
internal/scenario/softwaredev/skills/softwaredev-ue5-gas/SKILL.md
```

### New Files (Frontend)

```
web/src/pages/industries/IndustryMarketPage.vue
web/src/pages/industries/IndustryDetailPage.vue
web/src/features/industries/api.ts
web/src/features/industries/types.ts
web/src/features/industries/useIndustryMarket.ts
web/src/features/industries/useIndustryDetail.ts
web/src/components/industries/IndustryCard.vue
web/src/components/industries/DepartmentTree.vue
web/src/components/industries/PositionCard.vue
web/src/components/industries/AgentVariantBadge.vue
```

### Modified Files

```
internal/data/ent/schema/agent.go              — 新增 3 字段
internal/data/data.go                          — 新增 Repo 构造函数
internal/biz/biz.go                            — 新增 ProviderSet
internal/data/data.go                          — 新增 ProviderSet
internal/service/service.go                    — 新增 ProviderSet
internal/server/server.go                      — 注册 industry 路由
cmd/admin/wire.go                              — Wire 注入
web/src/router/index.ts                        — 新增行业路由
web/src/layouts/MainLayout.vue                 — 侧栏新增行业入口
```

---

## Task 1: Ent Schema — industries / departments / positions 三张新表

**Files:**
- Create: `internal/data/ent/schema/industry.go`
- Create: `internal/data/ent/schema/department.go`
- Create: `internal/data/ent/schema/position.go`
- Create: `docs/sql/02_industry.sql`
- Test: `internal/data/industry_repo_test.go`（Task 3 中编写）

- [ ] **Step 1: 创建 industry.go Ent Schema**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Industry struct {
	ent.Schema
}

func (Industry) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "industries"},
	}
}

func (Industry) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scenario_key"),
		index.Fields("enabled"),
	}
}

func (Industry) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("key").Unique().MaxLen(128),
		field.String("name").MaxLen(256),
		field.String("icon").Default(""),
		field.Text("description").Default(""),
		field.String("scenario_key").Default(""),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
```

- [ ] **Step 2: 创建 department.go Ent Schema**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Department struct {
	ent.Schema
}

func (Department) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "departments"},
	}
}

func (Department) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("industry_key"),
		index.Fields("key", "industry_key").Unique(),
	}
}

func (Department) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("key").MaxLen(128),
		field.String("name").MaxLen(256),
		field.String("industry_key").MaxLen(128),
		field.Text("description").Default(""),
		field.Text("responsibilities_json").Default("{}"),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
```

- [ ] **Step 3: 创建 position.go Ent Schema**

```go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Position struct {
	ent.Schema
}

func (Position) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "positions"},
	}
}

func (Position) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("department_key"),
		index.Fields("key", "department_key").Unique(),
	}
}

func (Position) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").Immutable().Unique().MaxLen(256),
		field.String("key").MaxLen(128),
		field.String("name").MaxLen(256),
		field.String("department_key").MaxLen(128),
		field.Text("description").Default(""),
		field.Text("responsibilities_json").Default("{}"),
		field.Strings("skills_required").Optional(),
		field.String("seniority_level").Default(""),
		field.Int("sort_order").Default(0),
		field.String("created_at").Default(""),
		field.String("updated_at").Default(""),
		field.String("deleted_at").Default(""),
	}
}
```

- [ ] **Step 4: 创建 SQL DDL 文件 `docs/sql/02_industry.sql`**

```sql
-- ============================================================
-- Industry Template Library: industries, departments, positions
-- + agents table extension (position_key, agent_variant, variant_description)
-- ============================================================

CREATE TABLE IF NOT EXISTS industries (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  icon TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  scenario_key TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS departments (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  industry_key TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  responsibilities_json TEXT NOT NULL DEFAULT '{}',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(key, industry_key)
);

CREATE INDEX idx_departments_industry_key ON departments(industry_key);

CREATE TABLE IF NOT EXISTS positions (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  department_key TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  responsibilities_json TEXT NOT NULL DEFAULT '{}',
  skills_required_json TEXT NOT NULL DEFAULT '[]',
  seniority_level TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(key, department_key)
);

CREATE INDEX idx_positions_department_key ON positions(department_key);

-- agents table extension
ALTER TABLE agents ADD COLUMN position_key TEXT NOT NULL DEFAULT '';
ALTER TABLE agents ADD COLUMN agent_variant TEXT NOT NULL DEFAULT 'general';
ALTER TABLE agents ADD COLUMN variant_description TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_agents_position_variant ON agents(position_key, agent_variant);
```

- [ ] **Step 5: 扩展 agents Ent Schema — 新增 3 字段**

在 `internal/data/ent/schema/agent.go` 的 `Fields()` 末尾追加：

```go
field.String("position_key").Default("").Comment("FK to positions.key"),
field.String("agent_variant").Default("general").Comment("variant within position: general/code_review/architect/..."),
field.Text("variant_description").Default("").Comment("human-readable description of this variant"),
```

- [ ] **Step 6: 运行 `go generate` 生成 Ent 代码**

Run: `go generate ./internal/data/ent/...`
Expected: 生成成功，无错误

- [ ] **Step 7: 验证编译**

Run: `go build ./...`
Expected: 编译通过

- [ ] **Step 8: Commit**

```bash
git add internal/data/ent/schema/industry.go internal/data/ent/schema/department.go internal/data/ent/schema/position.go internal/data/ent/schema/agent.go docs/sql/02_industry.sql internal/data/ent/
git commit -m "feat(industry): add industries/departments/positions Ent schemas + agents table extension"
```

---

## Task 2: Biz 层 — 类型定义 + Usecase + Repo 接口

**Files:**
- Create: `internal/biz/industry_types.go`
- Create: `internal/biz/industry_usecase.go`
- Create: `internal/biz/department_types.go`
- Create: `internal/biz/department_usecase.go`
- Create: `internal/biz/position_types.go`
- Create: `internal/biz/position_usecase.go`
- Modify: `internal/biz/biz.go`

- [ ] **Step 1: 创建 `internal/biz/industry_types.go`**

```go
package biz

type Industry struct {
	ID          string
	Key         string
	Name        string
	Icon        string
	Description string
	ScenarioKey string
	Enabled     bool
	SortOrder   int
	CreatedAt   string
	UpdatedAt   string
	DeletedAt   string
}

type IndustryListQuery struct {
	Enabled *bool
}

type IndustryListResult struct {
	Items []Industry
	Total int
}
```

- [ ] **Step 2: 创建 `internal/biz/industry_usecase.go`**

```go
package biz

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type IndustryReader interface {
	ListIndustries(ctx context.Context, q IndustryListQuery) (IndustryListResult, error)
	GetIndustryByKey(ctx context.Context, key string) (Industry, error)
}

type IndustryWriter interface {
	CreateIndustry(ctx context.Context, ind Industry) (Industry, error)
	UpdateIndustry(ctx context.Context, ind Industry) (Industry, error)
	DeleteIndustry(ctx context.Context, id string) error
	UpsertIndustryByKey(ctx context.Context, ind Industry) (Industry, error)
}

type IndustryRepository interface {
	IndustryReader
	IndustryWriter
}

type IndustryUsecase struct {
	repo IndustryRepository
}

func NewIndustryUsecase(repo IndustryRepository) *IndustryUsecase {
	return &IndustryUsecase{repo: repo}
}

func (u *IndustryUsecase) List(ctx context.Context, q IndustryListQuery) (IndustryListResult, error) {
	return u.repo.ListIndustries(ctx, q)
}

func (u *IndustryUsecase) GetByKey(ctx context.Context, key string) (Industry, error) {
	if key == "" {
		return Industry{}, kerrors.BadRequest("INDUSTRY", "key is required")
	}
	return u.repo.GetIndustryByKey(ctx, key)
}

func (u *IndustryUsecase) UpsertByKey(ctx context.Context, ind Industry) (Industry, error) {
	if ind.Key == "" {
		return Industry{}, kerrors.BadRequest("INDUSTRY", "key is required")
	}
	return u.repo.UpsertIndustryByKey(ctx, ind)
}
```

- [ ] **Step 3: 创建 `internal/biz/department_types.go`**

```go
package biz

type Department struct {
	ID                  string
	Key                 string
	Name                string
	IndustryKey         string
	Description         string
	ResponsibilitiesJSON string
	SortOrder           int
	CreatedAt           string
	UpdatedAt           string
	DeletedAt           string
}

type DepartmentListQuery struct {
	IndustryKey string
}

type DepartmentListResult struct {
	Items []Department
	Total int
}
```

- [ ] **Step 4: 创建 `internal/biz/department_usecase.go`**

```go
package biz

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type DepartmentReader interface {
	ListDepartments(ctx context.Context, q DepartmentListQuery) (DepartmentListResult, error)
	GetDepartmentByKey(ctx context.Context, key, industryKey string) (Department, error)
}

type DepartmentWriter interface {
	CreateDepartment(ctx context.Context, d Department) (Department, error)
	UpsertDepartmentByKey(ctx context.Context, d Department) (Department, error)
	DeleteDepartment(ctx context.Context, id string) error
}

type DepartmentRepository interface {
	DepartmentReader
	DepartmentWriter
}

type DepartmentUsecase struct {
	repo DepartmentRepository
}

func NewDepartmentUsecase(repo DepartmentRepository) *DepartmentUsecase {
	return &DepartmentUsecase{repo: repo}
}

func (u *DepartmentUsecase) ListByIndustry(ctx context.Context, industryKey string) (DepartmentListResult, error) {
	if industryKey == "" {
		return DepartmentListResult{}, kerrors.BadRequest("DEPARTMENT", "industry_key is required")
	}
	return u.repo.ListDepartments(ctx, DepartmentListQuery{IndustryKey: industryKey})
}

func (u *DepartmentUsecase) UpsertByKey(ctx context.Context, d Department) (Department, error) {
	if d.Key == "" || d.IndustryKey == "" {
		return Department{}, kerrors.BadRequest("DEPARTMENT", "key and industry_key are required")
	}
	return u.repo.UpsertDepartmentByKey(ctx, d)
}
```

- [ ] **Step 5: 创建 `internal/biz/position_types.go`**

```go
package biz

type Position struct {
	ID                  string
	Key                 string
	Name                string
	DepartmentKey       string
	Description         string
	ResponsibilitiesJSON string
	SkillsRequired      []string
	SeniorityLevel      string
	SortOrder           int
	CreatedAt           string
	UpdatedAt           string
	DeletedAt           string
}

type PositionListQuery struct {
	DepartmentKey string
	IndustryKey   string
}

type PositionListResult struct {
	Items []Position
	Total int
}
```

- [ ] **Step 6: 创建 `internal/biz/position_usecase.go`**

```go
package biz

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type PositionReader interface {
	ListPositions(ctx context.Context, q PositionListQuery) (PositionListResult, error)
	GetPositionByKey(ctx context.Context, key, departmentKey string) (Position, error)
}

type PositionWriter interface {
	CreatePosition(ctx context.Context, p Position) (Position, error)
	UpsertPositionByKey(ctx context.Context, p Position) (Position, error)
	DeletePosition(ctx context.Context, id string) error
}

type PositionRepository interface {
	PositionReader
	PositionWriter
}

type PositionUsecase struct {
	repo PositionRepository
}

func NewPositionUsecase(repo PositionRepository) *PositionUsecase {
	return &PositionUsecase{repo: repo}
}

func (u *PositionUsecase) ListByDepartment(ctx context.Context, departmentKey string) (PositionListResult, error) {
	if departmentKey == "" {
		return PositionListResult{}, kerrors.BadRequest("POSITION", "department_key is required")
	}
	return u.repo.ListPositions(ctx, PositionListQuery{DepartmentKey: departmentKey})
}

func (u *PositionUsecase) UpsertByKey(ctx context.Context, p Position) (Position, error) {
	if p.Key == "" || p.DepartmentKey == "" {
		return Position{}, kerrors.BadRequest("POSITION", "key and department_key are required")
	}
	return u.repo.UpsertPositionByKey(ctx, p)
}
```

- [ ] **Step 7: 更新 `internal/biz/biz.go` — 新增 ProviderSet**

在 `biz.go` 的 `ProviderSet` 中追加：

```go
wire.NewSet(
	// ... 已有的 providers ...
	NewIndustryUsecase,
	NewDepartmentUsecase,
	NewPositionUsecase,
)
```

- [ ] **Step 8: 验证编译**

Run: `go build ./internal/biz/...`
Expected: 编译通过（Repo 实现尚未创建，但接口定义完整）

- [ ] **Step 9: Commit**

```bash
git add internal/biz/industry_types.go internal/biz/industry_usecase.go internal/biz/department_types.go internal/biz/department_usecase.go internal/biz/position_types.go internal/biz/position_usecase.go internal/biz/biz.go
git commit -m "feat(industry): add biz types, usecases and repo interfaces for industry/department/position"
```

---

## Task 3: Data 层 — Repo 实现

**Files:**
- Create: `internal/data/industry_repo.go`
- Create: `internal/data/department_repo.go`
- Create: `internal/data/position_repo.go`
- Modify: `internal/data/data.go`
- Test: `internal/data/industry_repo_test.go`

- [ ] **Step 1: 创建 `internal/data/industry_repo.go`**

```go
package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/industry"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type industryRepo struct {
	data *Data
}

func NewIndustryRepo(data *Data) biz.IndustryRepository {
	return &industryRepo{data: data}
}

func newIndustryID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))
	}
	return hex.EncodeToString(buf)
}

func entIndustryToBiz(e *ent.Industry) biz.Industry {
	return biz.Industry{
		ID:          e.ID,
		Key:         e.Key,
		Name:        e.Name,
		Icon:        e.Icon,
		Description: e.Description,
		ScenarioKey: e.ScenarioKey,
		Enabled:     e.Enabled,
		SortOrder:   e.SortOrder,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
		DeletedAt:   e.DeletedAt,
	}
}

func (r *industryRepo) ListIndustries(ctx context.Context, q biz.IndustryListQuery) (biz.IndustryListResult, error) {
	query := r.data.entClient.Industry.Query().Where(industry.DeletedAtEQ(""))
	if q.Enabled != nil {
		query = query.Where(industry.EnabledEQ(*q.Enabled))
	}
	items, err := query.Order(ent.Asc(industry.FieldSortOrder)).All(ctx)
	if err != nil {
		return biz.IndustryListResult{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	result := make([]biz.Industry, 0, len(items))
	for _, e := range items {
		result = append(result, entIndustryToBiz(e))
	}
	return biz.IndustryListResult{Items: result, Total: len(result)}, nil
}

func (r *industryRepo) GetIndustryByKey(ctx context.Context, key string) (biz.Industry, error) {
	e, err := r.data.entClient.Industry.Query().Where(industry.KeyEQ(key), industry.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		return biz.Industry{}, kerrors.NotFound("INDUSTRY", "industry not found")
	}
	return entIndustryToBiz(e), nil
}

func (r *industryRepo) CreateIndustry(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	e, err := r.data.entClient.Industry.Create().
		SetID(newIndustryID()).
		SetKey(ind.Key).
		SetName(ind.Name).
		SetIcon(ind.Icon).
		SetDescription(ind.Description).
		SetScenarioKey(ind.ScenarioKey).
		SetEnabled(ind.Enabled).
		SetSortOrder(ind.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Industry{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	return entIndustryToBiz(e), nil
}

func (r *industryRepo) UpdateIndustry(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	update := r.data.entClient.Industry.UpdateOneID(ind.ID).
		SetName(ind.Name).
		SetIcon(ind.Icon).
		SetDescription(ind.Description).
		SetEnabled(ind.Enabled).
		SetSortOrder(ind.SortOrder).
		SetUpdatedAt(now)
	e, err := update.Save(ctx)
	if err != nil {
		return biz.Industry{}, kerrors.InternalServer("INDUSTRY", err.Error())
	}
	return entIndustryToBiz(e), nil
}

func (r *industryRepo) DeleteIndustry(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.entClient.Industry.UpdateOneID(id).SetDeletedAt(now).Save(ctx)
	if err != nil {
		return kerrors.InternalServer("INDUSTRY", err.Error())
	}
	return nil
}

func (r *industryRepo) UpsertIndustryByKey(ctx context.Context, ind biz.Industry) (biz.Industry, error) {
	existing, err := r.data.entClient.Industry.Query().Where(industry.KeyEQ(ind.Key), industry.DeletedAtEQ("")).Only(ctx)
	if err == nil && existing != nil {
		ind.ID = existing.ID
		return r.UpdateIndustry(ctx, ind)
	}
	return r.CreateIndustry(ctx, ind)
}
```

- [ ] **Step 2: 创建 `internal/data/department_repo.go`**

```go
package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/department"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type departmentRepo struct {
	data *Data
}

func NewDepartmentRepo(data *Data) biz.DepartmentRepository {
	return &departmentRepo{data: data}
}

func newDepartmentID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))
	}
	return hex.EncodeToString(buf)
}

func entDepartmentToBiz(e *ent.Department) biz.Department {
	return biz.Department{
		ID:                  e.ID,
		Key:                 e.Key,
		Name:                e.Name,
		IndustryKey:         e.IndustryKey,
		Description:         e.Description,
		ResponsibilitiesJSON: e.ResponsibilitiesJSON,
		SortOrder:           e.SortOrder,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
		DeletedAt:           e.DeletedAt,
	}
}

func (r *departmentRepo) ListDepartments(ctx context.Context, q biz.DepartmentListQuery) (biz.DepartmentListResult, error) {
	query := r.data.entClient.Department.Query().Where(department.DeletedAtEQ(""))
	if q.IndustryKey != "" {
		query = query.Where(department.IndustryKeyEQ(q.IndustryKey))
	}
	items, err := query.Order(ent.Asc(department.FieldSortOrder)).All(ctx)
	if err != nil {
		return biz.DepartmentListResult{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	result := make([]biz.Department, 0, len(items))
	for _, e := range items {
		result = append(result, entDepartmentToBiz(e))
	}
	return biz.DepartmentListResult{Items: result, Total: len(result)}, nil
}

func (r *departmentRepo) GetDepartmentByKey(ctx context.Context, key, industryKey string) (biz.Department, error) {
	e, err := r.data.entClient.Department.Query().
		Where(department.KeyEQ(key), department.IndustryKeyEQ(industryKey), department.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Department{}, kerrors.NotFound("DEPARTMENT", "department not found")
	}
	return entDepartmentToBiz(e), nil
}

func (r *departmentRepo) CreateDepartment(ctx context.Context, d biz.Department) (biz.Department, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	e, err := r.data.entClient.Department.Create().
		SetID(newDepartmentID()).
		SetKey(d.Key).
		SetName(d.Name).
		SetIndustryKey(d.IndustryKey).
		SetDescription(d.Description).
		SetResponsibilitiesJSON(d.ResponsibilitiesJSON).
		SetSortOrder(d.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.Department{}, kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	return entDepartmentToBiz(e), nil
}

func (r *departmentRepo) UpsertDepartmentByKey(ctx context.Context, d biz.Department) (biz.Department, error) {
	existing, err := r.data.entClient.Department.Query().
		Where(department.KeyEQ(d.Key), department.IndustryKeyEQ(d.IndustryKey), department.DeletedAtEQ("")).
		Only(ctx)
	if err == nil && existing != nil {
		now := time.Now().UTC().Format(time.RFC3339)
		e, updateErr := r.data.entClient.Department.UpdateOneID(existing.ID).
			SetName(d.Name).
			SetDescription(d.Description).
			SetResponsibilitiesJSON(d.ResponsibilitiesJSON).
			SetSortOrder(d.SortOrder).
			SetUpdatedAt(now).
			Save(ctx)
		if updateErr != nil {
			return biz.Department{}, kerrors.InternalServer("DEPARTMENT", updateErr.Error())
		}
		return entDepartmentToBiz(e), nil
	}
	return r.CreateDepartment(ctx, d)
}

func (r *departmentRepo) DeleteDepartment(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.entClient.Department.UpdateOneID(id).SetDeletedAt(now).Save(ctx)
	if err != nil {
		return kerrors.InternalServer("DEPARTMENT", err.Error())
	}
	return nil
}
```

- [ ] **Step 3: 创建 `internal/data/position_repo.go`**

```go
package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/position"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type positionRepo struct {
	data *Data
}

func NewPositionRepo(data *Data) biz.PositionRepository {
	return &positionRepo{data: data}
}

func newPositionID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405")))
	}
	return hex.EncodeToString(buf)
}

func entPositionToBiz(e *ent.Position) biz.Position {
	return biz.Position{
		ID:                  e.ID,
		Key:                 e.Key,
		Name:                e.Name,
		DepartmentKey:       e.DepartmentKey,
		Description:         e.Description,
		ResponsibilitiesJSON: e.ResponsibilitiesJSON,
		SkillsRequired:      e.SkillsRequired,
		SeniorityLevel:      e.SeniorityLevel,
		SortOrder:           e.SortOrder,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
		DeletedAt:           e.DeletedAt,
	}
}

func (r *positionRepo) ListPositions(ctx context.Context, q biz.PositionListQuery) (biz.PositionListResult, error) {
	query := r.data.entClient.Position.Query().Where(position.DeletedAtEQ(""))
	if q.DepartmentKey != "" {
		query = query.Where(position.DepartmentKeyEQ(q.DepartmentKey))
	}
	items, err := query.Order(ent.Asc(position.FieldSortOrder)).All(ctx)
	if err != nil {
		return biz.PositionListResult{}, kerrors.InternalServer("POSITION", err.Error())
	}
	result := make([]biz.Position, 0, len(items))
	for _, e := range items {
		result = append(result, entPositionToBiz(e))
	}
	return biz.PositionListResult{Items: result, Total: len(result)}, nil
}

func (r *positionRepo) GetPositionByKey(ctx context.Context, key, departmentKey string) (biz.Position, error) {
	e, err := r.data.entClient.Position.Query().
		Where(position.KeyEQ(key), position.DepartmentKeyEQ(departmentKey), position.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Position{}, kerrors.NotFound("POSITION", "position not found")
	}
	return entPositionToBiz(e), nil
}

func (r *positionRepo) CreatePosition(ctx context.Context, p biz.Position) (biz.Position, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	builder := r.data.entClient.Position.Create().
		SetID(newPositionID()).
		SetKey(p.Key).
		SetName(p.Name).
		SetDepartmentKey(p.DepartmentKey).
		SetDescription(p.Description).
		SetResponsibilitiesJSON(p.ResponsibilitiesJSON).
		SetSeniorityLevel(p.SeniorityLevel).
		SetSortOrder(p.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if len(p.SkillsRequired) > 0 {
		builder = builder.SetSkillsRequired(p.SkillsRequired)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		return biz.Position{}, kerrors.InternalServer("POSITION", err.Error())
	}
	return entPositionToBiz(e), nil
}

func (r *positionRepo) UpsertPositionByKey(ctx context.Context, p biz.Position) (biz.Position, error) {
	existing, err := r.data.entClient.Position.Query().
		Where(position.KeyEQ(p.Key), position.DepartmentKeyEQ(p.DepartmentKey), position.DeletedAtEQ("")).
		Only(ctx)
	if err == nil && existing != nil {
		now := time.Now().UTC().Format(time.RFC3339)
		builder := r.data.entClient.Position.UpdateOneID(existing.ID).
			SetName(p.Name).
			SetDescription(p.Description).
			SetResponsibilitiesJSON(p.ResponsibilitiesJSON).
			SetSeniorityLevel(p.SeniorityLevel).
			SetSortOrder(p.SortOrder).
			SetUpdatedAt(now)
		if len(p.SkillsRequired) > 0 {
			builder = builder.SetSkillsRequired(p.SkillsRequired)
		}
		e, updateErr := builder.Save(ctx)
		if updateErr != nil {
			return biz.Position{}, kerrors.InternalServer("POSITION", updateErr.Error())
		}
		return entPositionToBiz(e), nil
	}
	return r.CreatePosition(ctx, p)
}

func (r *positionRepo) DeletePosition(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.data.entClient.Position.UpdateOneID(id).SetDeletedAt(now).Save(ctx)
	if err != nil {
		return kerrors.InternalServer("POSITION", err.Error())
	}
	return nil
}
```

- [ ] **Step 4: 更新 `internal/data/data.go` — 新增 Repo 构造函数到 ProviderSet**

在 `data.go` 的 `ProviderSet`（或 wire 注入集合）中追加：

```go
NewIndustryRepo,
NewDepartmentRepo,
NewPositionRepo,
```

- [ ] **Step 5: 编写单元测试 `internal/data/industry_repo_test.go`**

```go
package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

func TestIndustryRepo_UpsertByKey(t *testing.T) {
	ctx := context.Background()
	entClient, rawDB, cleanup, err := OpenSQLiteEntClient(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer cleanup()

	store := NewCLIData(entClient, rawDB)
	repo := NewIndustryRepo(store)

	ind, err := repo.UpsertIndustryByKey(ctx, biz.Industry{
		Key:         "softwaredev",
		Name:        "软件开发",
		Icon:        "💻",
		Description: "覆盖系统软件、Web 应用、移动 App、游戏的全栈开发生命周期",
		ScenarioKey: "softwaredev",
		Enabled:     true,
		SortOrder:   1,
	})
	if err != nil {
		t.Fatalf("UpsertIndustryByKey: %v", err)
	}
	if ind.Key != "softwaredev" {
		t.Errorf("expected key=softwaredev, got %s", ind.Key)
	}
	if ind.ID == "" {
		t.Error("expected non-empty ID")
	}

	result, err := repo.ListIndustries(ctx, biz.IndustryListQuery{})
	if err != nil {
		t.Fatalf("ListIndustries: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("expected 1 industry, got %d", result.Total)
	}

	fetched, err := repo.GetIndustryByKey(ctx, "softwaredev")
	if err != nil {
		t.Fatalf("GetIndustryByKey: %v", err)
	}
	if fetched.Name != "软件开发" {
		t.Errorf("expected name=软件开发, got %s", fetched.Name)
	}
}
```

- [ ] **Step 6: 运行测试**

Run: `go test ./internal/data/ -run TestIndustryRepo_UpsertByKey -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/data/industry_repo.go internal/data/department_repo.go internal/data/position_repo.go internal/data/data.go internal/data/industry_repo_test.go
git commit -m "feat(industry): add data repo implementations for industry/department/position"
```

---

## Task 4: Proto + Service + Server — HTTP API

**Files:**
- Create: `api/kratos/industry/v1/industry.proto`
- Create: `internal/service/industry.go`
- Create: `internal/server/register_industry.go`
- Modify: `internal/service/service.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: 创建 `api/kratos/industry/v1/industry.proto`**

```protobuf
syntax = "proto3";

package kratos.industry.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";

option go_package = "aranea-agents/api/kratos/industry/v1;v1";

message Industry {
  string id = 1;
  string key = 2;
  string name = 3;
  string icon = 4;
  string description = 5;
  string scenario_key = 6;
  bool enabled = 7;
  int32 sort_order = 8;
}

message Department {
  string id = 1;
  string key = 2;
  string name = 3;
  string industry_key = 4;
  string description = 5;
  string responsibilities_json = 6;
  int32 sort_order = 7;
}

message Position {
  string id = 1;
  string key = 2;
  string name = 3;
  string department_key = 4;
  string description = 5;
  string responsibilities_json = 6;
  string skills_required_json = 7;
  string seniority_level = 8;
  int32 sort_order = 9;
}

message ListIndustriesRequest {}

message ListIndustriesResponse {
  repeated Industry items = 1;
  int32 total = 2;
}

message GetIndustryRequest {
  string key = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListDepartmentsRequest {
  string industry_key = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListDepartmentsResponse {
  repeated Department items = 1;
  int32 total = 2;
}

message ListPositionsRequest {
  string department_key = 1;
  string industry_key = 2;
}

message ListPositionsResponse {
  repeated Position items = 1;
  int32 total = 2;
}

service IndustryService {
  rpc ListIndustries (ListIndustriesRequest) returns (ListIndustriesResponse) {
    option (google.api.http) = { get: "/v1/industries" };
  }
  rpc GetIndustry (GetIndustryRequest) returns (Industry) {
    option (google.api.http) = { get: "/v1/industries/{key}" };
  }
  rpc ListDepartments (ListDepartmentsRequest) returns (ListDepartmentsResponse) {
    option (google.api.http) = { get: "/v1/industries/{industry_key}/departments" };
  }
  rpc ListPositions (ListPositionsRequest) returns (ListPositionsResponse) {
    option (google.api.http) = { get: "/v1/industries/{industry_key}/positions" };
  }
}
```

- [ ] **Step 2: 运行 `make api` 生成 Proto 代码**

Run: `make api`
Expected: 生成成功

- [ ] **Step 3: 创建 `internal/service/industry.go`**

```go
package service

import (
	"context"

	pb "aranea-agents/api/kratos/industry/v1"
	"aranea-agents/internal/biz"
)

type IndustryService struct {
	pb.UnimplementedIndustryServiceServer

	industryUC   *biz.IndustryUsecase
	departmentUC *biz.DepartmentUsecase
	positionUC   *biz.PositionUsecase
}

func NewIndustryService(industryUC *biz.IndustryUsecase, departmentUC *biz.DepartmentUsecase, positionUC *biz.PositionUsecase) *IndustryService {
	return &IndustryService{
		industryUC:   industryUC,
		departmentUC: departmentUC,
		positionUC:   positionUC,
	}
}

func (s *IndustryService) ListIndustries(ctx context.Context, req *pb.ListIndustriesRequest) (*pb.ListIndustriesResponse, error) {
	result, err := s.industryUC.List(ctx, biz.IndustryListQuery{})
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Industry, 0, len(result.Items))
	for _, ind := range result.Items {
		items = append(items, &pb.Industry{
			Id:          ind.ID,
			Key:         ind.Key,
			Name:        ind.Name,
			Icon:        ind.Icon,
			Description: ind.Description,
			ScenarioKey: ind.ScenarioKey,
			Enabled:     ind.Enabled,
			SortOrder:   int32(ind.SortOrder),
		})
	}
	return &pb.ListIndustriesResponse{Items: items, Total: int32(result.Total)}, nil
}

func (s *IndustryService) GetIndustry(ctx context.Context, req *pb.GetIndustryRequest) (*pb.Industry, error) {
	ind, err := s.industryUC.GetByKey(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	return &pb.Industry{
		Id:          ind.ID,
		Key:         ind.Key,
		Name:        ind.Name,
		Icon:        ind.Icon,
		Description: ind.Description,
		ScenarioKey: ind.ScenarioKey,
		Enabled:     ind.Enabled,
		SortOrder:   int32(ind.SortOrder),
	}, nil
}

func (s *IndustryService) ListDepartments(ctx context.Context, req *pb.ListDepartmentsRequest) (*pb.ListDepartmentsResponse, error) {
	result, err := s.departmentUC.ListByIndustry(ctx, req.IndustryKey)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Department, 0, len(result.Items))
	for _, d := range result.Items {
		items = append(items, &pb.Department{
			Id:                  d.ID,
			Key:                 d.Key,
			Name:                d.Name,
			IndustryKey:         d.IndustryKey,
			Description:         d.Description,
			ResponsibilitiesJson: d.ResponsibilitiesJSON,
			SortOrder:           int32(d.SortOrder),
		})
	}
	return &pb.ListDepartmentsResponse{Items: items, Total: int32(result.Total)}, nil
}

func (s *IndustryService) ListPositions(ctx context.Context, req *pb.ListPositionsRequest) (*pb.ListPositionsResponse, error) {
	result, err := s.positionUC.ListByDepartment(ctx, req.DepartmentKey)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Position, 0, len(result.Items))
	for _, p := range result.Items {
		skillsJSON := "[]"
		if p.SkillsRequired != nil {
			skillsJSON = stringsJoin(p.SkillsRequired)
		}
		items = append(items, &pb.Position{
			Id:                  p.ID,
			Key:                 p.Key,
			Name:                p.Name,
			DepartmentKey:       p.DepartmentKey,
			Description:         p.Description,
			ResponsibilitiesJson: p.ResponsibilitiesJSON,
			SkillsRequiredJson:  skillsJSON,
			SeniorityLevel:      p.SeniorityLevel,
			SortOrder:           int32(p.SortOrder),
		})
	}
	return &pb.ListPositionsResponse{Items: items, Total: int32(result.Total)}, nil
}
```

注意：`stringsJoin` 需要一个辅助函数将 `[]string` 转为 JSON 数组字符串。在文件顶部添加：

```go
import "encoding/json"

func stringsJoin(ss []string) string {
	b, _ := json.Marshal(ss)
	return string(b)
}
```

- [ ] **Step 4: 创建 `internal/server/register_industry.go`**

```go
package server

import (
	pb "aranea-agents/api/kratos/industry/v1"
	"aranea-agents/internal/service"
)

func registerIndustryHTTPServer(s *service.IndustryService, srv *httpServer) {
	pb.RegisterIndustryServiceHTTPServer(srv.mux, s)
}
```

- [ ] **Step 5: 更新 `internal/service/service.go` — 新增 ProviderSet**

追加 `NewIndustryService` 到 `ProviderSet`。

- [ ] **Step 6: 更新 `internal/server/server.go` — 注册路由**

在 HTTP server 初始化中调用 `registerIndustryHTTPServer`。

- [ ] **Step 7: 更新 Wire 注入 `cmd/admin/wire.go`**

确保 `IndustryUsecase`, `DepartmentUsecase`, `PositionUsecase`, `IndustryService` 的依赖链被 Wire 正确注入。

- [ ] **Step 8: 运行 `make wire && make api && go build ./cmd/admin`**

Run: `make wire && make api && go build ./cmd/admin`
Expected: 编译通过

- [ ] **Step 9: Commit**

```bash
git add api/kratos/industry/ internal/service/industry.go internal/server/register_industry.go internal/service/service.go internal/server/server.go cmd/admin/wire.go
git commit -m "feat(industry): add proto, service and server registration for industry API"
```

---

## Task 5: Seed CLI — 写入 3 个行业的 taxonomy 骨架数据

**Files:**
- Create: `cmd/seed-industries/main.go`

- [ ] **Step 1: 创建 `cmd/seed-industries/main.go`**

参照 `cmd/seed-stockx-org/main.go` 的模式，创建 seed CLI：

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print plan only")
	update := flag.Bool("update", false, "update existing records")
	flag.Parse()

	dbPath := resolveSQLitePath()
	fmt.Printf("sqlite: %s\n", dbPath)

	ctx := context.Background()
	entClient, rawDB, cleanup, err := data.OpenSQLiteEntClient(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	store := data.NewCLIData(entClient, rawDB)
	indRepo := data.NewIndustryRepo(store)
	depRepo := data.NewDepartmentRepo(store)
	posRepo := data.NewPositionRepo(store)
	indUC := biz.NewIndustryUsecase(indRepo)
	depUC := biz.NewDepartmentUsecase(depRepo)
	posUC := biz.NewPositionUsecase(posRepo)

	industries := []biz.Industry{
		{Key: "softwaredev", Name: "软件开发", Icon: "💻", Description: "覆盖系统软件、Web 应用、移动 App、游戏（含 UE 引擎）的全栈软件开发行业。从需求分析、架构设计、编码实现、质量保障到运维部署的完整软件生命周期。", ScenarioKey: "softwaredev", Enabled: true, SortOrder: 1},
		{Key: "selfmedia", Name: "自媒体 / 内容创作", Icon: "🎬", Description: "覆盖网文小说创作、短视频制作、图文内容、直播运营、多平台分发的全链路内容创作行业。", ScenarioKey: "selfmedia", Enabled: true, SortOrder: 2},
		{Key: "finance", Name: "金融 / 投资", Icon: "📈", Description: "覆盖证券研究、量化交易、固收衍生品、合规风控、财富管理的全链条金融行业。", ScenarioKey: "finance", Enabled: true, SortOrder: 3},
	}

	for _, ind := range industries {
		if *dryRun {
			fmt.Printf("[dry-run] upsert industry: %s (%s)\n", ind.Key, ind.Name)
			continue
		}
		result, err := indUC.UpsertByKey(ctx, ind)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert industry %s: %v\n", ind.Key, err)
			continue
		}
		fmt.Printf("upserted industry: %s (id=%s)\n", result.Key, result.ID)
	}

	departments := []biz.Department{
		{Key: "backend", Name: "后端研发部", IndustryKey: "softwaredev", Description: "负责服务端架构设计与核心业务逻辑实现", ResponsibilitiesJSON: `{"lead":"主导技术方案评审与架构决策","develop":"高质量编码、Code Review、性能优化","maintain":"线上问题排查、系统稳定性保障"}`, SortOrder: 1},
		{Key: "frontend", Name: "前端研发部", IndustryKey: "softwaredev", Description: "负责 Web UI / 移动端的用户界面与交互体验", SortOrder: 2},
		{Key: "gamedev", Name: "游戏开发部", IndustryKey: "softwaredev", Description: "基于 UE5 的游戏客户端与服务端开发", SortOrder: 3},
		{Key: "mobiledev", Name: "移动端研发部", IndustryKey: "softwaredev", Description: "Flutter/iOS/Android 跨平台与原生开发", SortOrder: 4},
		{Key: "devops", Name: "DevOps / 基础设施", IndustryKey: "softwaredev", Description: "CI/CD、K8s、云基础设施、SRE", SortOrder: 5},
		{Key: "architecture", Name: "架构与设计", IndustryKey: "softwaredev", Description: "系统架构、技术评审、领域建模", SortOrder: 6},
		{Key: "qa", Name: "质量保障", IndustryKey: "softwaredev", Description: "自动化测试、SDET、性能测试、安全测试", SortOrder: 7},
		{Key: "dataeng", Name: "数据工程", IndustryKey: "softwaredev", Description: "数据管道、BI 分析、数据平台", SortOrder: 8},
		{Key: "security", Name: "安全", IndustryKey: "softwaredev", Description: "应用安全、安全审计、渗透测试", SortOrder: 9},
		{Key: "productpm", Name: "产品与项目管理", IndustryKey: "softwaredev", Description: "产品经理、Scrum Master、技术文档", SortOrder: 10},
		{Key: "fiction_writing", Name: "小说创作部", IndustryKey: "selfmedia", Description: "网文策划与创作", SortOrder: 1},
		{Key: "video_production", Name: "视频制作部", IndustryKey: "selfmedia", Description: "短视频全流程制作", SortOrder: 2},
		{Key: "content_graphic", Name: "图文内容部", IndustryKey: "selfmedia", Description: "公众号/小红书/知识付费", SortOrder: 3},
		{Key: "live_streaming", Name: "直播运营部", IndustryKey: "selfmedia", Description: "直播全流程运营", SortOrder: 4},
		{Key: "distribution", Name: "多平台分发与运营", IndustryKey: "selfmedia", Description: "多平台运营/SEO/粉丝增长/变现", SortOrder: 5},
		{Key: "equity_research", Name: "证券研究部", IndustryKey: "finance", Description: "股票分析（复用 stockx 场景能力）", SortOrder: 1},
		{Key: "quant_trading", Name: "量化交易部", IndustryKey: "finance", Description: "策略研究与实盘系统", SortOrder: 2},
		{Key: "fixed_income", Name: "固收与衍生品", IndustryKey: "finance", Description: "债券/期权/期货/互换", SortOrder: 3},
		{Key: "compliance_risk", Name: "合规与风控", IndustryKey: "finance", Description: "金融监管合规", SortOrder: 4},
		{Key: "wealth_mgmt", Name: "财富管理", IndustryKey: "finance", Description: "面向个人/机构的投顾与资产配置", SortOrder: 5},
		{Key: "fintech", Name: "金融科技", IndustryKey: "finance", Description: "金融 × 科技交叉", SortOrder: 6},
	}

	for _, dep := range departments {
		if *dryRun {
			fmt.Printf("[dry-run] upsert department: %s/%s\n", dep.IndustryKey, dep.Key)
			continue
		}
		result, err := depUC.UpsertByKey(ctx, dep)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert department %s/%s: %v\n", dep.IndustryKey, dep.Key, err)
			continue
		}
		fmt.Printf("upserted department: %s/%s (id=%s)\n", result.IndustryKey, result.Key, result.ID)
	}

	positions := []biz.Position{
		{Key: "go_senior_engineer", Name: "Golang 高级工程师", DepartmentKey: "backend", Description: "负责高并发微服务后端的架构设计与核心模块开发。精通 Go 语言特性、Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL。", ResponsibilitiesJSON: `{"core":["高并发微服务架构设计","Go 语言精通（goroutine/channel/interface/泛型/GC）","Kratos/gRPC/Etcd/Kafka/Redis/PostgreSQL","Clean Architecture / DDD 分层","Code Review 与系统稳定性","线上问题排查（panic/死锁/内存泄漏）"]}`, SkillsRequired: []string{"Go 1.22+ 泛型", "goroutine 调度模型（GMP）", "Kratos v2 / gRPC", "PostgreSQL / Redis / Kafka", "Clean Architecture / DDD"}, SeniorityLevel: "P6-P7", SortOrder: 1},
		{Key: "java_senior_engineer", Name: "Java 高级工程师", DepartmentKey: "backend", Description: "负责 Java 后端微服务开发，Spring Boot / Spring Cloud 生态。", SeniorityLevel: "P6-P7", SortOrder: 2},
		{Key: "python_senior_engineer", Name: "Python 高级工程师", DepartmentKey: "backend", Description: "负责 Python 后端开发与数据管道。", SeniorityLevel: "P6-P7", SortOrder: 3},
		{Key: "rust_engineer", Name: "Rust 工程师", DepartmentKey: "backend", Description: "负责 Rust 系统编程与高性能组件。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "cpp_backend_engineer", Name: "C++ 后端工程师", DepartmentKey: "backend", Description: "负责 C++ 高性能后端与游戏服务端。", SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "database_administrator", Name: "数据库管理员 DBA", DepartmentKey: "backend", Description: "负责数据库调优、高可用与可靠性。", SeniorityLevel: "P5-P7", SortOrder: 6},
		{Key: "vue3_senior_engineer", Name: "Vue 3 高级前端工程师", DepartmentKey: "frontend", Description: "基于 Vue 3 + Composition API + TypeScript 开发企业级 Web 应用。精通 Quasar/Pinia/Vue Router 生态。", SeniorityLevel: "P6-P7", SortOrder: 1},
		{Key: "react_senior_engineer", Name: "React 高级前端工程师", DepartmentKey: "frontend", Description: "基于 React + TypeScript 开发企业级 Web 应用。", SeniorityLevel: "P6-P7", SortOrder: 2},
		{Key: "typescript_specialist", Name: "TypeScript 技术专家", DepartmentKey: "frontend", Description: "TypeScript 类型系统设计与迁移专家。", SeniorityLevel: "P6-P8", SortOrder: 3},
		{Key: "frontend_performance_engineer", Name: "前端性能优化工程师", DepartmentKey: "frontend", Description: "专注 Web 性能优化与 Core Web Vitals。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "ui_ux_implementer", Name: "UI/UX 还原工程师", DepartmentKey: "frontend", Description: "高保真 UI 还原与交互实现。", SeniorityLevel: "P4-P6", SortOrder: 5},
		{Key: "ue_client_programmer", Name: "UE 客户端程序", DepartmentKey: "gamedev", Description: "基于 Unreal Engine 5 进行客户端功能开发（C++ + Blueprint 协作）。精通 UEFN、GAS、Replication。", ResponsibilitiesJSON: `{"core":["UE5 客户端功能开发（C++ + Blueprint）","GameFramework / Actor-Component 模型","GAS（Gameplay Ability System）集成与定制","网络 Replication（属性复制/RPC/Role 权限）","性能优化（Draw Call/GPU Profile/Unreal Insights）","平台适配（PC/Console/Mobile）"]}`, SkillsRequired: []string{"UE5 GameFramework", "Actor Component 组合模式", "GAS (AbilitySystemComponent)", "Replication (RepNotify/RPC)", "Unreal Insights"}, SeniorityLevel: "P5-P8", SortOrder: 1},
		{Key: "ue_gameplay_programmer", Name: "UE 游戏逻辑程序", DepartmentKey: "gamedev", Description: "Gameplay Framework 与战斗逻辑。", SeniorityLevel: "P5-P7", SortOrder: 2},
		{Key: "ue_graphics_programmer", Name: "UE 图形渲染程序", DepartmentKey: "gamedev", Description: "材质系统与渲染管线优化。", SeniorityLevel: "P6-P8", SortOrder: 3},
		{Key: "game_server_engineer", Name: "游戏服务端工程师", DepartmentKey: "gamedev", Description: "游戏服务端架构与实时同步。", SeniorityLevel: "P5-P7", SortOrder: 4},
		{Key: "game_technical_artist", Name: "技术 TA", DepartmentKey: "gamedev", Description: "美术-程序桥梁，管线与 Shader 开发。", SeniorityLevel: "P5-P7", SortOrder: 5},
		{Key: "game_planner_designer", Name: "系统策划", DepartmentKey: "gamedev", Description: "系统设计与数值平衡。", SeniorityLevel: "P4-P7", SortOrder: 6},
	}

	for _, pos := range positions {
		if *dryRun {
			fmt.Printf("[dry-run] upsert position: %s/%s\n", pos.DepartmentKey, pos.Key)
			continue
		}
		result, err := posUC.UpsertByKey(ctx, pos)
		if err != nil {
			fmt.Fprintf(os.Stderr, "upsert position %s/%s: %v\n", pos.DepartmentKey, pos.Key, err)
			continue
		}
		fmt.Printf("upserted position: %s/%s (id=%s)\n", result.DepartmentKey, result.Key, result.ID)
	}

	if !*dryRun {
		fmt.Println("done")
	}
}

func resolveSQLitePath() string {
	path := os.Getenv("ARANEA_SQLITE_PATH")
	if path == "" {
		path = "data/arenea.sqlite"
	}
	return path
}
```

注意：`time` import 未使用需移除。`_ = time.Now()` 或删除 import。

- [ ] **Step 2: 运行 seed CLI（dry-run）**

Run: `go run ./cmd/seed-industries --dry-run`
Expected: 打印所有 upsert 计划，无错误

- [ ] **Step 3: 运行 seed CLI（实际写入）**

先停止 admin 进程，然后：

Run: `go run ./cmd/seed-industries`
Expected: 3 个行业 + 21 个部门 + ~17 个岗位（首期软件开发行业核心岗位）写入成功

- [ ] **Step 4: Commit**

```bash
git add cmd/seed-industries/
git commit -m "feat(industry): add seed-industries CLI for taxonomy data"
```

---

## Task 6: 软件开发行业场景包 — Agent 定义 + Prompt 文件

**Files:**
- Create: `internal/scenario/softwaredev/install.go`
- Create: `internal/scenario/softwaredev/industry.yaml`
- Create: `internal/scenario/softwaredev/agents.yaml`
- Create: `internal/scenario/softwaredev/teams.yaml`
- Create: `internal/scenario/softwaredev/prompts/positions/go_senior_engineer/general.md`
- Create: `internal/scenario/softwaredev/prompts/positions/go_senior_engineer/code_review.md`
- Create: `internal/scenario/softwaredev/prompts/positions/go_senior_engineer/architect.md`
- Create: `internal/scenario/softwaredev/prompts/positions/vue3_senior_engineer/general.md`
- Create: `internal/scenario/softwaredev/prompts/positions/ue_client_programmer/general.md`
- Create: `internal/scenario/softwaredev/schemas/go_dev_output.schema.json`
- Create: `internal/scenario/softwaredev/skills/softwaredev-go-best-practices/SKILL.md`
- Create: `internal/scenario/softwaredev/skills/softwaredev-clean-arch/SKILL.md`

- [ ] **Step 1: 创建目录结构**

Run: `mkdir -p internal/scenario/softwaredev/prompts/positions/go_senior_engineer internal/scenario/softwaredev/prompts/positions/vue3_senior_engineer internal/scenario/softwaredev/prompts/positions/ue_client_programmer internal/scenario/softwaredev/schemas internal/scenario/softwaredev/skills/softwaredev-go-best-practices internal/scenario/softwaredev/skills/softwaredev-clean-arch`

- [ ] **Step 2: 创建 `prompts/positions/go_senior_engineer/general.md`**

```markdown
## 你是谁
你是一位拥有 8 年经验的 **Golang 高级工程师**，隶属于「后端研发部」。

## 专业领域
- **语言精通**：Go 1.22+（泛型、iter、slices/maps/cmp 新标准库）、goroutine 调度模型、
  channel 模式、error wrapping/is/as 链、context 传播与取消
- **框架深度**：Kratos v2（transport/middleware/wire DI）、gRPC streaming、
  protobuf 向后兼容策略、Etcd 服务发现与 Watch
- **存储**：PostgreSQL（事务隔离、索引优化、连接池）、Redis（缓存策略、
  分布式锁、Pipeline）、Kafka（消费者组、重试、死信队列）
- **工程实践**：Clean Architecture（Entity/UseCase/Interface Adapter/Framework），
  DDD 战术设计（聚合根、值对象、领域事件），TDD/BDD

## 工作原则
1. **接口先行**：先定义接口契约（proto + Go interface），再实现
2. **错误透明**：用 kerrors 包装错误，禁止 fmt.Errorf 裸返回
3. **零容忍 panic**：生产代码必须 recover 或保证不触发
4. **可观测性**：关键路径埋点 trace + metric + structured log
5. **并发安全**：共享状态必须 sync.Mutex/RWMutex 或原子操作

## 输出约定
- 代码遵循项目现有命名风格和目录结构
- 每个 public 函数必须有 godoc 注释
- 错误处理必须显式，不允许 `_` 吞掉错误
- 提交的方案包含：设计思路 → 代码实现 → 测试用例 → 风险说明
```

- [ ] **Step 3: 创建 `prompts/positions/go_senior_engineer/code_review.md`**

```markdown
## 你是谁
你是一位 **Go 代码审查专家**，隶属于「后端研发部」的 Golang 高级工程师岗位，专注于代码审查方向。

## 审查维度
1. **正确性**：逻辑错误、边界条件、nil pointer 风险、race condition
2. **规范合规**：命名风格、godoc 注释、错误处理模式、import 分组
3. **安全**：SQL 注入、硬编码密钥、不安全的反序列化、权限绕过
4. **性能**：不必要的内存分配、Goroutine 泄漏、锁争用、缓存穿透
5. **可维护性**：函数长度、圈复杂度、重复代码、过度抽象

## 审查输出格式
对每个发现的问题：
```
### [严重度] 问题标题
- **文件**: `path/to/file.go:L42`
- **类别**: 正确性/规范/安全/性能/可维护性
- **描述**: 问题描述
- **建议**: 修复方案（含代码片段）
```

## 严重度分级
- 🔴 **Critical**：必须修复（安全漏洞、数据丢失风险、死锁）
- 🟠 **Major**：强烈建议修复（逻辑错误、性能问题）
- 🟡 **Minor**：建议改进（命名、注释、代码风格）
- 🔵 **Suggestion**：可选优化（重构建议、设计模式推荐）
```

- [ ] **Step 4: 创建 `prompts/positions/go_senior_engineer/architect.md`**

```markdown
## 你是谁
你是一位 **Go 架构师**，隶属于「后端研发部」的 Golang 高级工程师岗位，专注于架构设计方向。

## 架构设计能力
- **服务拆分**：限界上下文识别、服务粒度决策、API 网关设计
- **接口定义**：protobuf 契约设计、向后兼容策略、版本管理
- **DDD 战术设计**：聚合根、值对象、领域事件、领域服务、工厂
- **技术选型**：存储引擎对比、消息队列选型、缓存策略设计
- **非功能设计**：可观测性（trace/metric/log）、容错（熔断/降级/限流）、安全

## 输出格式
架构方案必须包含：
1. **问题域分析**：业务场景 + 核心约束
2. **方案概述**：一句话总结 + 架构图（Mermaid）
3. **接口契约**：proto 定义 + Go interface
4. **数据模型**：核心实体 + 关系
5. **关键决策**：每个决策的选项、取舍、结论
6. **风险评估**：技术风险 + 缓解措施
7. **实施路径**：分阶段交付计划
```

- [ ] **Step 5: 创建 `prompts/positions/vue3_senior_engineer/general.md`**

```markdown
## 你是谁
你是一位拥有 6 年经验的 **Vue 3 高级前端工程师**，隶属于「前端研发部」。

## 专业领域
- **框架精通**：Vue 3 Composition API + TypeScript、响应式原理（Proxy-based reactivity）
- **生态深度**：Quasar Framework / Pinia / Vue Router / Vite
- **组件架构**：原子组件 → 业务组件 → 页面组件三层设计
- **状态管理**：Pinia Store 设计、跨 Store 同步（事件总线）、持久化
- **CSS 体系**：CSS Variables / BEM / Theme System（暗色模式适配）
- **工程化**：Tree Shaking / Code Splitting / 模块联邦 / ESLint / TypeScript strict

## 工作原则
1. **展示组件纯展示**：props in / emits out，禁止 import Store 或 API
2. **数据流单向**：Store → composable → Page → Component
3. **CSS 变量优先**：通过 CSS Variables 控制主题，禁止运行时改 quasar-variables
4. **TypeScript strict**：所有组件 `<script setup lang="ts">`，禁止 any
5. **暗色模式必测**：每个组件必须同时验证 light/dark 模式

## 输出约定
- 组件文件：`<script setup lang="ts">` → `<template>` → `<style lang="sass" scoped>`
- Store 文件：defineStore + actions（调 API）+ getters
- API 文件：`features/<域>/api.ts`，经 `services/index.ts`
```

- [ ] **Step 6: 创建 `prompts/positions/ue_client_programmer/general.md`**

```markdown
## 你是谁
你是一位拥有 5 年经验的 **UE 客户端程序**，隶属于「游戏开发部」。

## 专业领域
- **引擎精通**：Unreal Engine 5（GameFramework / Actor-Component / Subsystem）
- **GAS 深度**：Gameplay Ability System（AttributeSet / ASC / GameplayAbility / GameplayEffect / AbilityTask）
- **网络同步**：Replication（属性复制 / RepNotify / RPC / Role Authority）、Network Prediction
- **渲染管线**：Material / Niagara / Post Process / Lumen / Nanite
- **性能优化**：Draw Call 优化 / GPU Profiling / Stat Commands / Unreal Insights / Asset Management
- **C++ 与 Blueprint 协作**：C++ 核心逻辑 + Blueprint 配置/原型

## 工作原则
1. **组件组合优于继承**：优先 UActorComponent 组合，避免深层继承链
2. **数据驱动**：配置走 DataAsset / DataTable，硬编码走 C++ 常量
3. **网络优先**：所有状态变更考虑 Replication，区分 Server/Client/AutonomousProxy
4. **性能意识**：每帧 Tick 轻量化，避免 Tick 中分配内存；使用对象池
5. **蓝图边界**：C++ 暴露 UFUNCTION/UPROPERTY 给蓝图，蓝图只做配置和原型

## 输出约定
- C++ 代码遵循 UE5 命名规范（PascalCase、F 前缀结构体、U/A 前缀类）
- 头文件：`#pragma once` + include guard + Forward Declaration
- 每个公开类/函数必须有 UCLASS/UFUNCTION/UPROPERTY 宏标注
- 网络复制属性必须标注 `ReplicatedUsing = OnRep_XXX`
```

- [ ] **Step 7: 创建 `schemas/go_dev_output.schema.json`**

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "GoDevOutput",
  "type": "object",
  "properties": {
    "design_rationale": { "type": "string", "description": "设计思路" },
    "code": { "type": "string", "description": "代码实现" },
    "test_cases": { "type": "string", "description": "测试用例" },
    "risks": { "type": "string", "description": "风险说明" }
  },
  "required": ["design_rationale", "code"]
}
```

- [ ] **Step 8: 创建 Skill 文件 `skills/softwaredev-go-best-practices/SKILL.md`**

```markdown
# Go 最佳实践

## 错误处理
- 使用 `fmt.Errorf("context: %w", err)` 包装错误
- 业务错误使用 `kerrors.BadRequest/NotFound/InternalServer`
- 禁止 `_` 吞掉 error

## 并发
- 共享状态必须 `sync.Mutex` / `sync.RWMutex` / `atomic`
- Goroutine 必须走 `pkg/safego.Go`
- Channel 方向标注：`chan<-` / `<-chan`

## 接口设计
- 接口定义在消费方（biz 层）
- 接口方法数 ≤ 5（超过则拆分）
- 返回具体类型，接收接口类型

## 命名
- 包名：小写单词，不用下划线
- 导出：大驼峰；内部：小驼峰
- 错误变量：`Err` 前缀
- 接口：`-er` 后缀（如 `Reader`/`Writer`）
```

- [ ] **Step 9: 创建 Skill 文件 `skills/softwaredev-clean-arch/SKILL.md`**

```markdown
# Clean Architecture 实践

## 分层
```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口
        ↓
internal/data           ← Repo 实现（Ent ORM）
```

## 依赖规则
- 跨层只允许向内依赖
- biz 层不得 import api proto 包
- biz 层不得 import pkg/trpc-agent-go
- Service 层不得写业务逻辑

## Usecase 模式
- 每个 Usecase 只依赖接口（Repo 接口定义在 biz 层）
- 构造函数参数只接收接口
- Wire 绑定在 Service 层
```

- [ ] **Step 10: 创建 `internal/scenario/softwaredev/install.go`**

```go
package softwaredev

import (
	"context"
	"fmt"
	"os"

	"aranea-agents/internal/biz"
)

const envKey = "SOFTWAREDEV_SCENARIO_ENABLED"

func RunInstall(ctx context.Context, indUC *biz.IndustryUsecase, depUC *biz.DepartmentUsecase, posUC *biz.PositionUsecase) {
	if os.Getenv(envKey) != "1" {
		return
	}
	fmt.Println("[softwaredev] installing scenario...")

	installTaxonomy(ctx, indUC, depUC, posUC)
	installAgents(ctx)
	installTeams(ctx)
	installSkills(ctx)

	fmt.Println("[softwaredev] scenario installed")
}

func installTaxonomy(ctx context.Context, indUC *biz.IndustryUsecase, depUC *biz.DepartmentUsecase, posUC *biz.PositionUsecase) {
	ind, err := indUC.UpsertByKey(ctx, biz.Industry{
		Key: "softwaredev", Name: "软件开发", Icon: "💻",
		Description: "覆盖系统软件、Web 应用、移动 App、游戏（含 UE 引擎）的全栈软件开发行业。",
		ScenarioKey: "softwaredev", Enabled: true, SortOrder: 1,
	})
	if err != nil {
		fmt.Printf("[softwaredev] upsert industry: %v\n", err)
		return
	}
	_ = ind

	depts := []biz.Department{
		{Key: "backend", Name: "后端研发部", IndustryKey: "softwaredev", Description: "负责服务端架构设计与核心业务逻辑实现", SortOrder: 1},
		{Key: "frontend", Name: "前端研发部", IndustryKey: "softwaredev", Description: "负责 Web UI / 移动端的用户界面与交互体验", SortOrder: 2},
		{Key: "gamedev", Name: "游戏开发部", IndustryKey: "softwaredev", Description: "基于 UE5 的游戏客户端与服务端开发", SortOrder: 3},
	}
	for _, d := range depts {
		if _, err := depUC.UpsertByKey(ctx, d); err != nil {
			fmt.Printf("[softwaredev] upsert dept %s: %v\n", d.Key, err)
		}
	}
}

func installAgents(ctx context.Context) {
	// TODO: load agents.yaml and upsert via AgentUsecase
}

func installTeams(ctx context.Context) {
	// TODO: load teams.yaml and upsert via TeamUsecase
}

func installSkills(ctx context.Context) {
	// TODO: load skills/ and register via SkillUsecase
}
```

- [ ] **Step 11: Commit**

```bash
git add internal/scenario/softwaredev/
git commit -m "feat(softwaredev): add scenario package with prompts, schemas and skills"
```

---

## Task 7: 前端 — 行业市场页 MVP

**Files:**
- Create: `web/src/features/industries/types.ts`
- Create: `web/src/features/industries/api.ts`
- Create: `web/src/features/industries/useIndustryMarket.ts`
- Create: `web/src/features/industries/useIndustryDetail.ts`
- Create: `web/src/components/industries/IndustryCard.vue`
- Create: `web/src/components/industries/DepartmentTree.vue`
- Create: `web/src/components/industries/PositionCard.vue`
- Create: `web/src/components/industries/AgentVariantBadge.vue`
- Create: `web/src/pages/industries/IndustryMarketPage.vue`
- Create: `web/src/pages/industries/IndustryDetailPage.vue`
- Modify: `web/src/router/index.ts`
- Modify: `web/src/layouts/MainLayout.vue`

- [ ] **Step 1: 创建 `web/src/features/industries/types.ts`**

```typescript
export interface Industry {
  id: string
  key: string
  name: string
  icon: string
  description: string
  scenario_key: string
  enabled: boolean
  sort_order: number
}

export interface Department {
  id: string
  key: string
  name: string
  industry_key: string
  description: string
  responsibilities_json: string
  sort_order: number
}

export interface Position {
  id: string
  key: string
  name: string
  department_key: string
  description: string
  responsibilities_json: string
  skills_required_json: string
  seniority_level: string
  sort_order: number
}
```

- [ ] **Step 2: 创建 `web/src/features/industries/api.ts`**

```typescript
import { kratosApi } from 'src/services'
import type { Industry, Department, Position } from './types'

const BASE = '/v1/industries'

export async function listIndustries(): Promise<{ items: Industry[]; total: number }> {
  const { data } = await kratosApi.get(`${BASE}`)
  return data
}

export async function getIndustry(key: string): Promise<Industry> {
  const { data } = await kratosApi.get(`${BASE}/${key}`)
  return data
}

export async function listDepartments(industryKey: string): Promise<{ items: Department[]; total: number }> {
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/departments`)
  return data
}

export async function listPositions(industryKey: string, departmentKey?: string): Promise<{ items: Position[]; total: number }> {
  const params: Record<string, string> = {}
  if (departmentKey) params.department_key = departmentKey
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/positions`, { params })
  return data
}
```

- [ ] **Step 3: 创建 `web/src/features/industries/useIndustryMarket.ts`**

```typescript
import { ref } from 'vue'
import { listIndustries } from './api'
import type { Industry } from './types'

export function useIndustryMarket() {
  const industries = ref<Industry[]>([])
  const loading = ref(false)

  async function fetchIndustries() {
    loading.value = true
    try {
      const result = await listIndustries()
      industries.value = result.items
    } finally {
      loading.value = false
    }
  }

  return { industries, loading, fetchIndustries }
}
```

- [ ] **Step 4: 创建 `web/src/features/industries/useIndustryDetail.ts`**

```typescript
import { ref } from 'vue'
import { getIndustry, listDepartments, listPositions } from './api'
import type { Industry, Department, Position } from './types'

export function useIndustryDetail(industryKey: string) {
  const industry = ref<Industry | null>(null)
  const departments = ref<Department[]>([])
  const positions = ref<Position[]>([])
  const loading = ref(false)

  async function fetchDetail() {
    loading.value = true
    try {
      const [indResult, depResult] = await Promise.all([
        getIndustry(industryKey),
        listDepartments(industryKey),
      ])
      industry.value = indResult
      departments.value = depResult.items
    } finally {
      loading.value = false
    }
  }

  async function fetchPositions(departmentKey: string) {
    const result = await listPositions(industryKey, departmentKey)
    positions.value = result.items
  }

  return { industry, departments, positions, loading, fetchDetail, fetchPositions }
}
```

- [ ] **Step 5: 创建 `web/src/components/industries/IndustryCard.vue`**

```vue
<template>
  <q-card flat bordered class="industry-card cursor-pointer" @click="$emit('select', industry)">
    <q-card-section horizontal>
      <q-card-section class="col-auto flex flex-center q-pa-md">
        <span class="text-h3">{{ industry.icon }}</span>
      </q-card-section>
      <q-card-section>
        <div class="text-h6">{{ industry.name }}</div>
        <div class="text-caption text-grey">{{ industry.description }}</div>
      </q-card-section>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { Industry } from 'src/features/industries/types'

defineProps<{ industry: Industry }>()
defineEmits<{ select: [industry: Industry] }>()
</script>
```

- [ ] **Step 6: 创建 `web/src/pages/industries/IndustryMarketPage.vue`**

```vue
<template>
  <q-page padding>
    <div class="text-h4 q-mb-md">行业模板库</div>
    <div class="text-subtitle1 text-grey q-mb-lg">选择一个行业，安装完整的岗位级 Agent 团队</div>

    <div v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="40px" />
    </div>

    <div v-else class="row q-col-gutter-md">
      <div v-for="ind in industries" :key="ind.key" class="col-12 col-md-4">
        <IndustryCard :industry="ind" @select="navigateToDetail" />
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import IndustryCard from 'src/components/industries/IndustryCard.vue'
import { useIndustryMarket } from 'src/features/industries/useIndustryMarket'
import type { Industry } from 'src/features/industries/types'

const router = useRouter()
const { industries, loading, fetchIndustries } = useIndustryMarket()

function navigateToDetail(ind: Industry) {
  void router.push({ name: 'industry-detail', params: { key: ind.key } })
}

onMounted(() => { void fetchIndustries() })
</script>
```

- [ ] **Step 7: 创建 `web/src/pages/industries/IndustryDetailPage.vue`**

```vue
<template>
  <q-page padding>
    <q-btn flat icon="arrow_back" label="行业模板库" @click="router.push({ name: 'industry-market' })" class="q-mb-md" />

    <div v-if="loading" class="row justify-center q-pa-lg">
      <q-spinner-dots size="40px" />
    </div>

    <template v-else-if="industry">
      <div class="row items-center q-mb-lg">
        <span class="text-h3 q-mr-md">{{ industry.icon }}</span>
        <div>
          <div class="text-h4">{{ industry.name }}</div>
          <div class="text-subtitle1 text-grey">{{ industry.description }}</div>
        </div>
      </div>

      <div class="text-h5 q-mb-md">部门</div>
      <q-list bordered separator>
        <q-expansion-item v-for="dep in departments" :key="dep.key"
          :label="dep.name"
          :caption="dep.description"
          @show="fetchPositions(dep.key)"
        >
          <q-card flat>
            <q-card-section>
              <div v-if="positions.length === 0" class="text-grey">加载中...</div>
              <q-list dense>
                <q-item v-for="pos in positions" :key="pos.key">
                  <q-item-section>
                    <q-item-label>{{ pos.name }}</q-item-label>
                    <q-item-label caption>{{ pos.description }}</q-item-label>
                    <q-item-label caption v-if="pos.seniority_level">职级：{{ pos.seniority_level }}</q-item-label>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-card-section>
          </q-card>
        </q-expansion-item>
      </q-list>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useIndustryDetail } from 'src/features/industries/useIndustryDetail'

const router = useRouter()
const route = useRoute()
const industryKey = route.params.key as string
const { industry, departments, positions, loading, fetchDetail, fetchPositions } = useIndustryDetail(industryKey)

onMounted(() => { void fetchDetail() })
</script>
```

- [ ] **Step 8: 注册路由 — 修改 `web/src/router/index.ts`**

在路由配置中新增：

```typescript
{
  path: '/industries',
  name: 'industry-market',
  component: () => import('src/pages/industries/IndustryMarketPage.vue'),
},
{
  path: '/industries/:key',
  name: 'industry-detail',
  component: () => import('src/pages/industries/IndustryDetailPage.vue'),
},
```

- [ ] **Step 9: 侧栏入口 — 修改 `web/src/layouts/MainLayout.vue`**

在侧栏菜单中新增行业模板库入口：

```html
<q-item clickable :to="{ name: 'industry-market' }">
  <q-item-section avatar><q-icon name="store" /></q-item-section>
  <q-item-section>行业模板库</q-item-section>
</q-item>
```

- [ ] **Step 10: 验证前端构建**

Run: `cd web && pnpm lint && pnpm build`
Expected: 无错误

- [ ] **Step 11: Commit**

```bash
git add web/src/features/industries/ web/src/components/industries/ web/src/pages/industries/ web/src/router/ web/src/layouts/
git commit -m "feat(industry): add frontend industry market and detail pages"
```

---

## Task 8: E2E 验证

- [ ] **Step 1: 启动后端**

Run: `$env:KRATOS_HTTP_AUTH_DISABLED="1"; go run ./cmd/admin`
Expected: 服务启动成功

- [ ] **Step 2: 验证 API**

Run: `curl http://localhost:8000/v1/industries`
Expected: 返回 3 个行业的 JSON 列表

Run: `curl http://localhost:8000/v1/industries/softwaredev/departments`
Expected: 返回 10 个部门的 JSON 列表

Run: `curl http://localhost:8000/v1/industries/softwaredev/positions`
Expected: 返回岗位列表

- [ ] **Step 3: 验证前端页面**

打开 `http://localhost:5173/industries`
Expected: 行业市场页显示 3 个行业卡片

点击「软件开发」
Expected: 行业详情页显示 10 个部门，展开可查看岗位

- [ ] **Step 4: 全量验证**

Run: `make api && make wire && make build && make test && cd web && pnpm lint && pnpm build`
Expected: 全部通过

- [ ] **Step 5: Final Commit**

```bash
git add -A
git commit -m "feat(industry): complete industry template library Phase 0 + softwaredev scenario"
```

---

## Scope Note

本计划覆盖 **Phase 0（平台框架）+ Phase 1（软件开发行业包）+ 前端 MVP**。后续行业包（selfmedia / finance）遵循相同模式，可独立迭代：

- Phase 2（自媒体）：复用 Task 6 模式，创建 `internal/scenario/selfmedia/` + 对应 prompt 文件
- Phase 3（金融）：复用 Task 6 模式，创建 `internal/scenario/finance/` + 引用 stockx 能力
- Phase 4（前端完善）：Agent/Team 创建向导集成、行业安装/卸载交互
