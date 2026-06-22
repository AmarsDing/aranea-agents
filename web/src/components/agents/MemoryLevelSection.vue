<template>
  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">L0 上下文窗口</span>
        </div>
        <p class="settings-section__hint">控制下一次 prompt 如何装配、摘要和记录快照。</p>
      </div>
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.memoryL0.recent_window_turns"
        dense
        outlined
        type="number"
        label="最近窗口轮数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL0.recent_window_tokens"
        dense
        outlined
        type="number"
        label="最近窗口 Tokens"
        hint="0 = 自动"
        :min="0"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL0.summary_threshold"
        dense
        outlined
        type="number"
        step="0.05"
        label="摘要触发阈值"
        :min="0"
        :max="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL0.summary_keep_turns"
        dense
        outlined
        type="number"
        label="摘要保留轮数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-select
        v-model="config.memoryL0.truncate_strategy"
        dense
        outlined
        emit-value
        map-options
        label="裁剪策略"
        :options="truncateStrategyOptions"
        :disable="memoryLayersDisabled"
      />
      <q-select
        v-model="config.memoryL0.snapshot_mode"
        dense
        outlined
        emit-value
        map-options
        label="快照模式"
        :options="snapshotModeOptions"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL0.l3_max_chunks"
        dense
        outlined
        type="number"
        label="L3 最大片段"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL0.l4_max_paths"
        dense
        outlined
        type="number"
        label="L4 最大路径"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-toggle v-model="config.memoryL0.inject_l1" label="注入 L1" :disable="memoryLayersDisabled" />
      <q-toggle v-model="config.memoryL0.inject_l3" label="注入 L3" :disable="memoryLayersDisabled" />
      <q-toggle v-model="config.memoryL0.inject_l4" label="注入 L4" :disable="memoryLayersDisabled" />
      <q-input
        v-model="config.memoryL0.compress_provider"
        dense
        outlined
        label="L0 摘要 Provider"
        hint="留空 → 使用会话/Agent 聊天模型"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryL0.compress_model"
        dense
        outlined
        label="L0 摘要 Model"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>

  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">巩固 Worker 模型</span>
        </div>
        <p class="settings-section__hint">
          Turn 完成后 AutoMemory LLM 提取所用模型；留空时依次回退 L0 摘要模型、聊天模型。
        </p>
      </div>
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model="config.memoryWorker.provider"
        dense
        outlined
        label="Worker Provider"
        placeholder="openai"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryWorker.model"
        dense
        outlined
        label="Worker Model"
        placeholder="gpt-4o-mini"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>

  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">L1 工作记忆</span>
        </div>
        <p class="settings-section__hint">当前任务的结构化状态、版本和预算。</p>
      </div>
      <q-toggle v-model="config.memoryL1.enabled" label="启用 L1" :disable="memoryLayersDisabled" />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.memoryL1.budget_tokens"
        dense
        outlined
        type="number"
        label="任务预算 Tokens"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL1.field_max_tokens"
        dense
        outlined
        type="number"
        label="单字段上限"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL1.history_keep_revisions"
        dense
        outlined
        type="number"
        label="版本保留数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL1.archive_on_idle_minutes"
        dense
        outlined
        type="number"
        label="闲置归档分钟"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryL1.default_schema_id"
        class="app-grid-span-full app-field-long"
        dense
        outlined
        label="默认 Schema ID"
        placeholder="schema_xxx"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>

  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">L2 会话事件</span>
        </div>
        <p class="settings-section__hint">控制 episode、索引、回忆注入和保留周期。</p>
      </div>
      <q-toggle v-model="config.memoryL2.episode_enabled" label="启用 Episode" :disable="memoryLayersDisabled" />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.memoryL2.episode_min_importance"
        dense
        outlined
        type="number"
        step="0.05"
        label="最低重要性"
        :min="0"
        :max="1"
        :disable="memoryLayersDisabled"
      />
      <q-toggle v-model="config.memoryL2.index_enabled" label="启用索引" :disable="memoryLayersDisabled" />
      <q-toggle v-model="config.memoryL2.recall_enabled" label="允许 L2 Recall" :disable="memoryLayersDisabled" />
      <q-input
        v-model.number="config.memoryL2.recall_max"
        dense
        outlined
        type="number"
        label="Recall 最大数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryL2.index_embedding_model"
        dense
        outlined
        label="Embedding 模型"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL2.retention_days"
        dense
        outlined
        type="number"
        label="保留天数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL2.archive_after_days"
        dense
        outlined
        type="number"
        label="归档天数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>

  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">L3 语义知识</span>
        </div>
        <p class="settings-section__hint">跨会话事实、偏好、规则和冲突治理。</p>
      </div>
      <q-toggle v-model="config.memoryL3.enabled" label="启用 L3" :disable="memoryLayersDisabled" />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.memoryL3.recall_top_k"
        dense
        outlined
        type="number"
        label="Recall TopK"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL3.recall_min_score"
        dense
        outlined
        type="number"
        step="0.05"
        label="最小分数"
        :min="0"
        :max="1"
        :disable="memoryLayersDisabled"
      />
      <q-select
        v-model="config.memoryL3.recall_scopes"
        class="app-grid-span-full app-field-long"
        dense
        outlined
        multiple
        use-chips
        emit-value
        map-options
        label="检索作用域"
        :options="memoryScopeOptions"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryL3.embedding_model"
        dense
        outlined
        label="Embedding 模型"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL3.decay_interval_hours"
        dense
        outlined
        type="number"
        label="衰减间隔小时"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL3.archive_threshold"
        dense
        outlined
        type="number"
        step="0.05"
        label="归档阈值"
        :min="0"
        :max="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL3.max_per_recall_chars"
        dense
        outlined
        type="number"
        label="Recall 最大字符"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-select
        v-model="config.memoryL3.pii_policy"
        dense
        outlined
        emit-value
        map-options
        label="PII 策略"
        :options="piiPolicyOptions"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>

  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">L4 图谱</span>
        </div>
        <p class="settings-section__hint">长期实体关系与 Prompt 注入；自我演化提议阈值见「进化」标签页。</p>
      </div>
      <q-toggle v-model="config.memoryL4.enabled" label="启用 L4" :disable="memoryLayersDisabled" />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-toggle v-model="config.memoryL4.graph_inject_neighbors" label="注入图邻居" :disable="memoryLayersDisabled" />
      <q-input
        v-model.number="config.memoryL4.graph_max_neighbors"
        dense
        outlined
        type="number"
        label="邻居数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memoryL4.graph_max_hops"
        dense
        outlined
        type="number"
        label="邻居跳数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-toggle v-model="config.memoryL4.identity_inject" label="注入身份" :disable="memoryLayersDisabled" />
      <q-toggle v-model="config.memoryL4.strategy_inject" label="注入策略" :disable="memoryLayersDisabled" />
      <q-input
        v-model.number="config.memoryL4.decay_interval_hours"
        dense
        outlined
        type="number"
        label="衰减间隔小时"
        hint="0 = 禁用衰减；默认 168（7天）"
        :min="0"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model="config.memoryL4.decay_overrides_json"
        class="app-grid-span-full app-field-long"
        dense
        outlined
        type="textarea"
        autogrow
        label="衰减覆盖 JSON"
        hint='按实体类型覆盖半衰期，如 {"person": 90, "event": 7}'
        :disable="memoryLayersDisabled"
      />
    </div>
    <q-banner rounded class="q-mt-md settings-info-banner">
      风格进化（SOUL.md）与自动提议流水线请在
      <a href="#" class="settings-link" @click.prevent="$emit('open-evolution-tab')">「进化」</a>
      Tab 配置。
    </q-banner>
  </section>

  <section
    class="settings-section settings-section--muted"
    :class="{ 'settings-section--disabled': memoryLayersDisabled }"
  >
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">兼容参数</span>
        </div>
        <p class="settings-section__hint">旧版 memory 工具检索；与 L3 Recall 并存时以 L3 分层配置为主。</p>
      </div>
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.memory.max_results"
        dense
        outlined
        type="number"
        label="最大结果数"
        :min="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memory.min_score"
        dense
        outlined
        type="number"
        step="0.01"
        label="最低分数"
        :min="0"
        :max="1"
        :disable="memoryLayersDisabled"
      />
      <q-input
        v-model.number="config.memory.max_chunk_length"
        dense
        outlined
        type="number"
        label="最大块长度"
        :min="1"
        :disable="memoryLayersDisabled"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
const config = defineModel<AgentRuntimeConfigForm>('config', { required: true });
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';

defineProps<{
  truncateStrategyOptions: { label: string; value: string }[];
  snapshotModeOptions: { label: string; value: string }[];
  memoryScopeOptions: { label: string; value: string }[];
  piiPolicyOptions: { label: string; value: string }[];
  memoryLayersDisabled: boolean;
}>();

defineEmits<{
  'open-evolution-tab': [];
}>();
</script>
