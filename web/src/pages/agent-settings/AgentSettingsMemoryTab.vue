<template>
  <div class="settings-grid">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">记忆总开关</span>
          </div>
          <p class="settings-section__hint">统一控制上下文、工作记忆、会话事件、语义知识与进化记忆。</p>
        </div>
        <q-toggle v-model="config.memory.enabled" label="启用记忆" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">心跳</span>
          </div>
          <p class="settings-section__hint">定时注入 HEARTBEAT.MD 检查清单，驱动 Agent 主动巡检。</p>
        </div>
        <q-toggle v-model="config.heartbeat.enabled" label="启用心跳" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input
          v-model.number="config.heartbeat.interval_minutes"
          dense
          outlined
          type="number"
          suffix="min"
          label="间隔"
          :disable="!config.heartbeat.enabled"
        />
        <q-input
          v-model="heartbeatFile.body"
          class="app-grid-span-full app-field-long"
          dense
          outlined
          autogrow
          type="textarea"
          label="检查清单 (HEARTBEAT.MD)"
          :disable="!config.heartbeat.enabled"
        />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">L0 上下文窗口</span>
          </div>
          <p class="settings-section__hint">控制下一次 prompt 如何装配、摘要和记录快照。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model.number="config.memoryL0.recent_window_turns" dense outlined type="number" label="最近窗口轮数" />
        <q-input v-model.number="config.memoryL0.recent_window_tokens" dense outlined type="number" label="最近窗口 Tokens" hint="0 = 自动" />
        <q-input v-model.number="config.memoryL0.summary_threshold" dense outlined type="number" step="0.05" label="摘要触发阈值" />
        <q-input v-model.number="config.memoryL0.summary_keep_turns" dense outlined type="number" label="摘要保留轮数" />
        <q-select v-model="config.memoryL0.truncate_strategy" dense outlined emit-value map-options label="裁剪策略" :options="truncateStrategyOptions" />
        <q-select v-model="config.memoryL0.snapshot_mode" dense outlined emit-value map-options label="快照模式" :options="snapshotModeOptions" />
        <q-input v-model.number="config.memoryL0.l3_max_chunks" dense outlined type="number" label="L3 最大片段" />
        <q-input v-model.number="config.memoryL0.l4_max_paths" dense outlined type="number" label="L4 最大路径" />
        <q-toggle v-model="config.memoryL0.inject_l1" label="注入 L1" />
        <q-toggle v-model="config.memoryL0.inject_l3" label="注入 L3" />
        <q-toggle v-model="config.memoryL0.inject_l4" label="注入 L4" />
        <q-input v-model="config.memoryL0.compress_provider" dense outlined label="L0 摘要 Provider" hint="留空 → 使用会话/Agent 聊天模型" />
        <q-input v-model="config.memoryL0.compress_model" dense outlined label="L0 摘要 Model" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">巩固 Worker 模型</span>
          </div>
          <p class="settings-section__hint">Turn 完成后 AutoMemory LLM 提取所用模型；留空时依次回退 L0 摘要模型、聊天模型。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="config.memoryWorker.provider" dense outlined label="Worker Provider" placeholder="openai" />
        <q-input v-model="config.memoryWorker.model" dense outlined label="Worker Model" placeholder="gpt-4o-mini" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">L1 工作记忆</span>
          </div>
          <p class="settings-section__hint">当前任务的结构化状态、版本和预算。</p>
        </div>
        <q-toggle v-model="config.memoryL1.enabled" label="启用 L1" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model.number="config.memoryL1.budget_tokens" dense outlined type="number" label="任务预算 Tokens" />
        <q-input v-model.number="config.memoryL1.field_max_tokens" dense outlined type="number" label="单字段上限" />
        <q-input v-model.number="config.memoryL1.history_keep_revisions" dense outlined type="number" label="版本保留数" />
        <q-input v-model.number="config.memoryL1.archive_on_idle_minutes" dense outlined type="number" label="闲置归档分钟" />
        <q-input v-model="config.memoryL1.default_schema_id" class="app-grid-span-full app-field-long" dense outlined label="默认 Schema ID" placeholder="schema_xxx" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">L2 会话事件</span>
          </div>
          <p class="settings-section__hint">控制 episode、索引、回忆注入和保留周期。</p>
        </div>
        <q-toggle v-model="config.memoryL2.episode_enabled" label="启用 Episode" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model.number="config.memoryL2.episode_min_importance" dense outlined type="number" step="0.05" label="最低重要性" />
        <q-toggle v-model="config.memoryL2.index_enabled" label="启用索引" />
        <q-toggle v-model="config.memoryL2.recall_enabled" label="允许 L2 Recall" />
        <q-input v-model.number="config.memoryL2.recall_max" dense outlined type="number" label="Recall 最大数" />
        <q-input v-model="config.memoryL2.index_embedding_model" dense outlined label="Embedding 模型" />
        <q-input v-model.number="config.memoryL2.retention_days" dense outlined type="number" label="保留天数" />
        <q-input v-model.number="config.memoryL2.archive_after_days" dense outlined type="number" label="归档天数" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">L3 语义知识</span>
          </div>
          <p class="settings-section__hint">跨会话事实、偏好、规则和冲突治理。</p>
        </div>
        <q-toggle v-model="config.memoryL3.enabled" label="启用 L3" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model.number="config.memoryL3.recall_top_k" dense outlined type="number" label="Recall TopK" />
        <q-input v-model.number="config.memoryL3.recall_min_score" dense outlined type="number" step="0.05" label="最小分数" />
        <q-select v-model="config.memoryL3.recall_scopes" class="app-grid-span-full app-field-long" dense outlined multiple use-chips emit-value map-options label="检索作用域" :options="memoryScopeOptions" />
        <q-input v-model="config.memoryL3.embedding_model" dense outlined label="Embedding 模型" />
        <q-input v-model.number="config.memoryL3.decay_interval_hours" dense outlined type="number" label="衰减间隔小时" />
        <q-input v-model.number="config.memoryL3.archive_threshold" dense outlined type="number" step="0.05" label="归档阈值" />
        <q-input v-model.number="config.memoryL3.max_per_recall_chars" dense outlined type="number" label="Recall 最大字符" />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">L4 图谱与进化</span>
          </div>
          <p class="settings-section__hint">长期实体关系、Agent 身份、策略和自我修正提议。</p>
        </div>
        <q-toggle v-model="config.memoryL4.enabled" label="启用 L4" />
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-toggle v-model="config.memoryL4.graph_inject_neighbors" label="注入图邻居" />
        <q-input v-model.number="config.memoryL4.graph_max_neighbors" dense outlined type="number" label="邻居数" />
        <q-input v-model.number="config.memoryL4.graph_max_hops" dense outlined type="number" label="邻居跳数" />
        <q-toggle v-model="config.memoryL4.identity_inject" label="注入身份" />
        <q-toggle v-model="config.memoryL4.strategy_inject" label="注入策略" />
        <q-toggle v-model="config.evolutionSettings.enabled" label="启用自我演化" />
        <q-toggle v-model="config.evolutionSettings.auto_apply" label="低风险自动应用" />
        <q-input v-model.number="config.evolutionSettings.min_episodes" dense outlined type="number" label="触发 Episode 数" />
        <q-input v-model.number="config.evolutionSettings.min_negative_feedback" dense outlined type="number" label="触发负反馈数" />
        <q-input v-model.number="config.evolutionSettings.throttle_hours" dense outlined type="number" label="节流小时" />
        <q-input v-model.number="config.evolutionSettings.proposal_ttl_days" dense outlined type="number" label="提议过期天数" />
        <q-input v-model.number="config.evolutionSettings.persona_max_chars" dense outlined type="number" label="Persona 最大字符" />
        <q-input v-model.number="config.evolutionSettings.system_prompt_max_appends" dense outlined type="number" label="Prompt 追加段上限" />
      </div>
    </section>

    <section class="settings-section settings-section--muted">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">兼容参数</span>
          </div>
          <p class="settings-section__hint">旧版 memory 检索字段，新配置优先使用上方 L0–L4 分层项。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model.number="config.memory.max_results" dense outlined type="number" label="最大结果数" />
        <q-input v-model.number="config.memory.min_score" dense outlined type="number" step="0.01" label="最低分数" />
        <q-input v-model.number="config.memory.max_chunk_length" dense outlined type="number" label="最大块长度" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import type { AgentFile } from "../../components/agents/agentUi";
import type { AgentRuntimeConfigForm } from "../../features/agents/agentRuntimeConfig";

withDefaults(
  defineProps<{
    config: AgentRuntimeConfigForm;
    truncateStrategyOptions: { label: string; value: string }[];
    snapshotModeOptions: { label: string; value: string }[];
    memoryScopeOptions: { label: string; value: string }[];
    heartbeatFile?: AgentFile;
  }>(),
  {
    heartbeatFile: () => ({ name: "HEARTBEAT.MD", caption: "", body: "" }),
  },
);
</script>
