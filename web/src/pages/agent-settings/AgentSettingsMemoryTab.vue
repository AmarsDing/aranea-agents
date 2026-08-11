<template>
  <div class="settings-grid settings-grid--wide">
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

    <memory-heartbeat-section v-model:config="config" :memory-layers-disabled="memoryLayersDisabled" />

    <memory-level-section
      v-model:config="config"
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
const config = defineModel<AgentRuntimeConfigForm>('config', { required: true });
import { computed } from 'vue';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import MemoryHeartbeatSection from '../../components/agents/MemoryHeartbeatSection.vue';
import MemoryLevelSection from '../../components/agents/MemoryLevelSection.vue';

defineProps<{
  truncateStrategyOptions: { label: string; value: string }[];
  snapshotModeOptions: { label: string; value: string }[];
  memoryScopeOptions: { label: string; value: string }[];
  piiPolicyOptions: { label: string; value: string }[];
}>();

defineEmits<{
  'open-evolution-tab': [];
}>();

const memoryLayersDisabled = computed(() => !config.value.memory.enabled);
</script>
