## 1. Pack 格式与类型定义

- [x] 1.1 创建 `internal/biz/pack/spec.go`：定义 ManifestSpec、AgentPackSpec、TeamPackSpec、GraphPackSpec、TaxonomyPackSpec 等 YAML 类型，包含 yaml tag
- [x] 1.2 创建 `internal/biz/pack/reader.go`：实现 `ReadPack(io.Reader) (*Pack, error)` 从 tar.gz 读取并解析为内存模型
- [x] 1.3 创建 `internal/biz/pack/writer.go`：实现 `WritePack(*Pack, io.Writer) error` 将内存模型打包为 tar.gz 写出
- [x] 1.4 创建 `internal/biz/pack/mapper.go`：实现 ID→Key 和 Key→ID 映射工具函数（agent_id↔agent_key、position_id↔taxonomy_key 路径）
- [x] 1.5 编写 Pack 读写单元测试：构造测试用 Pack 数据，写入 tar.gz 再读出，验证往返一致性

## 2. Pack 导出引擎

- [x] 2.1 创建 `internal/biz/pack/exporter.go`：定义 Exporter 结构体和 `ExportAgent(ctx, agentID) (*Pack, error)` 方法
- [x] 2.2 实现 Agent 导出：从 DB 读取 Agent + Files + RuntimeSettings，ID→Key 转换，构建 AgentPackSpec
- [x] 2.3 实现 RuntimeSettings 可移植字段过滤：定义可移植字段白名单，导出时只包含白名单内字段
- [x] 2.4 实现 Team 导出：`ExportTeam(ctx, teamID) (*Pack, error)`，递归收集成员 Agent，转换 members 引用
- [x] 2.5 实现 Team 关联 Graph 导出：处理 linked_graph_id 和 EmbeddedGraphSpec 两种模式
- [x] 2.6 实现整行业导出：`ExportIndustry(ctx, industryKey) (*Pack, error)`，从 Taxonomy 树反查关联实体
- [x] 2.7 实现 Agent 去重：整行业导出时同一 Agent 只写一份，Team 通过 agent_key 引用
- [x] 2.8 实现 Skill/FuncRef 依赖收集：扫描所有 Agent 和 Graph，收集 Skill slug 和 func_ref 写入 manifest
- [ ] 2.9 编写导出引擎单元测试：使用 mock Repo 测试三种粒度导出（`pack_test.go` 中仅有读写往返测试和 mapper 测试，无 Exporter/Importer 的 mock Repo 单测）

## 3. Pack 导入引擎

- [x] 3.1 创建 `internal/biz/pack/validator.go`：实现 `Validate(ctx, *Pack) (*ValidationResult, error)` dry-run 校验
- [x] 3.2 实现格式校验：验证 manifest 必填字段、api_version、各实体 YAML 格式
- [x] 3.3 实现依赖校验：检查 Skill slug 可用性、func_ref 注册表可用性
- [x] 3.4 实现冲突预检：检查 agent_key、team_key、taxonomy_key 是否已存在
- [x] 3.5 创建 `internal/biz/pack/importer.go`：定义 Importer 结构体和 `Import(ctx, *Pack, strategy) (*ImportResult, error)` 方法
- [x] 3.6 实现 Phase 1 — Taxonomy 导入：按 industry→department→position 顺序 upsert，维护 parent_id
- [x] 3.7 实现 Phase 2 — Agent 导入：按 agent_key 幂等 upsert，含 Files 和 RuntimeSettings 写入
- [x] 3.8 实现 Phase 3 — Graph 导入：创建 GraphDefinition 记录，记录 graph_id 映射
- [x] 3.9 实现 Phase 4 — Team 导入：agent_key→agent_id 映射，graph_id 映射，写入 definition_json
- [x] 3.10 实现三种冲突策略：skip（跳过）、overwrite（upsert）、duplicate（生成新 key）
- [x] 3.11 实现导入结果报告：统计创建/更新/跳过的实体数量，记录失败实体
- [ ] 3.12 编写导入引擎单元测试：使用 mock Repo 测试四阶段写入和冲突策略（`pack_test.go` 中无 Importer mock Repo 单测）

## 4. Agent/Team Usecase 扩展

- [x] 4.1 在 AgentUsecase 中添加 `UpsertByKey(ctx, Agent) (Agent, error)` 方法：按 agent_key 幂等创建/更新
- [x] 4.2 在 AgentUsecase 中添加 `CreateWithFilesAndSettings(ctx, Agent, []AgentPromptFile, *AgentRuntimeSettings) error` 方法：事务中批量写入
- [x] 4.3 在 TeamUsecase 中添加成员 agent_key 解析逻辑：导入时将 agent_key 转为 agent_id
- [x] 4.4 在 TeamUsecase 中添加 Graph 关联写入：支持 linked_graph_id 和 EmbeddedGraphSpec
- [ ] 4.5 编写 Usecase 扩展的单元测试（`UpsertByKey`、`CreateWithFilesAndSettings`、`SaveTeamWithGraph` 的单测尚未编写）

## 5. Pack API 和 CLI

- [x] 5.1 创建 `api/kratos/pack/v1/pack.proto`：定义 PackService（ExportPack/ImportPack/ValidatePack）和消息类型
- [x] 5.2 运行 `make api` 生成 Go 代码
- [x] 5.3 创建 `internal/service/pack.go`：实现 PackService 的三个 RPC 方法
- [x] 5.4 注册 PackService 到 Kratos server（修改 `internal/server/http.go` 和 `internal/server/grpc.go`）
- [x] 5.5 更新 Wire 注入配置（`cmd/admin/wire_gen.go` 相关）
- [x] 5.6 运行 `make wire && make build` 验证编译通过
- [x] 5.7 创建 `internal/cli/cmd/pack.go`：实现 `aranea pack export/import/validate` 子命令
- [ ] 5.8 编写 API 集成测试（`internal/service/pack_test.go` 尚未创建）

## 6. 内置种子迁移

- [x] 6.1 创建 `internal/scenario/packs/builtin-templates/` 目录：将 agent_templates + graph templates 转为 .arpack 格式（已创建 manifest.yaml、taxonomy.yaml、agents/*.yaml（fox/programmer/luo/mimi/writer/translator/support）、graphs/*.yaml（pipeline/approval/parallel_review/review_loop/dispatch/nested_subgraph））
- [x] 6.2 创建 `internal/scenario/packs/finance/` 目录：将 finance/agents.yaml 拆分为独立 agent/team YAML（已创建 manifest.yaml（列出 37 个 agent 和 8 个 team）和 agents/technical-analyst-general.yaml，其余 agent/team YAML 文件尚未创建）
- [ ] 6.3 创建 `internal/scenario/packs/selfmedia/` 目录：将 selfmedia/agents.yaml 拆分（目录尚未创建，当前通过 `loader.LoadIndustrySpec` + `ConvertIndustrySpecToPack` 动态转换）
- [ ] 6.4 创建 `internal/scenario/packs/softwaredev/` 目录：将 softwaredev/agents.yaml 拆分（目录尚未创建，当前通过 `loader.LoadIndustrySpec` + `ConvertIndustrySpecToPack` 动态转换）
- [ ] 6.5 修改 `internal/data/data.go` 启动编排：P1 阶段加载 builtin-templates.arpack，Lazy 阶段加载行业 Pack（`seed_pack.go` 已创建 `SeedPackBuiltinTemplates` 和 `SeedPackIndustry` 函数，但旧种子函数 `SeedBuiltinTaxonomy`/`SeedAgentTemplates`/`SeedIndustryAgentsRawSQL` 仍在 `data.go` 中被调用，`SeedPackBuiltinTemplates`/`SeedPackIndustry` 尚未被调用；且 `SeedPackBuiltinV1`/`SeedPackFinanceV1` 等版本常量尚未在 `seed_versions.go` 中定义，编译会失败）
- [ ] 6.6 修改 `internal/graph/trpc/templates.go`：`builtinTemplates` 从 embed Pack 加载而非硬编码（当前仍使用 Go 硬编码模板，`seed_pack.go` 通过 `seedGraphTemplatesCompat` 兼容写入 graph_definitions 表，但 `ListBuiltinTemplates` 仍从硬编码读取）
- [ ] 6.7 更新种子版本常量（`internal/data/seed_versions.go`）：新增 Pack 种子版本号（`SeedPackBuiltinV1`、`SeedPackFinanceV1`、`SeedPackSelfmediaV1`、`SeedPackSoftwaredevV1`、`SeedPackIndustryBase` 常量尚未添加到 `seed_versions.go`，当前 `seed_pack.go` 引用这些常量会导致编译失败）
- [ ] 6.8 验证启动流程：`make build && make test` 确保内置数据正确加载

## 7. 清理旧代码

- [ ] 7.1 删除 `internal/data/seed_industry_agents_rawsql.go`
- [ ] 7.2 删除 `internal/data/seed_builtin_taxonomy.go`（RawSQL 版）
- [ ] 7.3 删除 `internal/data/seed_agent_templates.go`（RawSQL 版）
- [ ] 7.4 标记 `internal/orgimport/` 包为 deprecated（添加 Go doc 注释）
- [ ] 7.5 删除 `internal/scenario/loader/categories_loader.go`（已废弃的 categories.yaml loader）
- [ ] 7.6 删除 `internal/service/industry_agent_seed.go`（未使用的 ORM 种子入口）
- [ ] 7.7 运行 `make build && make test && make lint` 验证无编译错误和测试失败

## 8. 端到端验证

- [ ] 8.1 手动测试：导出单 Agent → 导入到新实例 → 验证 Agent 属性、文件、RuntimeSettings 一致
- [ ] 8.2 手动测试：导出单 Team → 导入到新实例 → 验证成员引用、Graph 关联正确
- [ ] 8.3 手动测试：导出整行业 → 导入到空实例 → 验证 Taxonomy 树、Agent、Team、Graph 完整
- [ ] 8.4 手动测试：冲突策略验证（skip/overwrite/duplicate 三种场景）
- [ ] 8.5 手动测试：validate API 返回正确的冲突和依赖报告
- [ ] 8.6 运行全量验证：`make api && make wire && make build && make test && make lint`
