<template>
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
.settings-grid {
  display: grid;
  gap: 18px;
}
.settings-section {
  padding: 20px;
  border: 1px solid var(--glass-border);
  border-radius: 24px;
  background: var(--glass-surface);
}
.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 14px;
}
</style>
