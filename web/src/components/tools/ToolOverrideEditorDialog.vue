<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">{{ editing ? "编辑 Agent 覆盖" : "添加 Agent 覆盖" }}</div>
        <q-btn flat dense round icon="close" class="app-dialog-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="q-gutter-sm">
        <q-select
          :model-value="form.agent_id"
          label="Agent"
          dense
          outlined
          :options="agentOptions"
          emit-value
          map-options
          :disable="editing"
          :loading="agentsLoading"
          @update:model-value="emitFormPatch({ agent_id: String($event ?? '') })"
        />
        <q-select
          :model-value="form.mode"
          label="模式"
          dense
          outlined
          :options="modeOptions"
          emit-value
          map-options
          @update:model-value="emitFormPatch({ mode: String($event ?? 'inherit') })"
        />
        <q-toggle
          :model-value="form.enabled"
          label="启用"
          @update:model-value="emitFormPatch({ enabled: Boolean($event) })"
        />
        <q-toggle
          :model-value="form.requires_confirmation"
          label="需要确认"
          @update:model-value="emitFormPatch({ requires_confirmation: Boolean($event) })"
        />
        <q-input
          :model-value="form.config_override_json"
          label="配置覆盖 JSON"
          type="textarea"
          dense
          outlined
          autogrow
          @update:model-value="emitFormPatch({ config_override_json: String($event ?? '{}') })"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn no-caps unelevated class="app-dialog-accent-btn" label="保存" :loading="saving" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { listAgents } from "../../features/agents/api";
import type { Agent } from "../../features/agents/types";

export type ToolOverrideForm = {
  agent_id: string;
  mode: string;
  enabled: boolean;
  requires_confirmation: boolean;
  config_override_json: string;
};

const props = defineProps<{
  open: boolean;
  form: ToolOverrideForm;
  editing: boolean;
  saving: boolean;
}>();

const emit = defineEmits<{
  "update:open": [value: boolean];
  save: [];
  "update:form": [value: ToolOverrideForm];
}>();

const modeOptions = [
  { label: "继承 (inherit)", value: "inherit" },
  { label: "允许 (allow)", value: "allow" },
  { label: "拒绝 (deny)", value: "deny" }
];

const agentsLoading = ref(false);
const agentOptions = ref<{ label: string; value: string }[]>([]);

onMounted(async () => {
  agentsLoading.value = true;
  try {
    const agents: Agent[] = await listAgents({ limit: 200 });
    agentOptions.value = agents.map((a) => ({
      label: a.display_name || a.agent_key || a.id,
      value: a.id
    }));
  } catch {
    agentOptions.value = [];
  } finally {
    agentsLoading.value = false;
  }
});

function emitFormPatch(patch: Partial<ToolOverrideForm>) {
  emit("update:form", { ...props.form, ...patch });
}
</script>
