<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A Endpoint</div>
        <div class="text-caption text-grey-7">将本 Agent 暴露为 A2A 服务，供同工作区 call_agent 或外部客户端调用。</div>
      </div>
    </div>

    <q-inner-loading :showing="loading" />

    <div v-if="card" class="q-gutter-md">
      <q-toggle :model-value="card.enabled" color="primary" label="启用 A2A" @update:model-value="emit('update:cardEnabled', $event)" />
      <div>
        <div class="text-caption text-grey-7 q-mb-sm">Capabilities（JSON 名称列表，每行一个能力名）</div>
        <q-input :model-value="capabilityLines" class="app-field-long" outlined type="textarea" rows="4" hint="例如 chat、summarize" @update:model-value="emit('update:capabilityLines', String($event ?? ''))" />
      </div>
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn color="primary" rounded unelevated no-caps label="保存 AgentCard" :loading="saving" :disable="!card" @click="emit('save')" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { Ref } from "vue";

defineProps<{
  loading: boolean;
  saving: boolean;
  card: { enabled: boolean; [k: string]: unknown } | null;
  capabilityLines: string;
}>();

const emit = defineEmits<{
  save: [];
  'update:cardEnabled': [value: boolean];
  'update:capabilityLines': [value: string];
}>();
</script>
