<template>
  <q-dialog
    :model-value="open"
    position="right"
    no-shake
    class="tool-help-drawer-dialog"
    @update:model-value="$emit('update:open', $event)"
    @hide="$emit('update:open', false)"
  >
    <q-card class="app-dialog-card tool-help-drawer column">
      <q-card-section class="tool-help-drawer__head row items-center justify-between no-wrap">
        <div class="tool-help-drawer__title">Tool 编辑帮助</div>
        <q-btn flat dense round icon="close" class="app-registry-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>

      <q-tabs
        v-model="activeTab"
        dense
        narrow-indicator
        inline-label
        class="tool-help-drawer__tabs"
        active-color="var(--color-accent)"
        indicator-color="var(--color-accent)"
      >
        <q-tab name="fields" label="字段速查" no-caps />
        <q-tab name="concepts" label="概念说明" no-caps />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="activeTab" class="tool-help-drawer__panels col">
        <q-tab-panel name="fields" class="tool-help-drawer__panel scroll">
          <div class="tool-help-drawer__field-grid">
            <div v-for="entry in fieldEntries" :key="entry.key" class="tool-help-drawer__field-card">
              <div class="tool-help-drawer__field-header">
                <span class="tool-help-drawer__field-label">{{ entry.label }}</span>
                <span class="tool-help-drawer__field-key">{{ entry.key }}</span>
              </div>
              <div class="tool-help-drawer__field-hint">{{ fieldHints[entry.key] }}</div>
            </div>
          </div>
        </q-tab-panel>

        <q-tab-panel name="concepts" class="tool-help-drawer__panel scroll">
          <q-list class="tool-help-drawer__accordion">
            <q-expansion-item
              v-for="(sec, i) in sections"
              :key="i"
              :label="sec.title"
              dense-toggle
              switch-toggle-side
              class="tool-help-drawer__expansion"
            >
              <ul v-if="sec.items?.length" class="tool-help-drawer__list">
                <li v-for="(item, j) in sec.items" :key="j">{{ item }}</li>
              </ul>
              <p v-if="sec.body" class="tool-help-drawer__text">{{ sec.body }}</p>
              <pre v-if="sec.code" class="tool-help-drawer__code">{{ sec.code }}</pre>
            </q-expansion-item>
          </q-list>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { TOOL_FIELD_HINT_ENTRIES, TOOL_FIELD_HINTS, TOOL_HELP_SECTIONS } from '../../../features/tools/toolEditorCopy';

defineProps<{ open: boolean }>();
defineEmits<{ 'update:open': [value: boolean] }>();

const activeTab = ref<'fields' | 'concepts'>('fields');
const sections = TOOL_HELP_SECTIONS;
const fieldEntries = TOOL_FIELD_HINT_ENTRIES;
const fieldHints = TOOL_FIELD_HINTS;
</script>
