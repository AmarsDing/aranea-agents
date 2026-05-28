<template>
  <q-dialog
    :model-value="open"
    position="right"
    seamless
    no-shake
    @update:model-value="$emit('update:open', $event)"
  >
    <q-card class="tool-help-drawer column">
      <q-card-section class="tool-help-drawer__head row items-center justify-between no-wrap">
        <div class="tool-help-drawer__title">Tool 编辑帮助</div>
        <q-btn flat dense round icon="close" class="app-registry-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="tool-help-drawer__body col scroll column q-gutter-lg">
        <section v-for="(sec, i) in sections" :key="i" class="tool-help-drawer__section">
          <h3 class="tool-help-drawer__section-title">{{ sec.title }}</h3>
          <ul v-if="sec.items?.length" class="tool-help-drawer__list">
            <li v-for="(item, j) in sec.items" :key="j">{{ item }}</li>
          </ul>
          <p v-if="sec.body" class="tool-help-drawer__text">{{ sec.body }}</p>
          <pre v-if="sec.code" class="tool-help-drawer__code">{{ sec.code }}</pre>
        </section>

        <q-separator />

        <section class="tool-help-drawer__section">
          <h3 class="tool-help-drawer__section-title">字段速查</h3>
          <q-list dense bordered class="tool-help-drawer__field-list rounded-borders">
            <q-item v-for="entry in fieldEntries" :key="entry.key">
              <q-item-section>
                <q-item-label class="tool-help-drawer__field-label">{{ entry.label }}</q-item-label>
                <q-item-label caption class="tool-help-drawer__field-key">{{ entry.key }}</q-item-label>
                <q-item-label class="tool-help-drawer__field-hint">{{ fieldHints[entry.key] }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </section>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import {
  TOOL_FIELD_HINT_ENTRIES,
  TOOL_FIELD_HINTS,
  TOOL_HELP_SECTIONS
} from "../../../features/tools/toolEditorCopy";

defineProps<{ open: boolean }>();
defineEmits<{ "update:open": [value: boolean] }>();

const sections = TOOL_HELP_SECTIONS;
const fieldEntries = TOOL_FIELD_HINT_ENTRIES;
const fieldHints = TOOL_FIELD_HINTS;
</script>
