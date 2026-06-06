<template>
  <div class="settings-grid">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">记忆总开关</span>
          </div>
          <p class="settings-section__hint">关闭后禁用 Memory 工具、L0–L4 Prompt 注入与 Turn 后自动巩固。</p>
        </div>
        <q-toggle v-model="config.memory.enabled" label="启用记忆" />
      </div>
    </section>

    <memory-heartbeat-section :config="config" :memory-layers-disabled="memoryLayersDisabled" />

    <memory-optional-files-section
      :available-optional-files="availableOptionalFiles"
      :disabled="memoryLayersDisabled || !config.heartbeat.enabled"
      @add-optional-file="$emit('add-optional-file', $event)"
    />

    <memory-level-section
      :config="config"
      :truncate-strategy-options="truncateStrategyOptions"
      :snapshot-mode-options="snapshotModeOptions"
      :memory-scope-options="memoryScopeOptions"
      :pii-policy-options="piiPolicyOptions"
      :memory-layers-disabled="memoryLayersDisabled"
      @open-evolution-tab="$emit('open-evolution-tab')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { AgentFile } from '../../components/agents/agentUi';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import MemoryHeartbeatSection from '../../components/agents/MemoryHeartbeatSection.vue';
import MemoryOptionalFilesSection from '../../components/agents/MemoryOptionalFilesSection.vue';
import MemoryLevelSection from '../../components/agents/MemoryLevelSection.vue';

const props = withDefaults(
  defineProps<{
    config: AgentRuntimeConfigForm;
    truncateStrategyOptions: { label: string; value: string }[];
    snapshotModeOptions: { label: string; value: string }[];
    memoryScopeOptions: { label: string; value: string }[];
    piiPolicyOptions: { label: string; value: string }[];
    availableOptionalFiles?: AgentFile[];
  }>(),
  {
    availableOptionalFiles: () => [],
  },
);

defineEmits<{
  'open-evolution-tab': [];
  'add-optional-file': [name: string];
}>();

const memoryLayersDisabled = computed(() => !props.config.memory.enabled);
</script>
