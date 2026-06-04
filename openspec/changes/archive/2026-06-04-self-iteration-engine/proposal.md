## Why

Aranea-Agents 当前缺乏闭环自我迭代能力：CI 仅做 lint/test/smoke，无自动修复、无自动发布、无文档同步。每次 CI 失败需人工诊断修复，每次发布需手动操作，每次代码变更后文档需手动更新。受美国火箭发动机自我迭代模式启发（点火→传感器采集→自动分析→参数调优→重新点火），需要构建从"失败检测→AI诊断→自动修复→验证→发布→文档同步"的全链路自动化闭环，让系统从每次运行中学习并持续优化自身。

### 火箭发动机类比

```
火箭发动机迭代循环                    Aranea-Agents 自我迭代循环
═══════════════════                  ════════════════════════════

  ┌──────────┐                        ┌──────────────┐
  │ 设计迭代  │                        │ OpenSpec 提案 │
  └────┬─────┘                        └──────┬───────┘
       ▼                                     ▼
  ┌──────────┐                        ┌──────────────┐
  │ 点火测试  │                        │  自动化测试   │
  └────┬─────┘                        └──────┬───────┘
       ▼                                     ▼
  ┌──────────┐                        ┌──────────────┐
  │ 传感器采集│ ◄── 1000+ 传感器       │ 遥测+覆盖率  │ ◄── 指标采集
  └────┬─────┘                        └──────┬───────┘
       ▼                                     ▼
  ┌──────────┐                        ┌──────────────┐
  │ 自动分析  │ ◄── 失败模式识别       │ AI 诊断+修复  │ ◄── LLM 分析
  └────┬─────┘                        └──────┬───────┘
       ▼                                     ▼
  ┌──────────┐                        ┌──────────────┐
  │ 参数调优  │ ◄── 闭环反馈           │ 自动修复提交  │ ◄── PR 自动生成
  └────┬─────┘                        └──────┬───────┘
       ▼                                     ▼
  ┌──────────┐                        ┌──────────────┐
  │ 重新点火  │ ◄── 无人工干预         │ 自动发布     │ ◄── CD 流水线
  └──────────┘                        └──────────────┘
```

### 现状盘点

| 能力层 | 已有 | 缺失 |
|--------|------|------|
| **自动化测试** | Go 350+ 测试文件、前端 59 spec、CI 6 Job、覆盖率阈值 40% | E2E 测试、前端组件测试、集成测试自动化、性能基准 |
| **自动化修复** | araneactl lint（12 条规则）、stylelint --fix、fix-type-imports 脚本 | AI 驱动的测试失败诊断与自动修复、lint 错误自动修复 |
| **自动化发布** | Docker 多阶段构建、Makefile build/smoke/cli 目标 | GoReleaser、CD 流水线、版本号管理、Changelog 自动生成 |
| **文档自动化** | Proto→OpenAPI/TS 代码生成、Wire 生成、OpenSpec 工作流 | API 文档自动发布、Spec 与代码同步校验、Changelog 自动生成 |

## What Changes

### 五层自动化架构

```
┌─────────────────────────────────────────────────────────────┐
│                    Layer 5: 自我迭代引擎                      │
│   失败模式库 → 迭代策略 → 自动优化 → 闭环反馈                │
├─────────────────────────────────────────────────────────────┤
│                    Layer 4: 自动发布                          │
│   版本管理 → Changelog → 构建 → 部署 → 冒烟验证              │
├─────────────────────────────────────────────────────────────┤
│                    Layer 3: 自动修复                          │
│   失败检测 → AI 诊断 → 自动修复 → 验证 → PR                  │
├─────────────────────────────────────────────────────────────┤
│                    Layer 2: 自动化测试                        │
│   单元 → 集成 → E2E → 性能 → 安全 → 覆盖率门禁              │
├─────────────────────────────────────────────────────────────┤
│                    Layer 1: 基础设施                          │
│   Git Hooks → CI Pipeline → 遥测采集 → 通知                  │
└─────────────────────────────────────────────────────────────┘
```

### 具体变更

- **Git Hooks 自动化**：新增 Husky + lint-staged + commitlint，commit 前自动 lint/format，拦截不规范提交
- **前端 Lint 补全**：新增 ESLint + Prettier，补齐 JS/TS/Vue 代码规范（当前仅有 stylelint）
- **E2E 测试框架**：新增 Playwright E2E 测试，覆盖关键用户路径（聊天流程、Agent 创建、Team 编排）
- **集成测试增强**：新增 testcontainers-go 容器化集成测试，覆盖 API 端点 + DB 交互
- **CI Pipeline 增强**：从 6 Job 扩展到 12 Job，新增 commitlint / typecheck-web / test-integration / e2e-nightly / doc-sync-check / security-scan
- **自动修复引擎**：CI 失败时自动触发 LLM 诊断→生成修复 patch→验证→创建 PR，记录失败模式到知识库
- **Lint 自动修复**：araneactl 新增 --fix 模式，golangci-lint --fix、ESLint --fix、stylelint --fix 一键修复
- **自动发布流水线**：GoReleaser + GitHub Actions，tag 推送自动构建/发布/Changelog 生成
- **文档自动同步**：代码合并后自动检测文档影响→AI 更新受影响 spec→创建文档同步 PR
- **Changelog 自动生成**：PR 合并后自动生成 openspec/changelog/ 条目
- **迭代仪表盘**：每周自动生成迭代周报（覆盖率趋势、修复成功率、发布频率）
- **失败模式知识库**：记录每次自动修复的失败模式与修复结果，供后续迭代学习

## Capabilities

### New Capabilities

- `auto-fix-engine`: CI 失败自动检测→LLM 诊断→修复生成→验证→PR 创建的闭环引擎，含失败模式知识库
- `auto-release-pipeline`: 基于 GoReleaser 的自动构建/发布/Changelog 生成流水线，含 staging 冒烟验证
- `doc-sync-engine`: 代码变更→影响分析→文档自动更新→PR 创建的文档同步引擎
- `e2e-testing`: Playwright E2E 测试框架，覆盖关键用户路径，含 nightly CI 集成
- `iteration-dashboard`: 迭代指标采集与周报自动生成（覆盖率趋势、修复成功率、发布频率）

### Modified Capabilities

- `ci-pipeline`: 从 6 Job 扩展到 12 Job，新增 commitlint / typecheck-web / test-integration / e2e-nightly / doc-sync-check / security-scan
- `lint-system`: araneactl 新增 --fix 自动修复模式，前端新增 ESLint + Prettier，新增 Husky + lint-staged Git Hooks

## Impact

- **CI/CD**：`.github/workflows/` 新增 4 个 workflow（auto-fix / release / doc-sync / changelog），ci.yml 扩展 6 个 Job
- **后端 biz/data/service 层**：无业务逻辑变更，仅新增集成测试
- **前端**：新增 ESLint + Prettier 配置，新增 Playwright E2E 测试目录
- **开发工具链**：新增 Husky + lint-staged + commitlint + GoReleaser + testcontainers-go
- **文档**：openspec/specs/ 可能需要更新 architecture-blueprint.md 和 module-cross-reference.md 以反映新增的自动化模块
- **依赖**：新增 npm 依赖（husky, lint-staged, @commitlint/*, eslint, prettier, @playwright/test）、Go 依赖（testcontainers-go）、工具依赖（goreleaser）
- **安全**：auto-fix 需要 PAT token 和 LLM API key，需妥善管理 secrets

### 工具清单汇总

| 类别 | 工具 | 用途 | 当前状态 |
|------|------|------|----------|
| **Git Hooks** | Husky + lint-staged | commit 前自动 lint | 未安装 |
| **Commit 规范** | commitlint | 提交消息格式校验 | 未安装 |
| **前端 Lint** | ESLint + Prettier | JS/TS/Vue 代码规范 | 未安装 |
| **E2E 测试** | Playwright | 浏览器端到端测试 | 未安装 |
| **集成测试** | testcontainers-go | 容器化集成测试 | 未安装 |
| **自动修复** | GitHub Actions + LLM API | CI 失败自动诊断修复 | 未安装 |
| **发布** | GoReleaser | 自动构建+发布+Changelog | 未安装 |
| **版本管理** | standard-version | 自动版本号+CHANGELOG | 未安装 |
| **文档同步** | GitHub Actions + LLM | 代码变更→文档更新 | 未安装 |
| **容器镜像** | GHCR (GitHub Container Registry) | Docker 镜像托管 | 未安装 |
| **依赖更新** | Dependabot / Renovate | 依赖自动更新 PR | 未配置 |
| **安全扫描** | CodeQL (已有) + Trivy | 容器镜像安全扫描 | CodeQL 已有 |
| **遥测** | GitHub Actions Artifacts | 迭代指标采集 | 部分已有 |
