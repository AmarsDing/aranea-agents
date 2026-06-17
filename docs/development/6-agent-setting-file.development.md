# Agent 提示文件 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ CRUD + 构建注入 + AI 编辑 RPC + PGO V2 默认文件；🟡 无版本史 / 高亮编辑器
> **需求**：[6 agent-setting-file.md](./6%20agent-setting-file.md) · **设计**：[6-agent-setting-file.design.md](./6-agent-setting-file.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

> **文档边界**：本文件包含模块定位、代码锚点、现状评估、差距与优化、Phase 划分、任务清单（含状态）、验收标准、改动文件清单。用户故事与功能需求见需求文档；架构设计、Proto/API 契约、数据模型见设计文档。

---

## 1. 模块定位

Agent 提示文件（Markdown 分片）：编辑后在运行时经 `BuildSystemPrompt` + `FilesForMode` 注入 LLM Agent。

**代码锚点**：

| 层 | 文件 | 职责 |
|----|------|------|
| Proto | `api/kratos/agent/v1/agent.proto` | `AgentPromptFile` message + PromptFile CRUD RPC + `EstimateTokens` + `EditPromptFileByAI` |
| Biz | `internal/biz/agent_types.go` | `AgentPromptFile` / `FileTokenEstimate` / `FileTokenEstimates` 领域模型 |
| Biz | `internal/biz/agent_usecase.go` | `AgentPromptFileRepo` 接口 + `CreatePromptFile`/`UpdatePromptFile`/`DeletePromptFile`/`EstimateTokens` Usecase 方法 |
| Biz | `internal/biz/agent_settings_helpers.go` | `defaultPromptFiles` / `defaultPromptFilesV2` / `defaultPromptFilesLegacy` / `OptionalPromptFileTemplates` / `FilesForMode` |
| Data | `internal/data/ent/schema/agent_prompt_file.go` | Ent Schema（表 `agent_prompt_files`） |
| Data | `internal/data/agent_repo.go` | `ListAgentPromptFiles` / `ReplaceAgentPromptFiles` / `entPromptToBiz` / `sanitizePromptFileID` |
| Service | `internal/service/agent.go` | `CreateAgentPromptFile` / `UpdateAgentPromptFile` / `DeleteAgentPromptFile` / `EstimateTokens` / `EditPromptFileByAI` + `toProtoFile`/`fromProtoFile` |
| Service | `internal/service/agent_prompt_ai.go` | `PromptFileAIEditor`（LLM 修订） |
| Runtime | `internal/agent/prompt.go` | `BuildSystemPrompt`（组装 + `<internal_config>` 包裹） |
| Runtime | `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent`（构建时读 files） |
| Web | `web/src/features/agents/types.ts` | `AgentPromptFile` TS 类型 |
| Web | `web/src/features/agents/api.ts` | `updateAgent` / `getAgent`（files 整体提交） |
| Web | `web/src/features/agents/wireNormalize.ts` | `normalizePromptFileFromWire` / `promptFileToWire` |
| Web | `web/src/features/agents/useAgentPromptFiles.ts` | 文件编辑 composable |
| Web | `web/src/features/agents/useAgentPromptPreview.ts` | 提示词预览 composable |
| Web | `web/src/features/agents/aiRefine.ts` | AI Refine 逻辑（diff preview） |
| Web | `web/src/features/agents/fieldGuides.ts` | FieldGuide（6 file scopes） |
| Web | `web/src/components/agents/AgentFilesPanel.vue` | 文件 Tab UI |
| Web | `web/src/components/agents/AIRefineButton.vue` | AI Refine 按钮 |
| Web | `web/src/components/agents/MemoryOptionalFilesSection.vue` | 可选文件添加区 |
| Web | `web/src/pages/AgentSettingsPage.vue` | `files` Tab 宿主 |

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| PromptFile CRUD RPC | ✅ | `agent.proto` + `AgentService` 五个方法全部实现 |
| `AgentPromptFileRepo` 接口 | ✅ | `agent_usecase.go`（`Stability:stable`，5 方法） |
| 构建注入 | ✅ | `BuildTRPCLLMAgent` + `BuildSystemPrompt` |
| 模式过滤 | ✅ | `FilesForMode`（`agent_settings_helpers.go`） |
| 新建默认文件 | ✅ | `defaultPromptFiles()` → `defaultPromptFilesV2()` / `defaultPromptFilesLegacy()` |
| `EstimateTokens` RPC | ✅ | `AgentService.EstimateTokens` + `AgentUsecase.EstimateTokens` |
| AI 编辑 RPC | ✅ | `EditPromptFileByAI` + `PromptFileAIEditor.Revise` |
| PGO V2 默认文件 | ✅ | `defaultPromptFilesV2`（5 files: AGENTS_CORE/AGENTS_TASK/IDENTITY/CAPABILITIES/RULE） |
| Legacy 9-file set | ✅ | `defaultPromptFilesLegacy`（backward compat） |
| 可选文件模板 | ✅ | `OptionalPromptFileTemplates`（`USER_CONTEXT.md`） |
| 构建缓存失效 | ✅ | `invalidateAgentBuildCache` 在所有写操作后调用 |
| 版本历史 | ❌ | 无版本表 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 文件列表侧栏 | ✅ | `AgentFilesPanel.vue` splitter |
| Markdown 编辑 | 🟡 | `q-input` textarea，无 CodeMirror/Monaco |
| Token 展示 | ✅ | `refreshFileTokenEstimates` + 侧栏/页脚服务端估算（无数据时回退本地） |
| 脏检测保存 | ✅ | `:disable="!dirty"` |
| AI 编辑弹窗 | ✅ | `applyAiEdit` + `detailStore.editPromptFile`；独立 `aiEditing` loading |
| AI Refine button | ✅ | `AIRefineButton.vue` with diff preview |
| Optional files | ✅ | `MemoryOptionalFilesSection.vue` + `availableOptionalFiles`/`addOptionalFile` |
| FieldGuide | ✅ | `fieldGuides.ts`（6 file scopes） |
| 版本历史 UI | ❌ | — |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 | 状态 |
|----|--------|----------|------|
| FILE-01 | P2 | `EditPromptFileByAI` RPC + LLM 编排 | ✅ |
| FILE-02 | P2 | 前端 AI 编辑：指令 → 修订 → 应用 | ✅ |
| FILE-03 | P2 | 侧栏对接 `EstimateTokens` API | ✅ |
| FILE-04 | P3 | Markdown 语法高亮编辑器 | ❌ |
| FILE-05 | P3 | 文件版本表 + 回滚 | ❌ |
| FILE-06 | P3 | PGO V2 文件系统对齐（SOUL.md/HEARTBEAT.md deprecated） | ✅ |
| FILE-07 | P3 | FieldGuide integration for file scopes | ✅ |

---

## 4. 验收标准

- [x] 文件 CRUD 与保存后构建注入生效
- [x] 新建 Agent 可有默认 prompt 文件集（PGO V2: 5 files + Legacy 9-file compat）
- [x] AI 编辑可生成真实修订内容（`agent.prompt.ai_edit` FlowLog）
- [x] AI Refine button 带 diff preview
- [x] Optional files 可添加（`MemoryOptionalFilesSection.vue`）
- [x] FieldGuide 6 file scopes 可用
- [x] Token 估算与后端 RPC 一致（`EstimateTokens` API 对接）
- [ ] （可选）版本历史可查看/回滚
- [ ] （可选）Markdown 语法高亮编辑器

---

## 5. 依赖

| 模块 | 说明 |
|------|------|
| 5 设置 | `system_prompt_mode` 影响过滤 |
| 7 进化 | SOUL.md 自动演化（未通）；PGO V2 已 deprecated SOUL.md |
| 8 标题 | `GetAgentPromptPreview` 预览 |
| 9 Provider | AI 编辑需 LLM（`PromptFileAIEditor` 依赖 `LlmProviderModelUsecase` + `provider.RoundTrip`） |
| Learning Loop | Learning 观察可能触发文件修改建议 |
