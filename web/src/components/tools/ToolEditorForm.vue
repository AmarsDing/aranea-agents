<template>
  <div class="tool-editor-form">
    <div class="tool-editor-form__toolbar">
      <q-tabs v-model="activeTab" dense class="tool-editor-tabs" active-color="primary" indicator-color="primary" align="left" no-caps>
        <q-tab v-for="t in tabs" :key="t.name" :name="t.name" :label="t.label" />
      </q-tabs>
      <q-btn flat dense round icon="help_outline" class="app-registry-icon-btn" @click="helpOpen = true">
        <q-tooltip>编辑帮助</q-tooltip>
      </q-btn>
    </div>

    <q-tab-panels v-model="activeTab" animated class="tool-editor-panels bg-transparent">
      <q-tab-panel name="basic" class="q-pa-none">
        <tool-editor-basic-tab
          :form="form"
          :editing-id="editingId"
          :risk-options="riskOptions"
          :registry-locked="registryLocked"
          :selected-template="selectedTemplate"
          @apply-template="$emit('apply-template', $event)"
        />
      </q-tab-panel>
      <q-tab-panel name="policy" class="q-pa-none">
        <tool-editor-policy-tab :form="form" :registry-locked="registryLocked" />
      </q-tab-panel>
      <q-tab-panel name="schema" class="q-pa-none">
        <tool-editor-schema-tab :form="form" :errors="errors" :schema-readonly="registryLocked" />
      </q-tab-panel>
      <q-tab-panel name="advanced" class="q-pa-none">
        <tool-editor-advanced-tab :form="form" :errors="errors" :registry-locked="registryLocked" />
      </q-tab-panel>
    </q-tab-panels>

    <tool-editor-help-drawer v-model:open="helpOpen" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { isRegistryLockedTool, TOOL_EDITOR_TABS } from "../../features/tools/toolEditorCopy";
import type { ToolUpsertInput } from "../../features/tools/types";
import ToolEditorAdvancedTab from "./editor/ToolEditorAdvancedTab.vue";
import ToolEditorBasicTab from "./editor/ToolEditorBasicTab.vue";
import ToolEditorHelpDrawer from "./editor/ToolEditorHelpDrawer.vue";
import ToolEditorPolicyTab from "./editor/ToolEditorPolicyTab.vue";
import ToolEditorSchemaTab from "./editor/ToolEditorSchemaTab.vue";

const props = defineProps<{
  form: ToolUpsertInput;
  editingId: string;
  errors: Record<string, string>;
  riskOptions: { label: string; value: string }[];
  selectedTemplate: string;
  activeTab: string;
}>();

const emit = defineEmits<{ "apply-template": [id: string]; "update:activeTab": [value: string] }>();

const helpOpen = ref(false);
const tabs = TOOL_EDITOR_TABS;

const activeTab = computed({
  get: () => props.activeTab,
  set: (v: string) => emit("update:activeTab", v)
});

const registryLocked = computed(() => isRegistryLockedTool(props.form));
</script>
