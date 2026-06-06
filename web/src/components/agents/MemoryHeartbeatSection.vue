<template>
  <section class="settings-section" :class="{ 'settings-section--disabled': memoryLayersDisabled }">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <span class="section-title__text">心跳</span>
        </div>
        <p class="settings-section__hint">定时注入 HEARTBEAT.MD 检查清单，驱动 Agent 主动巡检。</p>
      </div>
      <q-toggle v-model="config.heartbeat.enabled" label="启用心跳" :disable="memoryLayersDisabled" />
    </div>
    <div class="app-form-field-grid app-form-field-grid--2col">
      <q-input
        v-model.number="config.heartbeat.interval_minutes"
        dense
        outlined
        type="number"
        suffix="min"
        label="间隔"
        :disable="memoryLayersDisabled || !config.heartbeat.enabled"
      />
      <!-- PGO-1: HEARTBEAT.md is deprecated; heartbeat content now lives in
           AGENTS_TASK.md. This section retains the interval toggle for
           backward-compatible agents; the inline body editor is removed. -->
      <q-banner
        v-if="!memoryLayersDisabled && config.heartbeat.enabled"
        dense
        rounded
        class="app-grid-span-full q-mt-xs memory-heartbeat-banner"
      >
        <q-icon name="info" color="warning" class="q-mr-sm" />
        HEARTBEAT.md 已在 PGO-1 中弃用，心跳检查清单请移至 AGENTS_TASK.md。
      </q-banner>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';

defineProps<{
  config: AgentRuntimeConfigForm;
  memoryLayersDisabled: boolean;
}>();
</script>

<style scoped>
.memory-heartbeat-banner {
  background: color-mix(in srgb, var(--q-warning) 10%, transparent);
}
</style>
