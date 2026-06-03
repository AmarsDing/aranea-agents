## Why

当前 Agent、Team、行业分类（Taxonomy）、Graph 模板的种子/导入机制严重分裂：内置种子走 RawSQL 硬编码 80+ 列，CLI 导入（orgimport）走 HTTP API 但功能远不如内置种子完整，Graph 模板 Go 硬编码无法扩展，两套类型体系（loader.Spec vs orgimport.Spec）概念重复但能力不对等。用户无法自助导出/导入场景包，也无法跨实例分享 Agent 配置。需要一个统一的场景包（Aranea Pack）格式和导入导出引擎，彻底替代现有分裂机制，并为后续商城分享奠定基础。

## What Changes

- **新增 `.arpack` 场景包格式**：基于 tar.gz 的标准打包格式，包含 manifest.yaml + 分类/Agent/Team/Graph YAML + Agent 文件目录
- **新增 Pack 导出引擎**：支持单 Agent、单 Team、整行业三种导出粒度，自动处理 ID→Key 映射和依赖传递
- **新增 Pack 导入引擎**：支持校验（dry-run）、冲突策略（skip/overwrite/duplicate）、按依赖顺序四阶段写入
- **新增 Pack API 端点**：`/v1/packs/export`、`/v1/packs/import`、`/v1/packs/validate`
- **新增 Pack CLI 命令**：`aranea pack export/import/validate`
- **BREAKING：移除 RawSQL 种子链路**：删除 `seed_industry_agents_rawsql.go`、`seed_builtin_taxonomy.go`（RawSQL 版）、`seed_agent_templates.go`（RawSQL 版），内置数据转为 `.arpack` 格式 embed 到二进制
- **BREAKING：废弃 orgimport 包**：`internal/orgimport/` 整包废弃，其功能由 Pack 导入引擎统一替代
- **BREAKING：移除 Go 硬编码 Graph 模板**：`internal/graph/trpc/templates.go` 中的 6 个内置模板转为 `.arpack` 格式
- **内置种子迁移**：现有 YAML 数据源（taxonomy.yaml、agents.yaml、agent_templates.yaml）转为 `.arpack` 目录结构，启动时通过统一 Pack 引擎加载

## Capabilities

### New Capabilities

- `pack-format`: Aranea Pack 物理格式规范（.arpack tar.gz 结构、manifest.yaml schema、各实体 YAML schema）
- `pack-export`: Pack 导出引擎（三种粒度导出、ID→Key 映射、依赖收集、文件打包）
- `pack-import`: Pack 导入引擎（解包、校验、冲突策略、四阶段写入、Key→ID 映射）
- `pack-api`: Pack HTTP API 和 CLI 命令（export/import/validate 端点）
- `pack-seed-migration`: 内置种子迁移（RawSQL/YAML loader → .arpack 格式，启动加载逻辑重构）

### Modified Capabilities

- `agent-crud`: Agent 创建/更新需支持 Pack 导入写入路径（通过 agent_key 幂等 upsert）
- `team-crud`: Team 创建/更新需支持 Pack 导入写入路径（成员通过 agent_key 引用而非 agent_id）
- `graph-template`: Graph 模板从 Go 硬编码迁移到 Pack 格式，ListGraphTemplates API 需兼容新数据源

## Non-goals

- 不做商城前端 UI（仅预留 manifest 中的版本/作者/依赖字段）
- 不做 Pack 数字签名和加密（后续商城阶段再引入）
- 不做增量 Pack 更新（首版仅支持全量导入）
- 不做 Pack 版本自动升级（首版仅支持 api_version: v1）
- 不做跨 Pack 依赖解析（一个 Pack 自包含，不引用其他 Pack）

## Impact

- **后端 biz 层**：新增 `internal/biz/pack/` 包（Pack 解析/导出/导入引擎），修改 Agent/Team/Graph Usecase 支持按 agent_key 幂等写入
- **后端 data 层**：删除 RawSQL 种子文件，新增 Pack 文件 embed 和解析，修改种子启动编排
- **后端 service 层**：新增 Pack Service（export/import/validate 端点）
- **后端 API 层**：新增 `api/kratos/pack/v1/pack.proto`
- **后端 CLI**：新增 `aranea pack` 子命令
- **后端删除**：`internal/orgimport/` 整包、`internal/scenario/loader/` 中 RawSQL 相关代码、`internal/data/seed_industry_agents_rawsql.go`
- **前端**：暂无前端改动（API 优先，前端后续对接）
- **数据兼容**：现有数据库数据不受影响，新导入走 ORM 路径而非 RawSQL
