<template>
  <section class="memory-hero">
    <div>
      <div class="memory-kicker">Memory Center</div>
      <h1 class="memory-title">记忆中心</h1>
      <p class="memory-subtitle">从上下文、工作记忆、会话事件、长期知识和 Agent 进化五个视角解释 Agent 为什么这样回答。</p>
    </div>
    <div class="row q-gutter-sm items-center">
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
      <q-btn color="primary" rounded unelevated icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
    </div>
  </section>
</template>

<script setup lang="ts">
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
