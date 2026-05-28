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
        <q-btn flat rounded label="跳过此节点" :disable="loading" @click="$emit('reject', 'skip')" />
        <q-btn flat rounded color="negative" label="终止运行" :disable="loading" @click="confirmHalt" />
        <q-btn flat rounded color="warning" label="切 fallback" :disable="loading" @click="$emit('fallback')" />
        <q-btn color="primary" rounded unelevated label="批准继续" :loading="loading" @click="$emit('approve')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useQuasar } from "quasar";
import type { AgentNodeState } from "../../features/orchestration/types";

const props = defineProps<{
  open: boolean;
  node: AgentNodeState | null;
  advancedJson: string;
  loading: boolean;
}>();

const emit = defineEmits<{
  approve: [];
  reject: [action: string];
  fallback: [];
  "update:advancedJson": [value: string];
  "update:open": [value: boolean];
}>();

const $q = useQuasar();

function confirmHalt() {
  $q.dialog({
    title: "终止运行",
    message: "确定要终止整个运行吗？此操作不可撤销。",
    cancel: { label: "取消", flat: true, noCaps: true },
    ok: { label: "终止运行", noCaps: true, color: "negative" },
    persistent: true,
  }).onOk(() => {
    emit("reject", "halt");
  });
}

function onDialogUpdate(value: boolean) {
  if (!value) emit("update:open", false);
}
</script>
