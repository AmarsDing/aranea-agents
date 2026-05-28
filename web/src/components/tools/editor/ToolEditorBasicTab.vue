<template>
  <div class="column q-gutter-md">
    <q-banner v-if="!editingId" rounded class="settings-info-banner">
      <template #avatar><q-icon name="lightbulb" color="primary" /></template>
      选择模板可预填 Schema；也可选「空白 Tool」从零开始。
    </q-banner>

    <div v-if="!editingId" class="tool-template-grid">
      <button
        v-for="tpl in templates"
        :key="tpl.id"
        type="button"
        class="tool-template-card"
        :class="{ 'tool-template-card--active': selectedTemplate === tpl.id }"
        @click="$emit('apply-template', tpl.id)"
      >
        <span class="tool-template-card__label">{{ tpl.label }}</span>
        <span class="tool-template-card__caption">{{ tpl.caption }}</span>
      </button>
    </div>

    <div v-if="editingId" class="tool-detail-meta">
      <q-chip dense outline>{{ form.source || "—" }}</q-chip>
      <q-chip dense outline>{{ form.category || "custom" }}</q-chip>
      <q-chip v-if="form.readonly" dense outline>只读</q-chip>
      <q-chip v-if="registryLocked" dense color="warning" text-color="dark">registry 同步</q-chip>
    </div>

    <div class="app-form-field-grid app-form-field-grid--2col">
      <tool-field-hint-input
        :model-value="form.key"
        label="工具标识 (Key)"
        :hint="hints.key"
        :disable="Boolean(editingId)"
        @update:model-value="patch({ key: $event })"
      />
      <tool-field-hint-input
        :model-value="form.display_name"
        label="显示名称"
        :hint="hints.display_name"
        @update:model-value="patch({ display_name: $event })"
      />
      <q-input
        class="app-grid-span-full app-field-long"
        :model-value="form.description"
        dense
        outlined
        autogrow
        type="textarea"
        label="描述"
        :hint="hints.description"
        @update:model-value="patch({ description: $event })"
      />
      <q-input
        :model-value="form.category"
        dense
        outlined
        label="分类"
        :hint="hints.category"
        @update:model-value="patch({ category: $event })"
      />
      <q-select
        v-if="!editingId"
        :model-value="form.source"
        dense
        outlined
        emit-value
        map-options
        label="来源"
        :hint="hints.source"
        :options="sourceOptions"
        @update:model-value="patch({ source: $event })"
      />
      <q-input
        v-else
        :model-value="form.source"
        dense
        outlined
        readonly
        label="来源"
        :hint="hints.source"
      />
      <q-select
        class="app-grid-span-full"
        :model-value="form.risk_level"
        dense
        outlined
        emit-value
        map-options
        label="风险级别"
        :hint="hints.risk_level"
        :options="riskOptions"
        @update:model-value="patch({ risk_level: $event })"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { TOOL_CREATE_TEMPLATES, TOOL_FIELD_HINTS } from "../../../features/tools/toolEditorCopy";
import { patchToolForm } from "../../../features/tools/toolFormPatch";
import type { ToolUpsertInput } from "../../../features/tools/types";
import { sourceSuggestions } from "../toolUi";
import ToolFieldHintInput from "./ToolFieldHintInput.vue";

const props = defineProps<{
  form: ToolUpsertInput;
  editingId: string;
  riskOptions: { label: string; value: string }[];
  registryLocked: boolean;
  selectedTemplate: string;
}>();

defineEmits<{ "apply-template": [id: string] }>();

const hints = TOOL_FIELD_HINTS;
const templates = TOOL_CREATE_TEMPLATES;
const sourceOptions = sourceSuggestions;

function patch(p: Partial<ToolUpsertInput>) {
  patchToolForm(props.form, p);
}
</script>
