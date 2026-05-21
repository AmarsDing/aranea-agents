<template>
  <div class="settings-grid">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">记忆总览</span>
        </div>
        <div class="text-caption text-grey-7">统一控制上下文、工作记忆、会话事件、语义知识和进化记忆。</div>
        <q-toggle v-model="config.memory.enabled" color="primary" label="记忆总开关" />
      </div>
      <div class="app-form-field-grid">
        <q-input v-model.number="config.memory.max_results" dense outlined type="number" label="兼容最大结果数" />
        <q-input v-model.number="config.memory.min_score" dense outlined type="number" step="0.01" label="兼容最低分数" />
        <q-input v-model.number="config.memory.max_chunk_length" dense outlined type="number" label="兼容最大块长度" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">L0 上下文窗口</span>
        </div>
        <div class="text-caption text-grey-7">控制下一次 prompt 如何装配、摘要和记录快照。</div>
      </div>
      <div class="app-form-field-grid">
        <q-input v-model.number="config.memoryL0.recent_window_turns" dense outlined type="number" label="最近窗口轮数" />
        <q-input v-model.number="config.memoryL0.recent_window_tokens" dense outlined type="number" label="最近窗口 Tokens" hint="0 = 自动" />
        <q-input v-model.number="config.memoryL0.summary_threshold" dense outlined type="number" step="0.05" label="摘要触发阈值" />
        <q-input v-model.number="config.memoryL0.summary_keep_turns" dense outlined type="number" label="摘要保留轮数" />
        <q-select v-model="config.memoryL0.truncate_strategy" dense outlined emit-value map-options label="裁剪策略" :options="truncateStrategyOptions" />
        <q-select v-model="config.memoryL0.snapshot_mode" dense outlined emit-value map-options label="快照模式" :options="snapshotModeOptions" />
        <q-input v-model.number="config.memoryL0.l3_max_chunks" dense outlined type="number" label="L3 最大片段" />
        <q-input v-model.number="config.memoryL0.l4_max_paths" dense outlined type="number" label="L4 最大路径" />
        <q-toggle v-model="config.memoryL0.inject_l1" color="primary" label="注入 L1" />
        <q-toggle v-model="config.memoryL0.inject_l3" color="primary" label="注入 L3" />
        <q-toggle v-model="config.memoryL0.inject_l4" color="primary" label="注入 L4" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">L1 工作记忆</span>
        </div>
        <div class="text-caption text-grey-7">当前任务的结构化状态、版本和预算。</div>
        <q-toggle v-model="config.memoryL1.enabled" color="primary" label="启用 L1" />
      </div>
      <div class="app-form-field-grid">
        <q-input v-model.number="config.memoryL1.budget_tokens" dense outlined type="number" label="任务预算 Tokens" />
        <q-input v-model.number="config.memoryL1.field_max_tokens" dense outlined type="number" label="单字段上限" />
        <q-input v-model.number="config.memoryL1.history_keep_revisions" dense outlined type="number" label="版本保留数" />
        <q-input v-model.number="config.memoryL1.archive_on_idle_minutes" dense outlined type="number" label="闲置归档分钟" />
        <q-input v-model="config.memoryL1.default_schema_id" class="app-field-long" dense outlined label="默认 Schema ID" placeholder="schema_xxx" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">L2 会话事件</span>
        </div>
        <div class="text-caption text-grey-7">控制 episode、索引、回忆注入和保留周期。</div>
        <q-toggle v-model="config.memoryL2.episode_enabled" color="primary" label="启用 Episode" />
      </div>
      <div class="app-form-field-grid">
        <q-input v-model.number="config.memoryL2.episode_min_importance" dense outlined type="number" step="0.05" label="最低重要性" />
        <q-toggle v-model="config.memoryL2.index_enabled" color="primary" label="启用索引" />
        <q-toggle v-model="config.memoryL2.recall_enabled" color="primary" label="允许 L2 Recall" />
        <q-input v-model.number="config.memoryL2.recall_max" dense outlined type="number" label="Recall 最大数" />
        <q-input v-model="config.memoryL2.index_embedding_model" dense outlined label="Embedding 模型" />
        <q-input v-model.number="config.memoryL2.retention_days" dense outlined type="number" label="保留天数" />
        <q-input v-model.number="config.memoryL2.archive_after_days" dense outlined type="number" label="归档天数" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">L3 语义知识</span>
        </div>
        <div class="text-caption text-grey-7">跨会话事实、偏好、规则和冲突治理。</div>
        <q-toggle v-model="config.memoryL3.enabled" color="primary" label="启用 L3" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--wide">
        <q-input v-model.number="config.memoryL3.recall_top_k" dense outlined type="number" label="Recall TopK" />
        <q-input v-model.number="config.memoryL3.recall_min_score" dense outlined type="number" step="0.05" label="最小分数" />
        <q-select v-model="config.memoryL3.recall_scopes" class="app-field-long" dense outlined multiple use-chips emit-value map-options label="检索作用域" :options="memoryScopeOptions" />
        <q-input v-model="config.memoryL3.embedding_model" dense outlined label="Embedding 模型" />
        <q-input v-model.number="config.memoryL3.decay_interval_hours" dense outlined type="number" label="衰减间隔小时" />
        <q-input v-model.number="config.memoryL3.archive_threshold" dense outlined type="number" step="0.05" label="归档阈值" />
        <q-input v-model.number="config.memoryL3.max_per_recall_chars" dense outlined type="number" label="Recall 最大字符" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-title">
          <span class="section-title__text">L4 图谱与进化</span>
        </div>
        <div class="text-caption text-grey-7">长期实体关系、Agent 身份、策略和自我修正提议。</div>
        <q-toggle v-model="config.memoryL4.enabled" color="primary" label="启用 L4" />
      </div>
      <div class="app-form-field-grid">
        <q-toggle v-model="config.memoryL4.graph_inject_neighbors" color="primary" label="注入图邻居" />
        <q-input v-model.number="config.memoryL4.graph_max_neighbors" dense outlined type="number" label="邻居数" />
        <q-input v-model.number="config.memoryL4.graph_max_hops" dense outlined type="number" label="邻居跳数" />
        <q-toggle v-model="config.memoryL4.identity_inject" color="primary" label="注入身份" />
        <q-toggle v-model="config.memoryL4.strategy_inject" color="primary" label="注入策略" />
        <q-toggle v-model="config.evolutionSettings.enabled" color="primary" label="启用自我演化" />
        <q-toggle v-model="config.evolutionSettings.auto_apply" color="warning" label="低风险自动应用" />
        <q-input v-model.number="config.evolutionSettings.min_episodes" dense outlined type="number" label="触发 Episode 数" />
        <q-input v-model.number="config.evolutionSettings.min_negative_feedback" dense outlined type="number" label="触发负反馈数" />
        <q-input v-model.number="config.evolutionSettings.throttle_hours" dense outlined type="number" label="节流小时" />
        <q-input v-model.number="config.evolutionSettings.proposal_ttl_days" dense outlined type="number" label="提议过期天数" />
        <q-input v-model.number="config.evolutionSettings.persona_max_chars" dense outlined type="number" label="Persona 最大字符" />
        <q-input v-model.number="config.evolutionSettings.system_prompt_max_appends" dense outlined type="number" label="Prompt 追加段上限" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  config: Record<string, unknown>;
  truncateStrategyOptions: { label: string; value: string }[];
  snapshotModeOptions: { label: string; value: string }[];
  memoryScopeOptions: { label: string; value: string }[];
}>();
</script>

<style scoped>
/* 通用样式由 agent-settings-page.scss 控制 */
</style>
