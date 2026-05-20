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
        @open-advanced="advancedDialog = true"
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
        <q-tab name="a2a" label="A2A" />
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
                  <div class="text-subtitle1 text-weight-bold">模型</div>
                  <div class="text-caption text-grey-7">选择数据库已录入的模型；单价在 Provider 管理中维护并同步至 model_pricing_rules。</div>
                </div>
              </div>
              <div class="row q-col-gutter-md">
                <q-select
                  :model-value="selectedProviderModelID"
                  class="col-12"
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
              </div>
              <q-banner rounded class="q-mt-md settings-info-banner">
                月度费用上限请在
                <a href="#" class="text-primary" @click.prevent="tab = 'permissions'">「权限」</a>
                Tab 的「用量配额」中配置（写入 usage_quotas，Chat Turn 前生效）。Agent 表上的 budget_monthly_cents 字段已弃用展示。
              </q-banner>
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
                <div class="col-12">
                  <agent-tools-section :agent-id="agentId" />
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
          <div class="settings-grid">
            <agent-usage-quota-panel v-if="form.id" :agent-id="form.id" />
            <q-banner v-else rounded class="settings-placeholder-banner">加载 Agent 后可配置用量配额。</q-banner>
          </div>
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
          <agent-evolution-panel v-model:range="evolutionRange" :agent-id="agentId" :evolution="config.evolution" :guardrails="config.evolution_guardrails" />
        </q-tab-panel>

        <q-tab-panel name="hooks">
          <agent-hooks-panel :agent-id="agentId" :agent-key="form.agent_key" />
        </q-tab-panel>

        <q-tab-panel name="a2a">
          <agent-settings-a2-a-tab
            v-if="form.agent_kind === 'a2a_proxy'"
            :agent-id="agentId"
            :a2a-proxy="form.a2a_proxy_config"
            @saved="reloadAgent"
          />
          <agent-settings-a2-a-endpoint-tab v-else :agent-id="agentId" />
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
    <agent-advanced-dialog
      v-model:open="advancedDialog"
      :saving="saving"
      :channel-id="advancedState.channel_id"
      :chat-id-input="advancedState.chat_id"
      :workspace-input="advancedState.workspace"
      :reasoning-mode-input="advancedState.reasoning_mode"
      :reasoning-level-input="advancedState.reasoning_level"
      :compaction-enabled-input="advancedState.context_compaction_enabled"
      :session-summary-enabled-input="advancedState.session_summary_enabled"
      :truncate-strategy-input="advancedState.truncate_strategy"
      :recent-window-turns-input="advancedState.recent_window_turns"
      :recent-window-tokens-input="advancedState.recent_window_tokens"
      :summary-keep-turns-input="advancedState.summary_keep_turns"
      @save="onAdvancedSave"
    />
    <agent-avatar-picker v-model="form.icon" v-model:open="avatarPickerOpen" />
  </q-page>
</template>

<script setup lang="ts">
import AgentAvatarPicker from "../components/avatar/AgentAvatarPicker.vue";
import AgentEvolutionPanel from "../components/agents/AgentEvolutionPanel.vue";
import AgentFilesPanel from "../components/agents/AgentFilesPanel.vue";
import AgentSettingsHeader from "../components/agents/AgentSettingsHeader.vue";
import AgentAdvancedDialog from "../components/agents/AgentAdvancedDialog.vue";
import AgentToolsSection from "../components/agents/AgentToolsSection.vue";
import AgentHooksPanel from "../components/agents/AgentHooksPanel.vue";
import AgentSettingsA2ATab from "../components/agents/AgentSettingsA2ATab.vue";
import AgentSettingsA2AEndpointTab from "../components/agents/AgentSettingsA2AEndpointTab.vue";
import AgentUsageQuotaPanel from "../components/agents/AgentUsageQuotaPanel.vue";
import { useAgentSettingsPage } from "../features/agents/useAgentSettingsPage";

const {
  tab,
  form,
  config,
  saving,
  router,
  avatarPickerOpen,
  promptDialog,
  advancedDialog,
  toggleFavorite,
  reloadAgent,
  saveAgent,
  promptModes,
  statusOptions,
  copyKey,
  selectedProviderModelID,
  filteredProviderModelOptions,
  loadingProviderModels,
  filterProviderModels,
  selectProviderModel,
  toolProfileOptions,
  toolSelectOptions,
  loadingCatalogTools,
  toolConflicts,
  agentId,
  heartbeatFile,
  activeFile,
  fileSplitter,
  files,
  fileDirty,
  updateFileBody,
  reloadActiveFile,
  aiEditOpen,
  truncateStrategyOptions,
  snapshotModeOptions,
  memoryScopeOptions,
  loadSkillSlugOptions,
  resetSkillRuntimeDefaults,
  loadingSkillSlugs,
  skillSlugOptions,
  evolutionRange,
  tokenEstimateFor,
  previewMode,
  promptPreview,
  aiInstruction,
  applyAiEditPlaceholder,
  advancedState,
  onAdvancedSave
} = useAgentSettingsPage();
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
  background: var(--glass-elevated, rgb(255 255 255 / 72%));
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
  background: var(--interaction-surface-hover, rgb(255 243 228 / 92%));
  box-shadow: inset 0 1px 0 rgb(255 255 255 / 45%);
}

.prompt-mode-card__token {
  background: var(--glass-elevated, var(--color-on-accent));
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
  background: var(--color-status-warning-bg);
  color: var(--color-status-warning-text-dark);
}

.settings-info-banner {
  background: var(--color-status-info-bg-alt);
  color: var(--color-status-info-text);
}

.settings-placeholder-banner {
  background: var(--interaction-surface-hover, var(--color-interaction-surface-alt));
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
  color: var(--color-surface-soft);
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
  background: rgb(0 229 255 / 8%);
  box-shadow: 0 0 0 1px rgb(0 229 255 / 35%);
}

body.body--dark .prompt-mode-card__token {
  background: rgb(30 41 59 / 94%);
  color: var(--color-text-slate-300);
}

body.body--dark .capability-card :deep(.q-card__section:first-child) {
  background: rgb(30 41 59 / 86%);
}

body.body--dark .settings-section :deep(.q-field__control) {
  background: rgb(15 23 42 / 72%);
}

body.body--dark .settings-section :deep(.q-toggle__label) {
  color: var(--color-text-dark);
}

body.body--dark .settings-section :deep(.text-grey-7) {
  color: var(--color-text-tertiary) !important;
}

body.body--dark .settings-warning-banner {
  background: rgb(154 52 18 / 22%);
  color: var(--color-accent-orange-bg);
}

body.body--dark .settings-info-banner {
  background: rgb(30 58 138 / 35%);
  color: var(--color-accent-blue-light);
}

body.body--dark .settings-placeholder-banner {
  background: rgb(30 41 59 / 82%);
  color: var(--color-text-slate-300);
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

@media (width <= 599px) {
  .agent-settings {
    padding: 18px;
  }

  .settings-panels :deep(.q-tab-panel) {
    padding: 14px;
  }
}
</style>
