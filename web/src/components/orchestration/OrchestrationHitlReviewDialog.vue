<template>
  <AppDialog
    :model-value="open"
    title="人工审核 (HITL)"
    :subtitle="`节点 ${node?.node_id || '—'} · ${node?.agent_name || node?.role || 'agent'}`"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="onDialogUpdate"
  >
    <div class="q-gutter-md">
      <q-banner dense rounded class="bg-warning text-dark">
        {{ node?.error_message || node?.output_preview || '该节点等待人工审核后继续执行。' }}
      </q-banner>
      <div v-if="node?.input_preview" class="text-caption">
        <div class="text-weight-medium q-mb-xs">输入预览</div>
        <JsonCodeViewer :text="node.input_preview" :show-toolbar="false" scroll-height="200px" />
      </div>
      <q-expansion-item dense label="高级：恢复 JSON" icon="code">
        <q-input
          :model-value="advancedJson"
          dense
          outlined
          autogrow
          type="textarea"
          class="app-field-long"
          label="resume_value (JSON)"
          hint='默认 {"action":"review_continue"}'
          @update:model-value="$emit('update:advancedJson', String($event ?? ''))"
        />
      </q-expansion-item>
    </div>
    <template #actions>
      <q-btn flat rounded label="跳过此节点" :disable="loading" @click="$emit('reject', 'skip')" />
      <q-btn flat rounded color="negative" label="终止运行" :disable="loading" @click="confirmHalt" />
      <q-btn flat rounded color="warning" label="切 fallback" :disable="loading" @click="$emit('fallback')" />
      <q-btn color="primary" rounded unelevated label="批准继续" :loading="loading" @click="$emit('approve')" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { confirm } from '../../composables/useAppConfirm';
import AppDialog from '../common/AppDialog.vue';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';
import type { AgentNodeState } from '../../features/orchestration/types';

defineProps<{
  open: boolean;
  node: AgentNodeState | null;
  advancedJson: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  approve: [];
  reject: [action: string];
  fallback: [];
  'update:advancedJson': [value: string];
  'update:open': [value: boolean];
}>();

async function confirmHalt() {
  const ok = await confirm({
    title: '终止运行',
    message: '确定要终止整个运行吗？此操作不可撤销。',
    cancelLabel: '取消',
    confirmLabel: '终止运行',
    tone: 'danger',
  });
  if (!ok) return;
  emit('reject', 'halt');
}

function onDialogUpdate(value: boolean) {
  if (!value) emit('update:open', false);
}
</script>
