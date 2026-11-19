<template>
  <AppDialog
    :model-value="open"
    position="right"
    size="sm"
    :title="$t('toolsPage.helpDrawer.title')"
    @update:model-value="$emit('update:open', $event)"
  >
    <q-tabs
      v-model="activeTab"
      dense
      narrow-indicator
      inline-label
      class="tool-help-drawer__tabs"
      active-color="var(--color-accent)"
      indicator-color="var(--color-accent)"
    >
      <q-tab name="fields" :label="$t('toolsPage.helpDrawer.tabFields')" no-caps />
      <q-tab name="concepts" :label="$t('toolsPage.helpDrawer.tabConcepts')" no-caps />
    </q-tabs>
    <q-separator />

    <q-tab-panels v-model="activeTab" class="tool-help-drawer__panels">
      <q-tab-panel name="fields" class="tool-help-drawer__panel">
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

      <q-tab-panel name="concepts" class="tool-help-drawer__panel">
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
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import AppDialog from '../../common/AppDialog.vue';
import { toolFieldHintEntries, toolFieldHints, toolHelpSections } from '../../../features/tools/toolEditorCopy';

defineProps<{ open: boolean }>();
defineEmits<{ 'update:open': [value: boolean] }>();

const activeTab = ref<'fields' | 'concepts'>('fields');
const sections = computed(() => toolHelpSections());
const fieldEntries = computed(() => toolFieldHintEntries());
const fieldHints = computed(() => toolFieldHints());
</script>
