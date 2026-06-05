# Self-iteration-remediation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修正原 self-iteration-engine 变更的 17 项假完成 + 18 项验证未执行，补齐集成测试断言，确认 ldflags 和 CI 配置正确。

**Architecture:** 纯修复变更，不新增功能。涉及 CI workflow 检查、集成测试增强（Ent Client + PostgreSQL testcontainers）、ldflags 验证、归档 tasks.md 标记。

**Tech Stack:** Go 1.23, Ent ORM, testcontainers-go (PostgreSQL), GitHub Actions CI

**Traceability (sddflow):**
- plan-ready: `openspec/changes/self-iteration-remediation/plan-ready.md`
- tasks: `openspec/changes/self-iteration-remediation/tasks.md`
- plan: `docs/superpowers/plans/2026-06-06-self-iteration-remediation.md`

---

### Task 1: 弃用 Husky/lint-staged/commitlint

> **trace:** plan-ready.md → `### Task 1: 弃用 Husky/lint-staged/commitlint` | tasks.md → `## 1. 弃用 Husky/lint-staged/commitlint`
> **sync:** tasks.md → `## 1. 弃用 Husky/lint-staged/commitlint` | plan-ready.md → `### Task 1: 弃用 Husky/lint-staged/commitlint`

**Files:**
- Verify: `.husky/pre-commit`
- Verify: `.husky/commit-msg`
- Modify: `openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`

- [x] **Step 1: Verify Husky hook deprecation comments already exist**

Read `.husky/pre-commit` and `.husky/commit-msg` to confirm they contain deprecation comments. Expected: both files contain `# DEPRECATED: This project relies on CI lint checks instead of Husky hooks`.

- [x] **Step 2: Update archive tasks.md — mark Husky tasks as deferred**

In `openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`, change tasks 1.1 through 1.4 from `- [x]` to `- [x] (deferred - Husky deprecated)`. Specifically, append ` (deferred - Husky deprecated)` after the existing checkbox marker text for items 1.1, 1.2, 1.3, 1.4. Also mark 1.5 as `- [ ] (deferred - Husky deprecated)`.

The changes should be:
- Line with `- [x] 1.1` → change to `- [x] 1.1 (deferred - Husky deprecated)`
- Line with `- [x] 1.2` → change to `- [x] 1.2 (deferred - Husky deprecated)`
- Line with `- [x] 1.3` → change to `- [x] 1.3 (deferred - Husky deprecated)`
- Line with `- [x] 1.4` → change to `- [x] 1.4 (deferred - Husky deprecated)`
- Line with `- [ ] 1.5` → change to `- [ ] 1.5 (deferred - Husky deprecated)`

- [x] **Step 3: Verify build passes**

Run: `go build ./...`
Expected: PASS (no compilation errors)

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 2: 修正 CI 配置名错误

> **trace:** plan-ready.md → `### Task 2: 修正 CI 配置名错误` | tasks.md → `## 2. 修正 CI 配置名错误`
> **sync:** tasks.md → `## 2. 修正 CI 配置名错误` | plan-ready.md → `### Task 2: 修正 CI 配置名错误`

**Files:**
- Review: `.github/workflows/ci.yml`
- Review: `.github/workflows/release.yml`
- Review: `.github/workflows/auto-fix.yml`
- Review: `.github/workflows/doc-sync.yml`
- Review: `.github/workflows/e2e-nightly.yml`
- Review: `.github/workflows/iteration-dashboard.yml`
- Review: `.github/workflows/codeql.yml`

- [x] **Step 1: Check CI job name references**

For each workflow file in `.github/workflows/`, verify that all `needs:` references match actual job names defined in the same file. Specifically check:

- `ci.yml`: `wire-clean` needs `lint` ✓, `test-go` needs `lint` ✓, `test-integration` needs `lint` ✓, `smoke` needs `test-go` ✓, `coverage-gate` needs `test-go` ✓
- `release.yml`: `release` needs `test` ✓

Also check that `release.yml` correctly references `ci.yml` via `workflow_call`.

- [x] **Step 2: Check commitlint config file reference**

In `ci.yml`, the `commitlint` job runs `npx commitlint --config .commitlintrc.yml`. Check if `.commitlintrc.yml` exists at the repo root. If it doesn't exist, the commitlint job will fail on PRs. Fix by either:
- Creating a minimal `.commitlintrc.yml` with `extends: ['@commitlint/config-conventional']`
- Or removing the `--config .commitlintrc.yml` flag (commitlint will use default config from package.json or built-in defaults)

Preferred fix: Create `.commitlintrc.yml` since CI explicitly references it.

- [x] **Step 3: Verify CI workflow syntax**

Run: Check each workflow file for YAML syntax correctness. If `actionlint` is available, run `actionlint .github/workflows/*.yml`. Otherwise, manually verify key fields (on, jobs, steps, uses, runs-on) are correct.

Expected: No job name mismatches; commitlint config file issue resolved.

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 3: 补齐集成测试断言

> **trace:** plan-ready.md → `### Task 3: 补齐集成测试断言` | tasks.md → `## 3. 补齐集成测试断言`
> **sync:** tasks.md → `## 3. 补齐集成测试断言` | plan-ready.md → `### Task 3: 补齐集成测试断言`

**Files:**
- Modify: `internal/service/chat_integration_test.go`
- Modify: `internal/service/agent_integration_test.go`

- [x] **Step 1: Write failing test — enhance chat_integration_test.go**

Replace the current skeleton test with Ent Client-based integration test. The test should:
1. Start PostgreSQL container (already done)
2. Create Ent Client with the PostgreSQL DSN
3. Run Ent schema migration
4. Test Session creation and retrieval using Ent Client
5. Assert actual data operations succeed

```go
//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/data/ent/session"
	"aranea-agents/internal/testutil"

	"aranea.io/ent/dialect"
	entsql "aranea.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

// TestIntegrationChatAPI tests Chat-related database operations with a real PostgreSQL container.
// Run with: go test -tags=integration ./internal/service/... -run TestIntegrationChatAPI -count=1
func TestIntegrationChatAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pg, cleanup, err := testutil.StartPostgres(ctx, t)
	if err != nil {
		t.Fatalf("failed to start postgres: %v", err)
	}
	defer cleanup()

	t.Logf("Postgres DSN: %s", pg.DSN())

	// Create Ent Client with PostgreSQL driver
	drv, err := entsql.Open(dialect.Postgres, pg.DSN())
	if err != nil {
		t.Fatalf("failed to open ent driver: %v", err)
	}
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// Run schema migration
	if err := client.Schema.Create(ctx, migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	t.Log("Ent schema migration completed")

	// Test: Create a session
	sess, err := client.Session.Create().
		SetTitle("Integration Test Session").
		SetSlug("integration-test-session").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	t.Logf("Created session: id=%s title=%s", sess.ID, sess.Title)

	// Assert: Session was created with correct fields
	if sess.Title != "Integration Test Session" {
		t.Fatalf("expected session title 'Integration Test Session', got %q", sess.Title)
	}
	if sess.Slug != "integration-test-session" {
		t.Fatalf("expected session slug 'integration-test-session', got %q", sess.Slug)
	}

	// Test: Retrieve session by ID
	retrieved, err := client.Session.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("failed to retrieve session: %v", err)
	}
	if retrieved.Title != sess.Title {
		t.Fatalf("expected retrieved title %q, got %q", sess.Title, retrieved.Title)
	}

	// Test: Query sessions by slug
	sessions, err := client.Session.Query().
		Where(session.Slug("integration-test-session")).
		All(ctx)
	if err != nil {
		t.Fatalf("failed to query sessions by slug: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session with slug 'integration-test-session', got %d", len(sessions))
	}

	// Test: Update session title
	updated, err := client.Session.UpdateOneID(sess.ID).
		SetTitle("Updated Integration Session").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	if updated.Title != "Updated Integration Session" {
		t.Fatalf("expected updated title 'Updated Integration Session', got %q", updated.Title)
	}

	// Test: Delete session
	err = client.Session.DeleteOneID(sess.ID).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete session: %v", err)
	}

	// Assert: Session no longer exists
	_, err = client.Session.Get(ctx, sess.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted session, got nil")
	}
	t.Log("Chat integration test passed: Session CRUD verified via Ent Client")
}
```

- [x] **Step 2: Run test to verify it compiles**

Run: `go build -tags=integration ./internal/service/...`
Expected: PASS (compiles without errors)

- [x] **Step 3: Write failing test — enhance agent_integration_test.go**

Replace the current skeleton test with Ent Client-based integration test for Agent CRUD:

```go
//go:build integration

package service

import (
	"context"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/migrate"
	"aranea-agents/internal/testutil"

	"aranea.io/ent/dialect"
	entsql "aranea.io/ent/dialect/sql"
	_ "github.com/lib/pq"
)

// TestIntegrationAgentCRUD tests Agent CRUD operations with a real PostgreSQL container.
// Run with: go test -tags=integration ./internal/service/... -run TestIntegrationAgentCRUD -count=1
func TestIntegrationAgentCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	pg, cleanup, err := testutil.StartPostgres(ctx, t)
	if err != nil {
		t.Fatalf("failed to start postgres: %v", err)
	}
	defer cleanup()

	t.Logf("Postgres DSN: %s", pg.DSN())

	// Create Ent Client with PostgreSQL driver
	drv, err := entsql.Open(dialect.Postgres, pg.DSN())
	if err != nil {
		t.Fatalf("failed to open ent driver: %v", err)
	}
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// Run schema migration
	if err := client.Schema.Create(ctx, migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	t.Log("Ent schema migration completed")

	// Test: Create an agent
	ag, err := client.Agent.Create().
		SetName("Integration Test Agent").
		SetSlug("integration-test-agent").
		SetKind("user").
		SetSystemPrompt("You are a test agent.").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	t.Logf("Created agent: id=%s name=%s kind=%s", ag.ID, ag.Name, ag.Kind)

	// Assert: Agent was created with correct fields
	if ag.Name != "Integration Test Agent" {
		t.Fatalf("expected agent name 'Integration Test Agent', got %q", ag.Name)
	}
	if ag.Kind != "user" {
		t.Fatalf("expected agent kind 'user', got %q", ag.Kind)
	}

	// Test: Retrieve agent by ID
	retrieved, err := client.Agent.Get(ctx, ag.ID)
	if err != nil {
		t.Fatalf("failed to retrieve agent: %v", err)
	}
	if retrieved.Name != ag.Name {
		t.Fatalf("expected retrieved name %q, got %q", ag.Name, retrieved.Name)
	}

	// Test: Query agents by slug
	agents, err := client.Agent.Query().
		Where(agent.Slug("integration-test-agent")).
		All(ctx)
	if err != nil {
		t.Fatalf("failed to query agents by slug: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent with slug 'integration-test-agent', got %d", len(agents))
	}

	// Test: Update agent name
	updated, err := client.Agent.UpdateOneID(ag.ID).
		SetName("Updated Integration Agent").
		Save(ctx)
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}
	if updated.Name != "Updated Integration Agent" {
		t.Fatalf("expected updated name 'Updated Integration Agent', got %q", updated.Name)
	}

	// Test: Delete agent
	err = client.Agent.DeleteOneID(ag.ID).Exec(ctx)
	if err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	// Assert: Agent no longer exists
	_, err = client.Agent.Get(ctx, ag.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted agent, got nil")
	}
	t.Log("Agent integration test passed: Agent CRUD verified via Ent Client")
}
```

- [x] **Step 4: Run test to verify it compiles**

Run: `go build -tags=integration ./internal/service/...`
Expected: PASS (compiles without errors)

- [x] **Step 5: Run integration tests (requires Docker)**

Run: `go test -tags=integration ./internal/service/... -run "TestIntegrationChatAPI|TestIntegrationAgentCRUD" -count=1 -timeout 10m`
Expected: Both tests PASS

Note: If Docker is not available in the current environment, this step can be deferred to CI. The compilation check in Steps 2 and 4 is the minimum gate.

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 4: 补齐 admin --version ldflags

> **trace:** plan-ready.md → `### Task 4: 补齐 admin --version ldflags` | tasks.md → `## 4. 补齐 admin --version ldflags`
> **sync:** tasks.md → `## 4. 补齐 admin --version ldflags` | plan-ready.md → `### Task 4: 补齐 admin --version ldflags`

**Files:**
- Verify: `Makefile` (lines 13-16)
- Verify: `cmd/admin/main.go` (lines 24-31)

- [x] **Step 1: Verify Makefile ldflags already include commit and date**

Read `Makefile` lines 13-16. Expected content:
```makefile
VERSION=$(shell git describe --tags --always)
COMMIT=$(shell git rev-parse HEAD)
BUILD_DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)
```

Confirm that `main.Commit` and `main.BuildDate` are present in LDFLAGS.

- [x] **Step 2: Verify cmd/admin/main.go has matching variable declarations**

Read `cmd/admin/main.go` lines 24-31. Expected:
```go
var (
	Name      string
	Version   string
	Commit    string
	BuildDate string
	...
)
```

Confirm that `Commit` and `BuildDate` variables exist and match the ldflags targets (`main.Commit`, `main.BuildDate`).

- [x] **Step 3: Build and verify --version output**

Run: `make build`
Then run: `./bin/admin --version`
Expected: Output contains a commit hash and build date, e.g. `aranea v0.0.1-abc1234 (commit: abc1234..., built: 2026-06-06T...)`

Note: On Windows, use `.\bin\admin.exe --version`.

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 5: Staging/Production 步骤处理

> **trace:** plan-ready.md → `### Task 5: Staging/Production 步骤处理` | tasks.md → `## 5. Staging/Production 步骤处理`
> **sync:** tasks.md → `## 5. Staging/Production 步骤处理` | plan-ready.md → `### Task 5: Staging/Production 步骤处理`

**Files:**
- Modify: `openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`
- Create: `openspec/changes/staging-deployment/proposal.md`

- [x] **Step 1: Update archive tasks.md — mark staging tasks as deferred**

In `openspec/changes/archive/2026-06-05-self-iteration-engine/tasks.md`, change tasks 7.5, 7.6, 7.7 from `- [x]` to `- [x] (deferred - no staging infra)`:

- Line with `- [x] 7.5 实现 staging 部署步骤` → change to `- [x] 7.5 (deferred - no staging infra) 实现 staging 部署步骤`
- Line with `- [x] 7.6 实现 staging 冒烟测试步骤` → change to `- [x] 7.6 (deferred - no staging infra) 实现 staging 冒烟测试步骤`
- Line with `- [x] 7.7 实现 production promote 步骤` → change to `- [x] 7.7 (deferred - no staging infra) 实现 production promote 步骤`

- [x] **Step 2: Create staging-deployment change placeholder**

Create `openspec/changes/staging-deployment/proposal.md`:

```markdown
## Why

当前项目无 staging 环境，release.yml 仅包含 GoReleaser + Docker push，缺少 staging 部署、冒烟测试和 production promote 步骤。需要基础设施先行。

## What Changes

- 新增 staging 部署步骤到 release.yml（需 K8s 集群）
- 新增 staging 冒烟测试步骤
- 新增 production promote 步骤

## Capabilities

### New Capabilities

- `staging-deployment`: Staging 环境部署 + 冒烟测试 + Production promote

### Modified Capabilities

（无）

## Impact

- **CI/CD**: `.github/workflows/release.yml` 需要新增 staging 相关 job
- **基础设施**: 需要 K8s 集群和 staging 命名空间
```

- [x] **Step 3: Verify staging-deployment change exists**

Run: `ls openspec/changes/staging-deployment/`
Expected: `proposal.md` file exists

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）

---

### Task 6: 全量验证

> **trace:** plan-ready.md → `### Task 6: 全量验证` | tasks.md → `## 6. 全量验证`
> **sync:** tasks.md → `## 6. 全量验证` | plan-ready.md → `### Task 6: 全量验证`

**Files:**
- None (verification only)

- [x] **Step 1: Backend full verification**

Run: `make api && make wire && make build && make test && make lint`
Expected: All commands PASS

Note: On Windows, run commands individually if needed. `make api` requires protoc; `make wire` requires wire; `make lint` may require golangci-lint.

- [x] **Step 2: Frontend full verification**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: All commands PASS

- [x] **Task complete**（本 Task 全部 Step 为 `[x]` 后勾选；与 plan-ready **任务完成**、tasks.md 对应行同步）
