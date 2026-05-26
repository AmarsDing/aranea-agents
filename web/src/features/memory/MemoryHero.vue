// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <AppPageHero
    kicker="Memory Center"
    title="记忆中心"
    subtitle="从上下文、工作记忆、会话事件、长期知识和 Agent 进化五个视角解释 Agent 为什么这样回答。"
  >
    <template #actions>
      <q-select
        :model-value="selectedAgentId"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="Agent"
        :options="agentOptions"
        class="memory-select"
        @update:model-value="$emit('update:selectedAgentId', $event as string | null)"
      />
      <q-btn color="primary" rounded unelevated no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import AppPageHero from "../../components/layout/AppPageHero.vue";

defineProps<{
  selectedAgentId: string | null;
  agentOptions: Array<{ label: string; value: string }>;
  loading: boolean;
}>();

defineEmits<{
  "update:selectedAgentId": [value: string | null];
  refresh: [];
}>();
</script>

<style scoped>
.memory-select {
  min-width: 220px;
}

@media (width <= 800px) {
  .memory-select {
    min-width: 100%;
  }
}
</style>
