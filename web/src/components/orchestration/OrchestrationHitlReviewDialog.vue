<template>
  <q-dialog :model-value="open" persistent @update:model-value="onDialogUpdate">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">人工审核 (HITL)</div>
        <div class="app-glass-dialog__subtitle">
          节点 {{ node?.node_id || "—" }} · {{ node?.agent_name || node?.role || "agent" }}
        </div>
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
        <q-banner dense rounded class="bg-warning text-dark">
          {{ node?.error_message || node?.output_preview || "该节点等待人工审核后继续执行。" }}
        </q-banner>
        <div v-if="node?.input_preview" class="text-caption">
          <div class="text-weight-medium q-mb-xs">输入预览</div>
          <pre class="orch-hitl-dialog__preview">{{ node.input_preview }}</pre>
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
      </q-card-section>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat rounded label="拒绝 / 终止" :disable="loading" @click="$emit('reject')" />
        <q-btn flat rounded color="warning" label="切 fallback" :disable="loading" @click="$emit('fallback')" />
        <q-btn color="primary" rounded unelevated label="批准继续" :loading="loading" @click="$emit('approve')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { AgentNodeState } from "../../features/orchestration/types";

defineProps<{
  open: boolean;
  node: AgentNodeState | null;
  advancedJson: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  approve: [];
  reject: [];
  fallback: [];
  "update:advancedJson": [value: string];
  "update:open": [value: boolean];
}>();

function onDialogUpdate(value: boolean) {
  if (!value) emit("update:open", false);
}
</script>
