<template>
  <q-page :class="['agent-settings', { 'agent-settings--files-fill': tab === 'files' }]">
    <q-card flat bordered :class="['settings-shell', { 'settings-shell--fill': tab === 'files' }]">
      <agent-settings-header
        :agent="form"
        :self-evolve="config.self_evolve"
        :favorite="form.is_favorite"
        :saving="saving"
        @back="router.back()"
        @change-avatar="avatarPickerOpen = true"
        @open-prompt="promptDialog = true"
        @toggle-favorite="toggleFavorite"
        @save="saveAgent"
      />
      <q-separator />
      <q-tabs v-model="tab" dense align="left" class="agent-settings-tabs" :breakpoint="0">
        <q-tab name="agent" label="Agent" />
        <q-tab name="memory" label="记忆" />
        <q-tab name="files" label="文件" />
        <q-tab name="permissions" label="权限" />
        <q-tab name="skills" label="Skill" />
        <q-tab name="evolution" label="进化" />
        <q-tab name="hooks" label="钩子" />
        <q-tab name="instances" label="用户实例" />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="settings-panels">
        <q-tab-panel name="agent">
          <div class="settings-grid">
            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">系统提示模式</div>
                  <div class="text-caption text-grey-7">控制运行时注入的提示块体量与人格强度。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <div v-for="mode in promptModes" :key="mode.value" class="col-12 col-md-3">
                  <q-card
                    flat
                    bordered
                    class="prompt-mode-card cursor-pointer"
                    :class="{ 'is-active': form.system_prompt_mode === mode.value }"
                    @click="form.system_prompt_mode = mode.value"
                  >
                    <q-card-section>
                      <div class="text-subtitle2 text-weight-bold">{{ mode.label }}</div>
                      <div class="text-caption text-grey-7 q-mt-xs">{{ mode.caption }}</div>
                      <q-chip dense square class="prompt-mode-card__token q-mt-sm">{{ mode.tokens }}</q-chip>
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">Agent 个性</div>
                  <div class="text-caption text-grey-7">身份、状态、分类与对外描述。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model="form.display_name" class="col-12 col-md-6" dense outlined label="显示名称" />
                <q-input v-model="form.agent_key" class="col-12 col-md-6" dense outlined readonly label="Agent 标识">
                  <template #append><q-btn flat round dense icon="content_copy" @click="copyKey" /></template>
                </q-input>
                <q-select v-model="form.status" class="col-12 col-md-6" dense outlined emit-value map-options label="状态" :options="statusOptions" />
                <q-toggle v-model="form.is_default" class="col-12 col-md-6" color="primary" label="默认 Agent" />
                <q-input v-model="form.agent_description" class="col-12" outlined autogrow type="textarea" label="专业摘要 / 能力描述" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">模型与预算</div>
                  <div class="text-caption text-grey-7">选择数据库已录入的模型；上下文大小在 Provider 管理中维护。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-select
                  :model-value="selectedProviderModelID"
                  class="col-12 col-md-6"
                  dense
                  outlined
                  emit-value
                  map-options
                  use-input
                  input-debounce="0"
                  label="模型"
                  hint="仅可选择 Provider 管理中已录入且启用的模型。"
                  :options="filteredProviderModelOptions"
                  :loading="loadingProviderModels"
                  :disable="loadingProviderModels"
                  @filter="filterProviderModels"
                  @update:model-value="selectProviderModel"
                >
                  <template #option="scope">
                    <q-item v-bind="scope.itemProps">
                      <q-item-section>
                        <q-item-label>{{ scope.opt.label }}</q-item-label>
                        <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
                      </q-item-section>
                    </q-item>
                  </template>
                </q-select>
                <q-input v-model.number="budgetUSD" class="col-12 col-md-6" dense outlined type="number" prefix="$" label="月度预算" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">能力</div>
                  <div class="text-caption text-grey-7">子 Agent 与工具策略。冲突工具会在保存前提示。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">子 Agent</div>
                        <div class="text-caption text-grey-7">控制生成限制和归档策略。</div>
                      </div>
                      <q-toggle v-model="config.subagents.enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section v-if="config.subagents.enabled" class="row q-col-gutter-sm">
                      <q-input v-model.number="config.subagents.max_concurrency" class="col-6" dense outlined type="number" label="最大并发数" />
                      <q-input v-model.number="config.subagents.max_generation_depth" class="col-6" dense outlined type="number" label="最大生成深度" />
                      <q-input v-model.number="config.subagents.max_children_per_agent" class="col-6" dense outlined type="number" label="每 Agent 最大子数" />
                      <q-input v-model.number="config.subagents.archive_after_minutes" class="col-6" dense outlined type="number" label="归档时间 (分钟)" />
                      <q-input v-model.number="config.subagents.max_retries" class="col-6" dense outlined type="number" label="最大重试次数" />
                      <q-input v-model="config.subagents.model_override" class="col-6" dense outlined label="模型覆盖" placeholder="继承自 Agent" />
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">工具策略</div>
                        <div class="text-caption text-grey-7">控制可调用工具、黑名单与并行白名单。</div>
                      </div>
                      <q-toggle v-model="config.tools.enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section v-if="config.tools.enabled" class="q-gutter-sm">
                      <q-select
                        v-model="config.tools.profile"
                        dense
                        outlined
                        emit-value
                        map-options
                        label="工具配置文件"
                        hint="按意图选择 Agent 的工具能力面：chat_only 仅对话；read_only 只读 + 时间；coding 代码 + 网页；research 网页 + 检索；full 全开放（高权限）。"
                        :options="toolProfileOptions"
                      />
                      <q-input v-model="config.tools.tool_call_prefix" dense outlined label="工具调用前缀" hint="如 proxy_，解析前会从工具名中剥离。" />
                      <q-select
                        v-model="config.tools.allow"
                        dense
                        outlined
                        multiple
                        use-chips
                        emit-value
                        map-options
                        label="允许"
                        :options="toolSelectOptions"
                        :loading="loadingCatalogTools"
                        hint="选项来自 Tools 目录中的平台工具；亦可保留已保存的自定义 key。"
                      />
                      <q-select
                        v-model="config.tools.deny"
                        dense
                        outlined
                        multiple
                        use-chips
                        emit-value
                        map-options
                        label="拒绝"
                        :options="toolSelectOptions"
                        :loading="loadingCatalogTools"
                      />
                      <q-select
                        v-model="config.tools.concurrent_allow"
                        dense
                        outlined
                        multiple
                        use-chips
                        emit-value
                        map-options
                        label="同时允许"
                        :options="toolSelectOptions"
                        :loading="loadingCatalogTools"
                      />
                      <q-banner v-if="toolConflicts.length" rounded class="settings-warning-banner">
                        以下工具同时出现在允许与拒绝中，运行时按拒绝优先：{{ toolConflicts.join(", ") }}
                      </q-banner>
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">工具重试</div>
                        <div class="text-caption text-grey-7">工具调用失败时自动重试，指数退避 + 随机抖动。</div>
                      </div>
                      <q-toggle v-model="config.tools.retry.enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section v-if="config.tools.retry.enabled" class="row q-col-gutter-sm">
                      <q-input v-model.number="config.tools.retry.max_attempts" class="col-6" dense outlined type="number" label="最大重试次数" hint="含首次调用" />
                      <q-input v-model.number="config.tools.retry.initial_interval_ms" class="col-6" dense outlined type="number" label="初始间隔 (ms)" />
                      <q-input v-model.number="config.tools.retry.backoff_factor" class="col-6" dense outlined type="number" step="0.1" label="退避因子" />
                      <q-input v-model.number="config.tools.retry.max_interval_ms" class="col-6" dense outlined type="number" label="最大间隔 (ms)" />
                      <q-toggle v-model="config.tools.retry.jitter" class="col-6" color="primary" label="随机抖动" />
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">并行与流式</div>
                        <div class="text-caption text-grey-7">并行执行多个工具调用；流式工具支持需工具实现 StreamableCall 接口。</div>
                      </div>
                    </q-card-section>
                    <q-separator />
                    <q-card-section class="row q-col-gutter-sm">
                      <q-toggle v-model="config.tools.parallel_enabled" class="col-12" color="primary" label="并行工具调用" hint="模型发出多个工具调用时并行执行" />
                      <q-toggle v-model="config.tools.streaming_enabled" class="col-12" color="primary" label="流式工具" hint="启用支持 StreamableCall 的工具流式输出" />
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">意图 Pass</div>
                        <div class="text-caption text-grey-7">
                          每轮用户消息进入主模型前先做一次轻量意图梳理（多一次 LLM 调用）。部署侧可用环境变量
                          <code>ARANEA_INTENT_PASS=0</code>
                          等对全部 Agent 做全局覆写。
                        </div>
                      </div>
                      <q-toggle v-model="config.intent_pass.enabled" color="primary" />
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">记忆与心跳</div>
                  <div class="text-caption text-grey-7">语义检索、Dreaming 与 HEARTBEAT.MD 注入。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-toggle v-model="config.memory.enabled" class="col-12 col-md-3" color="primary" label="记忆启用" />
                <q-input v-model.number="config.memory.max_chunk_length" class="col-12 col-md-3" dense outlined type="number" label="最大块长度" />
                <q-input v-model.number="config.memory.max_results" class="col-12 col-md-3" dense outlined type="number" label="最大结果数" />
                <q-input v-model.number="config.memory.min_score" class="col-12 col-md-3" dense outlined type="number" step="0.01" label="最低分数" />
                <q-toggle v-model="config.heartbeat.enabled" class="col-12 col-md-3" color="negative" label="心跳启用" />
                <q-input v-model.number="config.heartbeat.interval_minutes" class="col-12 col-md-3" dense outlined type="number" suffix="min" label="间隔" />
                <q-input v-model="heartbeatFile.body" class="col-12 col-md-6" dense outlined autogrow type="textarea" label="检查清单 (HEARTBEAT.MD)" />
              </div>
            </section>
          </div>
        </q-tab-panel>

        <q-tab-panel name="memory">
          <div class="settings-grid">
            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">记忆总览</div>
                  <div class="text-caption text-grey-7">统一控制上下文、工作记忆、会话事件、语义知识和进化记忆。</div>
                </div>
                <q-toggle v-model="config.memory.enabled" color="primary" label="记忆总开关" />
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model.number="config.memory.max_results" class="col-12 col-md-4" dense outlined type="number" label="兼容最大结果数" />
                <q-input v-model.number="config.memory.min_score" class="col-12 col-md-4" dense outlined type="number" step="0.01" label="兼容最低分数" />
                <q-input v-model.number="config.memory.max_chunk_length" class="col-12 col-md-4" dense outlined type="number" label="兼容最大块长度" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">L0 上下文窗口</div>
                  <div class="text-caption text-grey-7">控制下一次 prompt 如何装配、摘要和记录快照。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model.number="config.memoryL0.recent_window_turns" class="col-12 col-md-3" dense outlined type="number" label="最近窗口轮数" />
                <q-input v-model.number="config.memoryL0.recent_window_tokens" class="col-12 col-md-3" dense outlined type="number" label="最近窗口 Tokens" hint="0 = 自动" />
                <q-input v-model.number="config.memoryL0.summary_threshold" class="col-12 col-md-3" dense outlined type="number" step="0.05" label="摘要触发阈值" />
                <q-input v-model.number="config.memoryL0.summary_keep_turns" class="col-12 col-md-3" dense outlined type="number" label="摘要保留轮数" />
                <q-select v-model="config.memoryL0.truncate_strategy" class="col-12 col-md-3" dense outlined emit-value map-options label="裁剪策略" :options="truncateStrategyOptions" />
                <q-select v-model="config.memoryL0.snapshot_mode" class="col-12 col-md-3" dense outlined emit-value map-options label="快照模式" :options="snapshotModeOptions" />
                <q-input v-model.number="config.memoryL0.l3_max_chunks" class="col-12 col-md-3" dense outlined type="number" label="L3 最大片段" />
                <q-input v-model.number="config.memoryL0.l4_max_paths" class="col-12 col-md-3" dense outlined type="number" label="L4 最大路径" />
                <q-toggle v-model="config.memoryL0.inject_l1" class="col-12 col-md-3" color="primary" label="注入 L1" />
                <q-toggle v-model="config.memoryL0.inject_l3" class="col-12 col-md-3" color="primary" label="注入 L3" />
                <q-toggle v-model="config.memoryL0.inject_l4" class="col-12 col-md-3" color="primary" label="注入 L4" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">L1 工作记忆</div>
                  <div class="text-caption text-grey-7">当前任务的结构化状态、版本和预算。</div>
                </div>
                <q-toggle v-model="config.memoryL1.enabled" color="primary" label="启用 L1" />
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model.number="config.memoryL1.budget_tokens" class="col-12 col-md-3" dense outlined type="number" label="任务预算 Tokens" />
                <q-input v-model.number="config.memoryL1.field_max_tokens" class="col-12 col-md-3" dense outlined type="number" label="单字段上限" />
                <q-input v-model.number="config.memoryL1.history_keep_revisions" class="col-12 col-md-3" dense outlined type="number" label="版本保留数" />
                <q-input v-model.number="config.memoryL1.archive_on_idle_minutes" class="col-12 col-md-3" dense outlined type="number" label="闲置归档分钟" />
                <q-input v-model="config.memoryL1.default_schema_id" class="col-12" dense outlined label="默认 Schema ID" placeholder="schema_xxx" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">L2 会话事件</div>
                  <div class="text-caption text-grey-7">控制 episode、索引、回忆注入和保留周期。</div>
                </div>
                <q-toggle v-model="config.memoryL2.episode_enabled" color="primary" label="启用 Episode" />
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model.number="config.memoryL2.episode_min_importance" class="col-12 col-md-3" dense outlined type="number" step="0.05" label="最低重要性" />
                <q-toggle v-model="config.memoryL2.index_enabled" class="col-12 col-md-3" color="primary" label="启用索引" />
                <q-toggle v-model="config.memoryL2.recall_enabled" class="col-12 col-md-3" color="primary" label="允许 L2 Recall" />
                <q-input v-model.number="config.memoryL2.recall_max" class="col-12 col-md-3" dense outlined type="number" label="Recall 最大数" />
                <q-input v-model="config.memoryL2.index_embedding_model" class="col-12 col-md-4" dense outlined label="Embedding 模型" />
                <q-input v-model.number="config.memoryL2.retention_days" class="col-12 col-md-4" dense outlined type="number" label="保留天数" />
                <q-input v-model.number="config.memoryL2.archive_after_days" class="col-12 col-md-4" dense outlined type="number" label="归档天数" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">L3 语义知识</div>
                  <div class="text-caption text-grey-7">跨会话事实、偏好、规则和冲突治理。</div>
                </div>
                <q-toggle v-model="config.memoryL3.enabled" color="primary" label="启用 L3" />
              </div>
              <div class="row q-col-gutter-md">
                <q-input v-model.number="config.memoryL3.recall_top_k" class="col-12 col-md-3" dense outlined type="number" label="Recall TopK" />
                <q-input v-model.number="config.memoryL3.recall_min_score" class="col-12 col-md-3" dense outlined type="number" step="0.05" label="最小分数" />
                <q-select v-model="config.memoryL3.recall_scopes" class="col-12 col-md-6" dense outlined multiple use-chips emit-value map-options label="检索作用域" :options="memoryScopeOptions" />
                <q-input v-model="config.memoryL3.embedding_model" class="col-12 col-md-4" dense outlined label="Embedding 模型" />
                <q-input v-model.number="config.memoryL3.decay_interval_hours" class="col-12 col-md-4" dense outlined type="number" label="衰减间隔小时" />
                <q-input v-model.number="config.memoryL3.archive_threshold" class="col-12 col-md-4" dense outlined type="number" step="0.05" label="归档阈值" />
                <q-input v-model.number="config.memoryL3.max_per_recall_chars" class="col-12 col-md-4" dense outlined type="number" label="Recall 最大字符" />
              </div>
            </section>

            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">L4 图谱与进化</div>
                  <div class="text-caption text-grey-7">长期实体关系、Agent 身份、策略和自我修正提议。</div>
                </div>
                <q-toggle v-model="config.memoryL4.enabled" color="primary" label="启用 L4" />
              </div>
              <div class="row q-col-gutter-md">
                <q-toggle v-model="config.memoryL4.graph_inject_neighbors" class="col-12 col-md-3" color="primary" label="注入图邻居" />
                <q-input v-model.number="config.memoryL4.graph_max_neighbors" class="col-12 col-md-3" dense outlined type="number" label="邻居数" />
                <q-input v-model.number="config.memoryL4.graph_max_hops" class="col-12 col-md-3" dense outlined type="number" label="邻居跳数" />
                <q-toggle v-model="config.memoryL4.identity_inject" class="col-12 col-md-3" color="primary" label="注入身份" />
                <q-toggle v-model="config.memoryL4.strategy_inject" class="col-12 col-md-3" color="primary" label="注入策略" />
                <q-toggle v-model="config.evolutionSettings.enabled" class="col-12 col-md-3" color="primary" label="启用自我演化" />
                <q-toggle v-model="config.evolutionSettings.auto_apply" class="col-12 col-md-3" color="warning" label="低风险自动应用" />
                <q-input v-model.number="config.evolutionSettings.min_episodes" class="col-12 col-md-3" dense outlined type="number" label="触发 Episode 数" />
                <q-input v-model.number="config.evolutionSettings.min_negative_feedback" class="col-12 col-md-3" dense outlined type="number" label="触发负反馈数" />
                <q-input v-model.number="config.evolutionSettings.throttle_hours" class="col-12 col-md-3" dense outlined type="number" label="节流小时" />
                <q-input v-model.number="config.evolutionSettings.proposal_ttl_days" class="col-12 col-md-3" dense outlined type="number" label="提议过期天数" />
                <q-input v-model.number="config.evolutionSettings.persona_max_chars" class="col-12 col-md-3" dense outlined type="number" label="Persona 最大字符" />
                <q-input v-model.number="config.evolutionSettings.system_prompt_max_appends" class="col-12 col-md-3" dense outlined type="number" label="Prompt 追加段上限" />
              </div>
            </section>
          </div>
        </q-tab-panel>

        <q-tab-panel name="files" class="settings-tab-panel-fill">
          <agent-files-panel
            v-model:active-file="activeFile"
            v-model:splitter="fileSplitter"
            :files="files"
            :dirty="fileDirty"
            @update-file-body="updateFileBody"
            @reload="reloadActiveFile"
            @ai-edit="aiEditOpen = true"
            @save="saveAgent"
          />
        </q-tab-panel>

        <q-tab-panel name="permissions">
          <q-banner rounded class="settings-placeholder-banner">权限与用户可见范围将按独立 PRD 接入。当前保留入口。</q-banner>
        </q-tab-panel>

        <q-tab-panel name="skills">
          <div class="settings-grid">
            <section class="settings-section">
              <div class="section-heading">
                <div>
                  <div class="text-subtitle1 text-weight-bold">平台 Skill 挂载策略</div>
                  <div class="text-caption text-grey-7">
                    控制本会话中 ADK 可见的已发布 Skill：Agent 白名单/黑名单、必选标签，以及是否根据用户话术做意图收窄（详见仓库文档「20 skill struct design」十三′）。
                  </div>
                </div>
                <div class="row q-gutter-sm">
                  <q-btn outline rounded dense color="primary" label="刷新 Skill 列表" :loading="loadingSkillSlugs" @click="loadSkillSlugOptions" />
                  <q-btn outline rounded dense color="primary" label="恢复默认" @click="resetSkillRuntimeDefaults" />
                </div>
              </div>
              <q-banner rounded class="q-mb-md settings-info-banner">
                留空「允许的 slug」表示不按 slug 白名单过滤；「意图收窄」开启后，仅对与话术匹配的 taxonomy / 关键词相关的 Skill 并集挂载（可减少无关工具）。运行时只会挂载<strong>已发布且已启用</strong>的平台 Skill；草稿仅便于在此勾选 slug，需先到 Skill 管理发布并启用。
              </q-banner>
              <div class="row q-col-gutter-md">
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section class="row items-center justify-between">
                      <div>
                        <div class="text-subtitle2">意图收窄（层 B）</div>
                        <div class="text-caption text-grey-7">根据用户消息关键词匹配内置意图路径，缩小 Skill 候选集。</div>
                      </div>
                      <q-toggle v-model="config.skillRuntime.intent_routing_enabled" color="primary" />
                    </q-card-section>
                    <q-separator />
                    <q-card-section class="row q-col-gutter-sm">
                      <q-input
                        v-model.number="config.skillRuntime.intent_max_paths"
                        class="col-6"
                        dense
                        outlined
                        type="number"
                        label="最多意图路径数"
                        :min="1"
                        :max="32"
                      />
                      <q-input
                        v-model.number="config.skillRuntime.max_skills_in_toolset"
                        class="col-6"
                        dense
                        outlined
                        type="number"
                        label="工具集内 Skill 上限"
                        :min="1"
                        :max="256"
                      />
                    </q-card-section>
                  </q-card>
                </div>
                <div class="col-12 col-lg-6">
                  <q-card flat bordered class="capability-card">
                    <q-card-section>
                      <div class="text-subtitle2 q-mb-xs">slug 与标签（层 A）</div>
                      <div class="text-caption text-grey-7 q-mb-sm">
                        标签 token 写入 Skill 元数据的标签名（如 file_type:xlsx），多项为「同时满足」。允许与拒绝同一 slug 互斥：在一侧添加会从另一侧去掉同名项；若历史配置两侧重叠，载入/保存时会按运行时规则以<strong>拒绝优先</strong>规整。
                      </div>
                      <q-select
                        v-model="config.skillRuntime.allowed_slugs"
                        class="q-mb-sm"
                        dense
                        outlined
                        multiple
                        use-chips
                        use-input
                        new-value-mode="add-unique"
                        input-debounce="0"
                        :options="skillSlugOptions"
                        option-label="label"
                        option-value="value"
                        emit-value
                        map-options
                        label="允许的 Skill slug（skill_key）"
                        hint="从平台 Skill 勾选或手动输入；留空 = 不启用 slug 白名单。与「拒绝」互斥：此处勾选会从拒绝列表移除同名 slug。"
                      />
                      <q-select
                        v-model="config.skillRuntime.denied_slugs"
                        class="q-mb-sm"
                        dense
                        outlined
                        multiple
                        use-chips
                        use-input
                        new-value-mode="add-unique"
                        input-debounce="0"
                        :options="skillSlugOptions"
                        option-label="label"
                        option-value="value"
                        emit-value
                        map-options
                        label="拒绝的 Skill slug"
                        hint="与「允许」互斥：此处勾选会从允许列表移除同名 slug。"
                      />
                      <q-select
                        v-model="config.skillRuntime.allowed_tags"
                        dense
                        outlined
                        multiple
                        use-chips
                        use-input
                        new-value-mode="add-unique"
                        input-debounce="0"
                        label="要求的标签（AND）"
                        hint="可与用户话术中的 domain:/file_type: 提示合并"
                      />
                    </q-card-section>
                  </q-card>
                </div>
              </div>
            </section>
          </div>
        </q-tab-panel>

        <q-tab-panel name="evolution">
          <agent-evolution-panel v-model:range="evolutionRange" :evolution="config.evolution" :guardrails="config.evolution_guardrails" />
        </q-tab-panel>

        <q-tab-panel name="hooks">
          <q-banner rounded class="settings-placeholder-banner">Hook 绑定入口已保留。全局 Hook 管理见左侧 Tools / Hook 页面。</q-banner>
        </q-tab-panel>

        <q-tab-panel name="instances">
          <q-banner rounded class="settings-placeholder-banner">用户实例用于按用户覆盖 USER.md、权限与默认上下文；当前保留入口。</q-banner>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>

    <q-dialog v-model="promptDialog">
      <q-card class="prompt-dialog">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">系统提示词</div>
            <div class="text-caption text-grey-7">{{ tokenEstimateFor(promptPreview) }} tokens</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-tabs v-model="previewMode" dense active-color="primary" indicator-color="primary">
          <q-tab v-for="mode in promptModes" :key="mode.value" :name="mode.value" :label="mode.label" />
        </q-tabs>
        <q-separator />
        <q-card-section>
          <pre class="agent-prompt-preview">{{ promptPreview }}</pre>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="aiEditOpen">
      <q-card style="width: 560px; max-width: 92vw">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">AI 编辑</div>
            <div class="text-caption text-grey-7">描述您想要更改的内容。AI 将读取当前文件并相应更新。</div>
          </div>
          <q-btn flat round icon="close" v-close-popup />
        </q-card-section>
        <q-card-section>
          <q-input v-model="aiInstruction" outlined type="textarea" rows="6" label="编辑指令" placeholder="例如：使 Agent 更正式、添加中文支持..." />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated label="重新生成" @click="applyAiEditPlaceholder" />
        </q-card-actions>
      </q-card>
    </q-dialog>
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import type { Agent, AgentPromptFile, AgentRuntimeSettings } from "../features/agents/api";
import { useAgentDetailStore } from "../stores/agents";
import AgentAvatarPicker from "../components/avatar/AgentAvatarPicker.vue";
import AgentEvolutionPanel from "../components/agents/AgentEvolutionPanel.vue";
import AgentFilesPanel from "../components/agents/AgentFilesPanel.vue";
import AgentSettingsHeader from "../components/agents/AgentSettingsHeader.vue";
import {
  defaultAgentFiles,
  promptModes,
  statusOptions,
  tokenEstimateFor,
  type AgentFile,
  type EvolutionKey,
  type PromptMode
} from "../components/agents/agentUi";
import { listPlatformResources, type PlatformResource } from "../features/platform/api";
import { listSkills } from "../features/skills/api";
import { listTools } from "../features/tools/api";
import { isAvatarAssetRef } from "../features/avatar/iconModel";
import { useAppStore } from "../stores/app";
import { useAvatarCatalogStore } from "../stores/avatar";

const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const store = useAppStore();
const avatarCatalogStore = useAvatarCatalogStore();
const detailStore = useAgentDetailStore();
const { saving } = storeToRefs(detailStore);
const tab = ref("agent");
const promptDialog = ref(false);
const previewMode = ref<PromptMode>("complete");
const promptPreview = ref("");
const fileSplitter = ref(28);
const activeFile = ref("AGENTS_CORE.md");
const initialFileBodies = ref<Record<string, string>>({});
const aiEditOpen = ref(false);
const avatarPickerOpen = ref(false);
const aiInstruction = ref("");
const evolutionRange = ref("30d");
const providerModels = ref<PlatformResource[]>([]);
const providerModelSearch = ref("");
const loadingProviderModels = ref(false);

const form = reactive<Agent>({
  id: "",
  agent_key: "",
  display_name: "",
  provider: "",
  model: "",
  status: "active",
  is_default: false,
  is_favorite: false,
  icon: "",
  agent_description: "",
  category_position_id: "",
  system_prompt_mode: "complete",
  context_window: 0,
  budget_monthly_cents: 0,
  config_json: "",
  created_at: "",
  updated_at: "",
  deleted_at: ""
});

const config = reactive({
  self_evolve: true,
  subagents: {
    enabled: true,
    max_concurrency: 20,
    max_generation_depth: 1,
    max_children_per_agent: 5,
    archive_after_minutes: 60,
    max_retries: 2,
    model_override: ""
  },
  tools: {
    enabled: true,
    profile: "chat_only",
    tool_call_prefix: "",
    allow: [] as string[],
    deny: [] as string[],
    concurrent_allow: [] as string[],
    retry: {
      enabled: false,
      max_attempts: 2,
      initial_interval_ms: 500,
      backoff_factor: 2.0,
      max_interval_ms: 5000,
      jitter: true
    },
    parallel_enabled: false,
    streaming_enabled: false
  },
  memory: {
    enabled: true,
    max_chunk_length: 1000,
    max_results: 6,
    min_score: 0.35
  },
  memoryL0: {
    recent_window_turns: 12,
    recent_window_tokens: 0,
    summary_threshold: 0.6,
    summary_keep_turns: 4,
    truncate_strategy: "summary",
    inject_l1: true,
    inject_l3: true,
    inject_l4: false,
    l3_max_chunks: 5,
    l4_max_paths: 3,
    snapshot_mode: "on_warning"
  },
  memoryL1: {
    enabled: true,
    budget_tokens: 8192,
    field_max_tokens: 2048,
    history_keep_revisions: 10,
    default_schema_id: "",
    archive_on_idle_minutes: 60
  },
  memoryL2: {
    episode_enabled: true,
    episode_min_importance: 0.3,
    index_enabled: true,
    index_embedding_model: "",
    recall_enabled: false,
    recall_max: 3,
    retention_days: 90,
    archive_after_days: 30
  },
  memoryL3: {
    enabled: true,
    recall_top_k: 5,
    recall_min_score: 0.55,
    recall_scopes: ["agent", "user", "team", "workspace"] as string[],
    embedding_model: "",
    decay_interval_hours: 24,
    archive_threshold: 0.2,
    max_per_recall_chars: 1500
  },
  memoryL4: {
    enabled: true,
    graph_inject_neighbors: true,
    graph_max_neighbors: 6,
    graph_max_hops: 2,
    identity_inject: true,
    strategy_inject: false
  },
  evolutionSettings: {
    enabled: false,
    auto_apply: false,
    min_episodes: 20,
    min_negative_feedback: 3,
    throttle_hours: 24,
    proposal_ttl_days: 14,
    persona_max_chars: 1500,
    system_prompt_max_appends: 5
  },
  heartbeat: {
    enabled: false,
    interval_minutes: 30
  },
  evolution: {
    self_evolve: true,
    skill_evolve: true,
    evolution_metrics_enabled: true,
    evolution_suggestions_enabled: true
  } as Record<EvolutionKey, boolean>,
  evolution_guardrails: {
    max_change_per_period: 0.1,
    min_data_points: 100,
    rollback_on_decline_percent: 20
  },
  skillRuntime: {
    intent_routing_enabled: true,
    intent_max_paths: 3,
    max_skills_in_toolset: 32,
    allowed_slugs: [] as string[],
    denied_slugs: [] as string[],
    allowed_tags: [] as string[]
  },
  intent_pass: {
    enabled: true
  }
});

const files = reactive<AgentFile[]>(defaultAgentFiles.map((file) => ({ ...file })));

const heartbeatFile = computed(() => files.find((file) => file.name === "HEARTBEAT.md") ?? files[0]);
const activeFileMeta = computed(() => files.find((file) => file.name === activeFile.value) ?? files[0]);
const activeFileBody = computed({
  get: () => activeFileMeta.value.body,
  set: (value: string) => {
    activeFileMeta.value.body = value;
  }
});
const fileDirty = computed(() => activeFileBody.value !== (initialFileBodies.value[activeFile.value] ?? ""));
const budgetUSD = computed({
  get: () => Math.round((form.budget_monthly_cents || 0) / 100),
  set: (value: number) => {
    form.budget_monthly_cents = Math.round((Number(value) || 0) * 100);
  }
});
const providerModelOptions = computed(() =>
  providerModels.value
    .filter((row) => row.enabled && row.provider && row.model)
    .map((row) => {
      const contextWindowK = providerContextWindowK(row);
      return {
        label: row.name || row.model,
        value: row.id,
        caption: `${row.provider} / ${row.model}${contextWindowK ? ` · ${contextWindowK}K ctx` : ""}`,
        provider: row.provider,
        model: row.model
      };
    })
);
const filteredProviderModelOptions = computed(() => {
  const keyword = providerModelSearch.value.trim().toLowerCase();
  if (!keyword) return providerModelOptions.value;
  return providerModelOptions.value.filter((option) =>
    [option.label, option.caption, option.provider, option.model].some((value) => value.toLowerCase().includes(keyword))
  );
});
const selectedProviderModelID = computed(() => providerModelOptions.value.find((row) => row.provider === form.provider && row.model === form.model)?.value ?? "");

/** Native / legacy tool keys always listed so older agents keep working without catalog rows. */
const defaultNativeToolKeys = [
  "datetime",
  "web_search",
  "web_fetch",
  "list_files",
  "read_file",
  "write_file",
  "edit_file",
  "shell_exec"
];

const catalogTools = ref<{ key: string; display_name: string }[]>([]);
const loadingCatalogTools = ref(false);

/** 平台 Skill 下拉：名称 + slug + 状态（Agent 策略仍存 slug 列表）。 */
const skillSlugOptions = ref<{ label: string; value: string }[]>([]);
const loadingSkillSlugs = ref(false);

async function loadCatalogTools() {
  loadingCatalogTools.value = true;
  try {
    const res = await listTools({ page: 1, page_size: 500 });
    catalogTools.value = (res.items ?? [])
      .map((t) => ({ key: String(t.key ?? "").trim(), display_name: String(t.display_name ?? "").trim() || String(t.key ?? "").trim() }))
      .filter((t) => t.key !== "");
  } catch {
    catalogTools.value = [];
  } finally {
    loadingCatalogTools.value = false;
  }
}

async function loadSkillSlugOptions() {
  loadingSkillSlugs.value = true;
  try {
    const data = await listSkills({ page: 1, page_size: 500 });
    const seen = new Set<string>();
    const next: { label: string; value: string }[] = [];
    for (const s of data.items) {
      const slug = String(s.slug ?? "").trim();
      if (!slug || seen.has(slug)) continue;
      seen.add(slug);
      const statusTip =
        s.status === "published" ? "已发布" : s.status === "draft" ? "草稿" : s.status === "archived" ? "已归档" : s.status;
      next.push({
        label: `${s.name || slug} · ${slug} · ${statusTip}`,
        value: slug
      });
    }
    skillSlugOptions.value = next;
  } catch {
    skillSlugOptions.value = [];
  } finally {
    loadingSkillSlugs.value = false;
  }
}

const toolSelectOptions = computed(() => {
  const byKey = new Map<string, { label: string; value: string }>();
  for (const k of defaultNativeToolKeys) {
    byKey.set(k, { label: `${k} · 内置`, value: k });
  }
  for (const t of catalogTools.value) {
    const label =
      t.display_name && t.display_name !== t.key ? `${t.display_name} (${t.key})` : t.key;
    byKey.set(t.key, { label, value: t.key });
  }
  const extra = [...config.tools.allow, ...config.tools.deny, ...config.tools.concurrent_allow];
  for (const raw of extra) {
    const key = String(raw ?? "").trim();
    if (key && !byKey.has(key)) {
      byKey.set(key, { label: `${key} · 已保存`, value: key });
    }
  }
  return Array.from(byKey.values()).sort((a, b) => a.label.localeCompare(b.label, "zh-CN"));
});

const toolConflicts = computed(() => config.tools.allow.filter((tool) => config.tools.deny.includes(tool)));
// Profile options surface the current canonical names to the user.
// Backend still accepts legacy values (minimal/safe/system_admin) so
// existing agents keep their behaviour even before they are re-saved.
const toolProfileOptions = [
  { label: "chat_only · 仅对话（无工具）", value: "chat_only" },
  { label: "read_only · 只读 + 时间", value: "read_only" },
  { label: "coding · 文件读写 + 网页 + 技能", value: "coding" },
  { label: "research · 网页 + 检索 + 技能", value: "research" },
  { label: "full · 全工具（高权限，慎用）", value: "full" }
];
const truncateStrategyOptions = ["summary", "drop_oldest", "drop_tool_results", "hybrid"].map((value) => ({ label: value, value }));
const snapshotModeOptions = ["always", "on_warning", "off"].map((value) => ({ label: value, value }));
const memoryScopeOptions = ["agent", "user", "team", "workspace", "global"].map((value) => ({ label: value, value }));

/** 打开设置页时强制预热缩略图（支持 icon 为 asset id 或 asset_key） */
async function primeAvatarThumbnailCacheForAgentIcon() {
  const raw = String(form.icon ?? "").trim();
  if (!raw || /^(https?:|data:|blob:)/i.test(raw)) return;
  let fetchId = raw;
  if (!isAvatarAssetRef(raw)) {
    await avatarCatalogStore.ensureAgentsCatalog();
    const hit = avatarCatalogStore.agentsCatalog.find((a) => a.id === raw || (a.key && a.key === raw));
    if (!hit?.id) return;
    fetchId = hit.id;
  }
  avatarCatalogStore.forgetThumbnail(fetchId);
  await avatarCatalogStore.ensureThumbnail(fetchId);
}

async function applyLoadedAgent(agent: Agent | null | undefined) {
  if (!agent?.id) {
    $q.notify({ type: "warning", message: "未找到该 Agent" });
    router.back();
    return false;
  }
  Object.assign(form, agent);
  hydrateSettings(agent);
  store.upsertAgent(agent);
  snapshotFiles();
  previewMode.value = (form.system_prompt_mode as PromptMode) || "complete";
  await loadPromptPreview();
  await primeAvatarThumbnailCacheForAgentIcon();
  return true;
}

onMounted(async () => {
  const id = String(route.params.id ?? "").trim();
  if (!id) {
    $q.notify({ type: "negative", message: "缺少 Agent ID" });
    router.back();
    return;
  }
  try {
    const [agent] = await Promise.all([
      detailStore.fetchById(id),
      loadProviderModels(),
      loadCatalogTools(),
      loadSkillSlugOptions()
    ]);
    await applyLoadedAgent(agent);
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
    router.back();
  }
});

watch(
  () => String(route.params.id ?? "").trim(),
  async (newId, prevId) => {
    if (!newId || newId === prevId) return;
    try {
      const agent = await detailStore.fetchById(newId);
      await applyLoadedAgent(agent);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
      router.back();
    }
  }
);

watch(previewMode, () => void loadPromptPreview());
watch(
  () => form.system_prompt_mode,
  (value) => {
    previewMode.value = (value as PromptMode) || "complete";
    config.evolution.self_evolve = config.self_evolve;
  }
);
watch(
  () => config.evolution.self_evolve,
  (value) => {
    config.self_evolve = value;
  }
);

/** Skill slug 比较规则（与后端 Layer A 一致：小写 + trim） */
function normSkillSlug(s: string): string {
  return String(s ?? "").trim().toLowerCase();
}

/** 与运行时一致：拒绝列表优先，去掉与白名单重叠项后再去掉多余的拒绝项，保证两侧无交集 */
function reconcileSkillSlugListsDenyWins() {
  const rt = config.skillRuntime;
  const denySet = new Set(rt.denied_slugs.map(normSkillSlug).filter(Boolean));
  rt.allowed_slugs = rt.allowed_slugs.filter((a) => !denySet.has(normSkillSlug(a)));
  const allowSet = new Set(rt.allowed_slugs.map(normSkillSlug).filter(Boolean));
  rt.denied_slugs = rt.denied_slugs.filter((d) => !allowSet.has(normSkillSlug(d)));
}

/** 编辑中双向同步时抑制循环触发 */
const skillSlugListsSyncing = ref(false);

watch(
  () => config.skillRuntime.allowed_slugs,
  (allowed) => {
    if (skillSlugListsSyncing.value) return;
    skillSlugListsSyncing.value = true;
    try {
      const allowSet = new Set((allowed ?? []).map(normSkillSlug).filter(Boolean));
      config.skillRuntime.denied_slugs = config.skillRuntime.denied_slugs.filter((d) => !allowSet.has(normSkillSlug(d)));
    } finally {
      void nextTick(() => {
        skillSlugListsSyncing.value = false;
      });
    }
  },
  { deep: true }
);

watch(
  () => config.skillRuntime.denied_slugs,
  (denied) => {
    if (skillSlugListsSyncing.value) return;
    skillSlugListsSyncing.value = true;
    try {
      const denySet = new Set((denied ?? []).map(normSkillSlug).filter(Boolean));
      config.skillRuntime.allowed_slugs = config.skillRuntime.allowed_slugs.filter((a) => !denySet.has(normSkillSlug(a)));
    } finally {
      void nextTick(() => {
        skillSlugListsSyncing.value = false;
      });
    }
  },
  { deep: true }
);

function parseSkillRuntimeForm(raw?: string) {
  const out = {
    intent_routing_enabled: true,
    intent_max_paths: 3,
    max_skills_in_toolset: 32,
    allowed_slugs: [] as string[],
    denied_slugs: [] as string[],
    allowed_tags: [] as string[]
  };
  try {
    const o = JSON.parse(String(raw ?? "{}").trim() || "{}");
    if (typeof o.intent_routing_enabled === "boolean") out.intent_routing_enabled = o.intent_routing_enabled;
    if (typeof o.intent_max_paths === "number" && Number.isFinite(o.intent_max_paths)) {
      const n = Math.floor(o.intent_max_paths);
      if (n >= 1 && n <= 32) out.intent_max_paths = n;
    }
    if (typeof o.max_skills_in_toolset === "number" && Number.isFinite(o.max_skills_in_toolset)) {
      const n = Math.floor(o.max_skills_in_toolset);
      if (n >= 1 && n <= 256) out.max_skills_in_toolset = n;
    }
    const strList = (v: unknown) =>
      Array.isArray(v) ? v.map((x) => String(x).trim()).filter(Boolean) : [];
    out.allowed_slugs = strList(o.allowed_slugs);
    out.denied_slugs = strList(o.denied_slugs);
    out.allowed_tags = strList(o.allowed_tags);
  } catch {
    /* keep defaults */
  }
  return out;
}

function normalizeSkillRuntimeState() {
  const rt = config.skillRuntime;
  rt.intent_max_paths = Math.min(32, Math.max(1, Math.floor(Number(rt.intent_max_paths) || 3)));
  rt.max_skills_in_toolset = Math.min(256, Math.max(1, Math.floor(Number(rt.max_skills_in_toolset) || 32)));
  for (const key of ["allowed_slugs", "denied_slugs", "allowed_tags"] as const) {
    if (!Array.isArray(rt[key])) rt[key] = [];
    rt[key] = rt[key].map((x) => String(x).trim()).filter(Boolean);
  }
  skillSlugListsSyncing.value = true;
  try {
    reconcileSkillSlugListsDenyWins();
  } finally {
    void nextTick(() => {
      skillSlugListsSyncing.value = false;
    });
  }
}

function stringifySkillRuntimeJSON(): string {
  normalizeSkillRuntimeState();
  return JSON.stringify({
    intent_routing_enabled: config.skillRuntime.intent_routing_enabled,
    intent_max_paths: config.skillRuntime.intent_max_paths,
    max_skills_in_toolset: config.skillRuntime.max_skills_in_toolset,
    allowed_slugs: [...config.skillRuntime.allowed_slugs],
    denied_slugs: [...config.skillRuntime.denied_slugs],
    allowed_tags: [...config.skillRuntime.allowed_tags]
  });
}

function resetSkillRuntimeDefaults() {
  Object.assign(config.skillRuntime, parseSkillRuntimeForm("{}"));
  normalizeSkillRuntimeState();
  $q.notify({ type: "info", message: "Skill 策略已恢复默认（尚未保存）" });
}

function hydrateConfig(raw: string) {
  try {
    const parsed = JSON.parse(raw || "{}");
    Object.assign(config, {
      ...config,
      ...parsed,
      subagents: { ...config.subagents, ...(parsed.subagents || {}) },
      tools: { ...config.tools, ...(parsed.tools || {}), retry: { ...config.tools.retry, ...((parsed.tools || {}).retry || {}) } },
      memory: { ...config.memory, ...(parsed.memory || {}) },
      memoryL0: { ...config.memoryL0, ...(parsed.memoryL0 || {}) },
      memoryL1: { ...config.memoryL1, ...(parsed.memoryL1 || {}) },
      memoryL2: { ...config.memoryL2, ...(parsed.memoryL2 || {}) },
      memoryL3: { ...config.memoryL3, ...(parsed.memoryL3 || {}) },
      memoryL4: { ...config.memoryL4, ...(parsed.memoryL4 || {}) },
      evolutionSettings: { ...config.evolutionSettings, ...(parsed.evolutionSettings || {}) },
      heartbeat: { ...config.heartbeat, ...(parsed.heartbeat || {}) },
      evolution: { ...config.evolution, ...(parsed.evolution || {}), self_evolve: parsed.self_evolve ?? config.self_evolve },
      evolution_guardrails: { ...config.evolution_guardrails, ...(parsed.evolution_guardrails || {}) },
      skillRuntime: { ...config.skillRuntime, ...(parsed.skillRuntime || {}) },
      intent_pass: { ...config.intent_pass, ...(parsed.intent_pass || {}) }
    });
    if (Array.isArray(parsed.files)) {
      for (const saved of parsed.files) {
        const file = files.find((item) => item.name === saved.name);
        if (file) file.body = saved.body;
      }
    }
  } catch {
    // Legacy config can be plain text; keep defaults.
  }
}

function hydrateSettings(agent: Agent) {
  if (agent.settings) {
    Object.assign(config, {
      ...config,
      self_evolve: agent.settings.self_evolve,
      subagents: {
        enabled: agent.settings.subagents_enabled,
        max_concurrency: agent.settings.subagents_max_concurrency,
        max_generation_depth: agent.settings.subagents_max_generation_depth,
        max_children_per_agent: agent.settings.subagents_max_children_per_agent,
        archive_after_minutes: agent.settings.subagents_archive_after_minutes,
        max_retries: agent.settings.subagents_max_retries,
        model_override: agent.settings.subagents_model_override
      },
      tools: {
        enabled: agent.settings.tools_enabled,
        profile: agent.settings.tools_profile,
        tool_call_prefix: agent.settings.tools_tool_call_prefix,
        allow: parseJSONList(agent.settings.tools_allow_json),
        deny: parseJSONList(agent.settings.tools_deny_json),
        concurrent_allow: parseJSONList(agent.settings.tools_concurrent_allow_json),
        retry: {
          enabled: agent.settings.tools_retry_enabled ?? config.tools.retry.enabled,
          max_attempts: agent.settings.tools_retry_max_attempts ?? config.tools.retry.max_attempts,
          initial_interval_ms: agent.settings.tools_retry_initial_interval_ms ?? config.tools.retry.initial_interval_ms,
          backoff_factor: agent.settings.tools_retry_backoff_factor ?? config.tools.retry.backoff_factor,
          max_interval_ms: agent.settings.tools_retry_max_interval_ms ?? config.tools.retry.max_interval_ms,
          jitter: agent.settings.tools_retry_jitter ?? config.tools.retry.jitter
        },
        parallel_enabled: agent.settings.tools_parallel_enabled ?? config.tools.parallel_enabled,
        streaming_enabled: agent.settings.tools_streaming_enabled ?? config.tools.streaming_enabled
      },
      memory: {
        enabled: agent.settings.memory_enabled,
        max_chunk_length: agent.settings.memory_max_chunk_length,
        max_results: agent.settings.memory_max_results,
        min_score: agent.settings.memory_min_score
      },
      memoryL0: {
        recent_window_turns: agent.settings.l0_recent_window_turns ?? config.memoryL0.recent_window_turns,
        recent_window_tokens: agent.settings.l0_recent_window_tokens ?? config.memoryL0.recent_window_tokens,
        summary_threshold: agent.settings.l0_summary_threshold ?? config.memoryL0.summary_threshold,
        summary_keep_turns: agent.settings.l0_summary_keep_turns ?? config.memoryL0.summary_keep_turns,
        truncate_strategy: agent.settings.l0_truncate_strategy || config.memoryL0.truncate_strategy,
        inject_l1: agent.settings.l0_inject_l1 ?? config.memoryL0.inject_l1,
        inject_l3: agent.settings.l0_inject_l3 ?? config.memoryL0.inject_l3,
        inject_l4: agent.settings.l0_inject_l4 ?? config.memoryL0.inject_l4,
        l3_max_chunks: agent.settings.l0_l3_max_chunks ?? config.memoryL0.l3_max_chunks,
        l4_max_paths: agent.settings.l0_l4_max_paths ?? config.memoryL0.l4_max_paths,
        snapshot_mode: agent.settings.l0_snapshot_mode || config.memoryL0.snapshot_mode
      },
      memoryL1: {
        enabled: agent.settings.l1_enabled ?? config.memoryL1.enabled,
        budget_tokens: agent.settings.l1_budget_tokens ?? config.memoryL1.budget_tokens,
        field_max_tokens: agent.settings.l1_field_max_tokens ?? config.memoryL1.field_max_tokens,
        history_keep_revisions: agent.settings.l1_history_keep_revisions ?? config.memoryL1.history_keep_revisions,
        default_schema_id: agent.settings.l1_default_schema_id || config.memoryL1.default_schema_id,
        archive_on_idle_minutes: agent.settings.l1_archive_on_idle_minutes ?? config.memoryL1.archive_on_idle_minutes
      },
      memoryL2: {
        episode_enabled: agent.settings.l2_episode_enabled ?? config.memoryL2.episode_enabled,
        episode_min_importance: agent.settings.l2_episode_min_importance ?? config.memoryL2.episode_min_importance,
        index_enabled: agent.settings.l2_index_enabled ?? config.memoryL2.index_enabled,
        index_embedding_model: agent.settings.l2_index_embedding_model || config.memoryL2.index_embedding_model,
        recall_enabled: agent.settings.l2_recall_enabled ?? config.memoryL2.recall_enabled,
        recall_max: agent.settings.l2_recall_max ?? config.memoryL2.recall_max,
        retention_days: agent.settings.l2_retention_days ?? config.memoryL2.retention_days,
        archive_after_days: agent.settings.l2_archive_after_days ?? config.memoryL2.archive_after_days
      },
      memoryL3: {
        enabled: agent.settings.l3_enabled ?? config.memoryL3.enabled,
        recall_top_k: agent.settings.l3_recall_top_k ?? config.memoryL3.recall_top_k,
        recall_min_score: agent.settings.l3_recall_min_score ?? config.memoryL3.recall_min_score,
        recall_scopes: parseJSONList(agent.settings.l3_recall_scopes_json || JSON.stringify(config.memoryL3.recall_scopes)),
        embedding_model: agent.settings.l3_embedding_model || config.memoryL3.embedding_model,
        decay_interval_hours: agent.settings.l3_decay_interval_hours ?? config.memoryL3.decay_interval_hours,
        archive_threshold: agent.settings.l3_archive_threshold ?? config.memoryL3.archive_threshold,
        max_per_recall_chars: agent.settings.l3_max_per_recall_chars ?? config.memoryL3.max_per_recall_chars
      },
      memoryL4: {
        enabled: agent.settings.l4_enabled ?? config.memoryL4.enabled,
        graph_inject_neighbors: agent.settings.l4_graph_inject_neighbors ?? config.memoryL4.graph_inject_neighbors,
        graph_max_neighbors: agent.settings.l4_graph_max_neighbors ?? config.memoryL4.graph_max_neighbors,
        graph_max_hops: agent.settings.l4_graph_max_hops ?? config.memoryL4.graph_max_hops,
        identity_inject: agent.settings.l4_identity_inject ?? config.memoryL4.identity_inject,
        strategy_inject: agent.settings.l4_strategy_inject ?? config.memoryL4.strategy_inject
      },
      evolutionSettings: {
        enabled: agent.settings.evo_enabled ?? config.evolutionSettings.enabled,
        auto_apply: agent.settings.evo_auto_apply ?? config.evolutionSettings.auto_apply,
        min_episodes: agent.settings.evo_min_episodes ?? config.evolutionSettings.min_episodes,
        min_negative_feedback: agent.settings.evo_min_negative_feedback ?? config.evolutionSettings.min_negative_feedback,
        throttle_hours: agent.settings.evo_throttle_hours ?? config.evolutionSettings.throttle_hours,
        proposal_ttl_days: agent.settings.evo_proposal_ttl_days ?? config.evolutionSettings.proposal_ttl_days,
        persona_max_chars: agent.settings.evo_persona_max_chars ?? config.evolutionSettings.persona_max_chars,
        system_prompt_max_appends: agent.settings.evo_system_prompt_max_appends ?? config.evolutionSettings.system_prompt_max_appends
      },
      heartbeat: {
        enabled: agent.settings.heartbeat_enabled,
        interval_minutes: agent.settings.heartbeat_interval_minutes
      },
      evolution: {
        self_evolve: agent.settings.evolution_self_evolve,
        skill_evolve: agent.settings.evolution_skill_evolve,
        evolution_metrics_enabled: agent.settings.evolution_metrics_enabled,
        evolution_suggestions_enabled: agent.settings.evolution_suggestions_enabled
      },
      evolution_guardrails: {
        max_change_per_period: agent.settings.guardrail_max_change_per_period,
        min_data_points: agent.settings.guardrail_min_data_points,
        rollback_on_decline_percent: agent.settings.guardrail_rollback_on_decline_percent
      },
      skillRuntime: parseSkillRuntimeForm(agent.settings.skill_runtime_json),
      intent_pass: {
        enabled: agent.settings.intent_pass_enabled !== false
      }
    });
  } else {
    hydrateConfig(agent.config_json);
  }
  normalizeSkillRuntimeState();
  if (agent.files?.length) {
    hydrateFiles(agent.files);
  }
}

async function saveAgent() {
  if (!selectedProviderModelID.value) {
    $q.notify({ type: "negative", message: "请选择已录入且启用的模型" });
    return;
  }
  try {
    const updated = await detailStore.patch(form.id, {
      ...form,
      settings: buildSettingsPayload(),
      files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 })),
      config_json: JSON.stringify({
        self_evolve: config.self_evolve,
        subagents: config.subagents,
        tools: config.tools,
        memory: config.memory,
        memoryL0: config.memoryL0,
        memoryL1: config.memoryL1,
        memoryL2: config.memoryL2,
        memoryL3: config.memoryL3,
        memoryL4: config.memoryL4,
        evolutionSettings: config.evolutionSettings,
        heartbeat: config.heartbeat,
        evolution: config.evolution,
        evolution_guardrails: config.evolution_guardrails,
        skillRuntime: config.skillRuntime,
        intent_pass: config.intent_pass,
        files: files.map((file) => ({ name: file.name, body: file.body }))
      })
    });
    Object.assign(form, updated);
    hydrateSettings(updated);
    store.upsertAgent(updated);
    snapshotFiles();
    await loadPromptPreview();
    await primeAvatarThumbnailCacheForAgentIcon();
    $q.notify({ type: "positive", message: "已保存" });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
  }
}

async function toggleFavorite() {
  const next = !form.is_favorite;
  form.is_favorite = next;
  try {
    const updated = await detailStore.patch(form.id, {
      ...form,
      is_favorite: next,
      settings: buildSettingsPayload(),
      files: files.map((file, index) => ({ name: file.name, body: file.body, sort_order: (index + 1) * 10 }))
    });
    Object.assign(form, updated);
    hydrateSettings(updated);
    store.upsertAgent(updated);
  } catch (error) {
    form.is_favorite = !next;
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "收藏保存失败" });
  }
}

async function loadPromptPreview() {
  if (!form.id) return;
  promptPreview.value = await detailStore.fetchPromptPreview(form.id, previewMode.value);
}

async function loadProviderModels() {
  loadingProviderModels.value = true;
  try {
    providerModels.value = await listPlatformResources("llm-provider-models");
  } finally {
    loadingProviderModels.value = false;
  }
}

function selectProviderModel(value: string | null) {
  const selected = providerModels.value.find((row) => row.id === value);
  if (!selected) {
    form.provider = "";
    form.model = "";
    return;
  }
  form.provider = selected.provider;
  form.model = selected.model;
}

function filterProviderModels(value: string, update: (callback: () => void) => void) {
  update(() => {
    providerModelSearch.value = value;
  });
}

function providerContextWindowK(row: PlatformResource) {
  try {
    const parsed = JSON.parse(row.config_json || "{}") as { context_window_k?: number | string | null };
    const value = Number(parsed.context_window_k);
    return Number.isFinite(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

function reloadActiveFile() {
  activeFileBody.value = initialFileBodies.value[activeFile.value] ?? activeFileBody.value;
}

function updateFileBody(name: string, body: string) {
  const file = files.find((item) => item.name === name);
  if (file) file.body = body;
}

function snapshotFiles() {
  initialFileBodies.value = Object.fromEntries(files.map((file) => [file.name, file.body]));
}

function hydrateFiles(savedFiles: AgentPromptFile[]) {
  const byName = new Map(savedFiles.map((file) => [file.name, file]));
  for (const file of files) {
    const saved = byName.get(file.name);
    if (saved) file.body = saved.body;
  }
  for (const saved of savedFiles) {
    if (!files.some((file) => file.name === saved.name)) {
      files.push({ name: saved.name, caption: "自定义 Prompt 文件", body: saved.body });
    }
  }
}

function buildSettingsPayload(): AgentRuntimeSettings {
  return {
    self_evolve: config.self_evolve,
    subagents_enabled: config.subagents.enabled,
    subagents_max_concurrency: config.subagents.max_concurrency,
    subagents_max_generation_depth: config.subagents.max_generation_depth,
    subagents_max_children_per_agent: config.subagents.max_children_per_agent,
    subagents_archive_after_minutes: config.subagents.archive_after_minutes,
    subagents_max_retries: config.subagents.max_retries,
    subagents_model_override: config.subagents.model_override,
    tools_enabled: config.tools.enabled,
    tools_profile: config.tools.profile,
    tools_tool_call_prefix: config.tools.tool_call_prefix,
    tools_allow_json: JSON.stringify(config.tools.allow),
    tools_deny_json: JSON.stringify(config.tools.deny),
    tools_concurrent_allow_json: JSON.stringify(config.tools.concurrent_allow),
    tools_retry_enabled: config.tools.retry.enabled,
    tools_retry_max_attempts: config.tools.retry.max_attempts,
    tools_retry_initial_interval_ms: config.tools.retry.initial_interval_ms,
    tools_retry_backoff_factor: config.tools.retry.backoff_factor,
    tools_retry_max_interval_ms: config.tools.retry.max_interval_ms,
    tools_retry_jitter: config.tools.retry.jitter,
    tools_parallel_enabled: config.tools.parallel_enabled,
    tools_streaming_enabled: config.tools.streaming_enabled,
    memory_enabled: config.memory.enabled,
    memory_max_chunk_length: config.memory.max_chunk_length,
    memory_max_results: config.memory.max_results,
    memory_min_score: config.memory.min_score,
    l0_recent_window_turns: config.memoryL0.recent_window_turns,
    l0_recent_window_tokens: config.memoryL0.recent_window_tokens,
    l0_summary_threshold: config.memoryL0.summary_threshold,
    l0_summary_keep_turns: config.memoryL0.summary_keep_turns,
    l0_truncate_strategy: config.memoryL0.truncate_strategy,
    l0_inject_l1: config.memoryL0.inject_l1,
    l0_inject_l3: config.memoryL0.inject_l3,
    l0_inject_l4: config.memoryL0.inject_l4,
    l0_l3_max_chunks: config.memoryL0.l3_max_chunks,
    l0_l4_max_paths: config.memoryL0.l4_max_paths,
    l0_snapshot_mode: config.memoryL0.snapshot_mode,
    l1_enabled: config.memoryL1.enabled,
    l1_budget_tokens: config.memoryL1.budget_tokens,
    l1_field_max_tokens: config.memoryL1.field_max_tokens,
    l1_history_keep_revisions: config.memoryL1.history_keep_revisions,
    l1_default_schema_id: config.memoryL1.default_schema_id,
    l1_archive_on_idle_minutes: config.memoryL1.archive_on_idle_minutes,
    l2_episode_enabled: config.memoryL2.episode_enabled,
    l2_episode_min_importance: config.memoryL2.episode_min_importance,
    l2_index_enabled: config.memoryL2.index_enabled,
    l2_index_embedding_model: config.memoryL2.index_embedding_model,
    l2_recall_enabled: config.memoryL2.recall_enabled,
    l2_recall_max: config.memoryL2.recall_max,
    l2_retention_days: config.memoryL2.retention_days,
    l2_archive_after_days: config.memoryL2.archive_after_days,
    l3_enabled: config.memoryL3.enabled,
    l3_recall_top_k: config.memoryL3.recall_top_k,
    l3_recall_min_score: config.memoryL3.recall_min_score,
    l3_recall_scopes_json: JSON.stringify(config.memoryL3.recall_scopes),
    l3_embedding_model: config.memoryL3.embedding_model,
    l3_decay_interval_hours: config.memoryL3.decay_interval_hours,
    l3_archive_threshold: config.memoryL3.archive_threshold,
    l3_max_per_recall_chars: config.memoryL3.max_per_recall_chars,
    l4_enabled: config.memoryL4.enabled,
    l4_graph_inject_neighbors: config.memoryL4.graph_inject_neighbors,
    l4_graph_max_neighbors: config.memoryL4.graph_max_neighbors,
    l4_graph_max_hops: config.memoryL4.graph_max_hops,
    l4_identity_inject: config.memoryL4.identity_inject,
    l4_strategy_inject: config.memoryL4.strategy_inject,
    evo_enabled: config.evolutionSettings.enabled,
    evo_auto_apply: config.evolutionSettings.auto_apply,
    evo_min_episodes: config.evolutionSettings.min_episodes,
    evo_min_negative_feedback: config.evolutionSettings.min_negative_feedback,
    evo_throttle_hours: config.evolutionSettings.throttle_hours,
    evo_proposal_ttl_days: config.evolutionSettings.proposal_ttl_days,
    evo_persona_max_chars: config.evolutionSettings.persona_max_chars,
    evo_system_prompt_max_appends: config.evolutionSettings.system_prompt_max_appends,
    heartbeat_enabled: config.heartbeat.enabled,
    heartbeat_interval_minutes: config.heartbeat.interval_minutes,
    evolution_self_evolve: config.evolution.self_evolve,
    evolution_skill_evolve: config.evolution.skill_evolve,
    evolution_metrics_enabled: config.evolution.evolution_metrics_enabled,
    evolution_suggestions_enabled: config.evolution.evolution_suggestions_enabled,
    guardrail_max_change_per_period: config.evolution_guardrails.max_change_per_period,
    guardrail_min_data_points: config.evolution_guardrails.min_data_points,
    guardrail_rollback_on_decline_percent: config.evolution_guardrails.rollback_on_decline_percent,
    skill_runtime_json: stringifySkillRuntimeJSON(),
    intent_pass_enabled: config.intent_pass.enabled
  };
}

function parseJSONList(raw: string) {
  try {
    const parsed = JSON.parse(raw || "[]");
    return Array.isArray(parsed) ? parsed.map(String) : [];
  } catch {
    return [];
  }
}

function applyAiEditPlaceholder() {
  if (!aiInstruction.value.trim()) return;
  activeFileBody.value = `${activeFileBody.value}\n\n<!-- AI edit instruction: ${aiInstruction.value.trim()} -->`;
  aiInstruction.value = "";
  aiEditOpen.value = false;
}

async function copyKey() {
  await copyToClipboard(form.agent_key);
  $q.notify({ type: "positive", message: "Agent 标识已复制" });
}
</script>

<style scoped>
.agent-settings {
  display: flex;
  flex-direction: column;
  min-height: 100%;
  padding: 28px;
  background: var(--canvas-base);
  color: var(--color-text-primary);
}

.agent-settings--files-fill {
  flex: 1 1 auto;
  min-height: calc(100vh - 52px);
  min-height: calc(100dvh - 52px);
}

.settings-shell,
.settings-section {
  border-radius: 24px;
}

.agent-settings-tabs {
  padding: 8px 16px 0;
  background: var(--nav-bg-light, var(--glass-surface));
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.agent-settings-tabs :deep(.q-tab) {
  min-height: 46px;
  border-radius: 14px 14px 0 0;
  font-weight: 700;
  color: var(--color-text-secondary);
}

.agent-settings-tabs :deep(.q-tab--active) {
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-accent) 14%, transparent);
}

.agent-settings-tabs :deep(.q-tab__indicator) {
  height: 3px;
  border-radius: 2px;
  background: var(--color-accent);
}

.settings-panels {
  background: transparent;
}

.settings-panels :deep(.q-tab-panel) {
  padding: 18px;
}

.settings-shell {
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
  box-shadow: none;
  overflow: hidden;
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.settings-shell--fill {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  min-height: 0;
  height: calc(100vh - 192px);
  height: calc(100dvh - 192px);
  max-height: calc(100vh - 192px);
  max-height: calc(100dvh - 192px);
  overflow: hidden;
}

.settings-shell--fill .settings-panels {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.settings-shell--fill .settings-panels :deep(.q-tab-panel.settings-tab-panel-fill) {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.settings-grid {
  display: grid;
  gap: 18px;
}

.settings-section {
  padding: 20px;
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
  box-shadow: none;
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}

.prompt-mode-card {
  height: 100%;
  border-color: var(--glass-border);
  border-radius: 18px;
  background: var(--glass-elevated, rgba(255, 255, 255, 0.72));
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    background 180ms ease;
}

.prompt-mode-card:hover {
  transform: translateY(-2px);
  background: var(--glass-surface-hover);
  border-color: var(--glass-border);
}

.prompt-mode-card.is-active {
  border-color: var(--color-accent);
  background: var(--interaction-surface-hover, rgba(255, 243, 228, 0.92));
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.45);
}

.prompt-mode-card__token {
  background: var(--glass-elevated, #ffffff);
  color: var(--color-text-secondary);
  font-weight: 700;
}

.capability-card {
  border-color: var(--glass-border);
  border-radius: 20px;
  overflow: hidden;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.capability-card :deep(.q-card__section:first-child) {
  background: var(--glass-elevated);
}

.settings-section :deep(.q-field__control) {
  border-radius: 14px;
  background: var(--glass-elevated);
}

.settings-section :deep(.q-toggle__label) {
  font-weight: 600;
  color: var(--color-text-primary);
}

.settings-warning-banner {
  background: #fff7ed;
  color: #9a3412;
}

.settings-info-banner {
  background: #eff6ff;
  color: #1e40af;
}

.settings-placeholder-banner {
  background: var(--interaction-surface-hover, #f2f4f7);
  color: var(--color-text-primary);
}

.prompt-dialog {
  width: 860px;
  max-width: 94vw;
  border-radius: 24px;
  border: 1px solid var(--glass-border);
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-elevated));
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated));
  box-shadow: none;
}

.agent-prompt-preview {
  max-height: 64vh;
  overflow: auto;
  margin: 0;
  white-space: pre-wrap;
  padding: 16px;
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  background: var(--glass-elevated);
  color: var(--color-text-primary);
  line-height: 1.6;
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
}

.min-width-0 {
  min-width: 0;
}

body.body--dark .agent-settings {
  background: var(--canvas-base);
  color: var(--color-text-primary);
}

body.body--dark .settings-shell {
  border-color: var(--glass-border);
  background: var(--glass-surface);
  box-shadow: none;
}

body.body--dark .agent-settings-tabs {
  background: var(--nav-bg-dark, var(--glass-surface));
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}

body.body--dark .agent-settings-tabs :deep(.q-tab--active) {
  color: #f8fafc;
  background: color-mix(in srgb, var(--color-accent) 22%, transparent);
}

body.body--dark .settings-section {
  border-color: var(--glass-border);
  background: var(--glass-surface);
  box-shadow: none;
}

body.body--dark .prompt-mode-card,
body.body--dark .capability-card {
  border-color: var(--glass-border);
  background: var(--glass-surface);
}

body.body--dark .prompt-mode-card:hover {
  background: var(--glass-surface-hover);
  border-color: var(--glass-border-hover, var(--glass-border));
}

body.body--dark .prompt-mode-card.is-active {
  border-color: var(--color-neon-cyan);
  background: rgba(0, 229, 255, 0.08);
  box-shadow: 0 0 0 1px rgba(0, 229, 255, 0.35);
}

body.body--dark .prompt-mode-card__token {
  background: rgba(30, 41, 59, 0.94);
  color: #cbd5e1;
}

body.body--dark .capability-card :deep(.q-card__section:first-child) {
  background: rgba(30, 41, 59, 0.86);
}

body.body--dark .settings-section :deep(.q-field__control) {
  background: rgba(15, 23, 42, 0.72);
}

body.body--dark .settings-section :deep(.q-toggle__label) {
  color: #e2e8f0;
}

body.body--dark .settings-section :deep(.text-grey-7) {
  color: #94a3b8 !important;
}

body.body--dark .settings-warning-banner {
  background: rgba(154, 52, 18, 0.22);
  color: #fed7aa;
}

body.body--dark .settings-info-banner {
  background: rgba(30, 58, 138, 0.35);
  color: #bfdbfe;
}

body.body--dark .settings-placeholder-banner {
  background: rgba(30, 41, 59, 0.82);
  color: #cbd5e1;
}

body.body--dark .prompt-dialog {
  background: var(--glass-surface);
  border-color: var(--glass-border);
  box-shadow: none;
}

body.body--dark .agent-prompt-preview {
  border-color: var(--glass-border);
  background: var(--glass-surface-hover);
  color: var(--color-text-primary);
}

@media (max-width: 599px) {
  .agent-settings {
    padding: 18px;
  }

  .settings-panels :deep(.q-tab-panel) {
    padding: 14px;
  }
}
</style>
