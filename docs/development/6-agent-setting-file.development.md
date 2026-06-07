# Agent 提示文件 — 开发计划

> **版本**：2026-06-06 | **状态**：✅ CRUD + 构建注入 + AI 编辑 RPC + PGO V2 默认文件；🟡 无版本史 / 高亮编辑器
> **需求**：[6 agent-setting-file.md](./6%20agent-setting-file.md) · **设计**：[6 agent-setting-file.design.md](./6%20agent-setting-file.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

Agent 提示文件（Markdown 分片）：编辑后在运行时经 `BuildSystemPrompt` + `FilesForMode` 注入 LLM Agent。

**代码锚点**：
- `api/kratos/agent/v1/agent.proto` — PromptFile CRUD + `EstimateTokens`
- `internal/biz/agent_usecase.go` — Create/Update/Delete + `syncConfigJSON`
- `internal/biz/agent_defaults.go` — `defaultPromptFiles` / `FilesForMode`
- `internal/agent/trpc_build.go` — 构建时读 files
- `web/src/features/agents/useAgentPromptFiles.ts` — file editor composable
- `web/src/features/agents/useAgentPromptPreview.ts` — prompt preview composable
- `web/src/features/agents/fieldGuides.ts` — FieldGuide（6 file scopes）
- `web/src/components/agents/AIRefineButton.vue` — AI Refine button
- `web/src/components/agents/MemoryOptionalFilesSection.vue` — optional file adding
- `web/src/components/agents/AgentFilesPanel.vue` — 文件 Tab UI
- `web/src/pages/AgentSettingsPage.vue` — `files` Tab 宿主

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| PromptFile CRUD | ✅ | proto + `AgentUsecase` |
| 构建注入 | ✅ | `BuildTRPCLLMAgent` + `BuildSystemPrompt` |
| 模式过滤 | ✅ | `FilesForMode` |
| 新建默认文件 | ✅ | `hydrate` / `Create` 路径 `defaultPromptFiles()` |
| `EstimateTokens` RPC | ✅ | `AgentService.EstimateTokens` |
| AI 编辑 RPC | ✅ | `EditPromptFileByAI` + `PromptFileAIEditor`（service 层 LLM） |
| `EditPromptFileByAI` | ✅ | `PromptFileAIEditor.Revise` |
| PGO V2 默认文件 | ✅ | `defaultPromptFilesV2`（6 files: AGENTS_CORE/AGENTS_TASK/IDENTITY/RULE/CAPABILITIES/USER_CONTEXT） |
| Legacy 9-file set | ✅ | `defaultPromptFilesLegacy`（backward compat） |
| 版本历史 | ❌ | 无版本表 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 文件列表侧栏 | ✅ | `AgentFilesPanel` splitter |
| Markdown 编辑 | 🟡 | `q-input` textarea，无 CodeMirror/Monaco |
| Token 展示 | ✅ | `estimateAgentTokens` + 侧栏/页脚服务端估算（无数据时回退本地） |
| 脏检测保存 | ✅ | `:disable="!dirty"` |
| AI 编辑弹窗 | ✅ | `editPromptFileByAI` + `applyAiEdit`；独立 `aiEditing` loading |
| AI Refine button | ✅ | `AIRefineButton.vue` with diff preview |
| Optional files | ✅ | `MemoryOptionalFilesSection.vue` |
| 版本历史 UI | ❌ | — |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 |
|----|--------|----------|
| FILE-01 | P2 | `EditPromptFileByAI` RPC + LLM 编排 | ✅ |
| FILE-02 | P2 | 前端 AI 编辑：指令 → 修订 → 应用 | ✅ |
| FILE-03 | P2 | 侧栏对接 `EstimateTokens` API | ✅ |
| FILE-04 | P3 | Markdown 语法高亮编辑器 |
| FILE-05 | P3 | 文件版本表 + 回滚 |
| FILE-06 | P3 | PGO V2 文件系统对齐（SOUL.md/HEARTBEAT.md deprecated） |
| FILE-07 | P3 | FieldGuide integration for file scopes | ✅ |

---

## 4. 验收标准

- [x] 文件 CRUD 与保存后构建注入生效
- [x] 新建 Agent 可有默认 prompt 文件集（PGO V2: 6 files + Legacy 9-file compat）
- [x] AI 编辑可生成真实修订内容（`agent.prompt.ai_edit` FlowLog）
- [x] AI Refine button 带 diff preview
- [x] Optional files 可添加（`MemoryOptionalFilesSection.vue`）
- [x] FieldGuide 6 file scopes 可用
- [ ] Token 估算与后端 RPC 一致
- [ ] （可选）版本历史可查看/回滚
- [ ] （可选）Markdown 语法高亮编辑器

---

## 5. 依赖

| 模块 | 说明 |
|------|------|
| 5 设置 | `system_prompt_mode` 影响过滤 |
| 7 进化 | SOUL.md 自动演化（未通）；PGO V2 已 deprecated SOUL.md |
| 8 标题 | `GetAgentPromptPreview` 预览 |
| 9 Provider | AI 编辑需 LLM |
| Learning Loop | Learning 观察可能触发文件修改建议 |
