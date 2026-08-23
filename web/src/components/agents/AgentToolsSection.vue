<template>
  <div class="tool-overrides">
    <div class="tool-overrides__toolbar">
      <q-input
        v-model="search"
        dense
        outlined
        clearable
        :placeholder="$t('agentSettings.toolOverrideSearchPlaceholder')"
        class="tool-overrides__search"
      >
        <template #prepend>
          <q-icon name="search" />
        </template>
      </q-input>
      <q-select
        v-model="groupFilter"
        dense
        outlined
        emit-value
        map-options
        :options="groupFilterOptions"
        :label="$t('toolsPage.agentTools.filterGroup')"
        class="tool-overrides__filter"
      />
      <q-select
        v-model="stateFilter"
        dense
        outlined
        emit-value
        map-options
        :options="stateFilterOptions"
        :label="$t('toolsPage.agentTools.filterState')"
        class="tool-overrides__filter"
      />
      <q-toggle v-model="onlyOverridden" dense :label="$t('toolsPage.agentTools.onlyOverridden')" />
      <q-space />
      <span v-if="overriddenCount" class="tool-overrides__count">
        {{ $t('toolsPage.agentTools.overriddenBadge', { count: overriddenCount }) }}
      </span>
      <q-btn flat dense no-caps icon="refresh" :label="$t('common.refresh')" :loading="loading" @click="reload()" />
    </div>

    <q-banner v-if="!toolsEnabled" rounded class="settings-warning-banner">
      {{ $t('toolsPage.agentTools.masterOff') }}
    </q-banner>

    <div v-if="loading" class="tool-overrides__loading">
      <q-spinner-dots size="28px" />
    </div>
    <div v-else-if="!groupedRows.length" class="tool-overrides__empty">
      {{ $t('toolsPage.agentTools.empty') }}
    </div>

    <div v-else class="tool-overrides__groups">
      <section v-for="group in groupedRows" :key="group.category" class="tool-group">
        <button type="button" class="tool-group__head" @click="toggleGroup(group.category)">
          <q-icon :name="collapsed.has(group.category) ? 'chevron_right' : 'expand_more'" size="18px" />
          <span class="tool-group__name">{{ group.category }}</span>
          <span class="tool-group__meta">
            {{ $t('toolsPage.agentTools.groupSummary', { count: group.rows.length }) }}
          </span>
          <span v-if="group.overriddenCount" class="tool-group__overridden">
            {{ $t('toolsPage.agentTools.overriddenBadge', { count: group.overriddenCount }) }}
          </span>
        </button>
        <ul v-show="!collapsed.has(group.category)" class="tool-group__list">
          <li
            v-for="row in group.rows"
            :key="row.tool_key"
            class="tool-row"
            :class="{ 'tool-row--overridden': Boolean(row.override) }"
          >
            <span
              class="tool-row__state"
              :class="row.effective_state === 'allowed' ? 'is-allowed' : 'is-denied'"
              :title="row.reason || effectiveStateLabel(row.effective_state)"
            />
            <div class="tool-row__main">
              <span class="tool-row__key" :title="row.tool_key">{{ row.tool_key }}</span>
              <span class="tool-row__name" :title="row.display_name">{{ row.display_name }}</span>
            </div>
            <div class="tool-row__controls">
              <q-spinner v-if="pendingKeys.has(row.tool_key)" size="18px" color="primary" />
              <template v-else>
                <q-btn-toggle
                  :model-value="row.override?.mode ?? 'inherit'"
                  dense
                  no-caps
                  unelevated
                  class="tool-row__mode"
                  :options="modeSegOptions"
                  @update:model-value="quickSetMode(row, String($event))"
                />
                <q-btn
                  v-if="row.catalog_requires_confirmation"
                  flat
                  dense
                  round
                  size="sm"
                  icon="lock"
                  color="warning"
                  :aria-label="$t('toolsPage.agentTools.confirmLockedHint')"
                >
                  <q-tooltip>{{ $t('toolsPage.agentTools.confirmLockedHint') }}</q-tooltip>
                </q-btn>
                <q-btn
                  v-else
                  flat
                  dense
                  round
                  size="sm"
                  icon="warning"
                  :color="row.effective_requires_confirmation ? 'warning' : undefined"
                  :class="{ 'tool-row__confirm--off': !row.effective_requires_confirmation }"
                  :aria-label="$t('toolsPage.agentTools.requiresConfirmation')"
                  @click="quickToggleConfirm(row)"
                >
                  <q-tooltip>{{ $t('toolsPage.agentTools.requiresConfirmation') }}</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  dense
                  round
                  size="sm"
                  icon="tune"
                  :aria-label="row.override ? $t('toolsPage.agentTools.editOverride') : $t('toolsPage.agentTools.addOverride')"
                  @click="openEditor(row)"
                >
                  <q-tooltip>{{
                    row.override ? $t('toolsPage.agentTools.editOverride') : $t('toolsPage.agentTools.addOverride')
                  }}</q-tooltip>
                </q-btn>
              </template>
            </div>
          </li>
        </ul>
      </section>
    </div>

    <agent-tool-override-editor-dialog
      :open="editorOpen"
      :editing="Boolean(editingRow?.override)"
      :saving="saving"
      :row="editingRow"
      :form="form"
      @update:open="editorOpen = $event"
      @update:form="form = $event"
      @save="saveOverride()"
      @remove="onEditorRemove()"
    />

    <q-dialog :model-value="confirmRemoveOpen" persistent @update:model-value="cancelRemoveOverride()">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">{{ $t('toolsPage.agentTools.removeTitle') }}</div>
          <div class="text-body2 text-grey-7 q-mt-sm">
            {{ $t('toolsPage.agentTools.removeMessage', { key: pendingRemoveRow?.tool_key }) }}
          </div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps :label="$t('common.cancel')" @click="cancelRemoveOverride()" />
          <q-btn color="negative" rounded unelevated no-caps :label="$t('common.delete')" @click="confirmRemoveOverride()" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
// Container: Agent 设置「工具覆盖」分区；编排 useAgentToolOverrides，行内快捷操作点击即存。
import { computed, ref, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import AgentToolOverrideEditorDialog from './AgentToolOverrideEditorDialog.vue';
import { useAgentToolOverrides } from '../../features/agents/useAgentToolOverrides';

const props = defineProps<{
  agentId: string;
}>();

const { t } = useI18n();

const {
  loading,
  saving,
  toolsEnabled,
  search,
  stateFilter,
  groupFilter,
  onlyOverridden,
  groupedRows,
  groupOptions,
  overriddenCount,
  pendingKeys,
  editorOpen,
  editingRow,
  confirmRemoveOpen,
  pendingRemoveRow,
  form,
  effectiveStateLabel,
  reload,
  openEditor,
  saveOverride,
  quickSetMode,
  quickToggleConfirm,
  requestRemoveOverride,
  confirmRemoveOverride,
  cancelRemoveOverride,
} = useAgentToolOverrides(toRef(props, 'agentId'));

/** 分组折叠为纯 UI 状态，保留在容器本地。 */
const collapsed = ref<Set<string>>(new Set());

function toggleGroup(category: string) {
  const next = new Set(collapsed.value);
  if (next.has(category)) next.delete(category);
  else next.add(category);
  collapsed.value = next;
}

const modeSegOptions = computed(() => [
  { label: t('toolsPage.overrideMode.shortInherit'), value: 'inherit' },
  { label: t('toolsPage.overrideMode.shortAllow'), value: 'allow' },
  { label: t('toolsPage.overrideMode.shortDeny'), value: 'deny' },
]);

const stateFilterOptions = computed(() => [
  { label: t('toolsPage.agentTools.stateAll'), value: '' },
  { label: t('toolsPage.agentTools.stateAllowed'), value: 'allowed' },
  { label: t('toolsPage.agentTools.stateDenied'), value: 'denied' },
]);

const groupFilterOptions = computed(() => [
  { label: t('toolsPage.agentTools.stateAll'), value: '' },
  ...groupOptions.value,
]);

function onEditorRemove() {
  const row = editingRow.value;
  editorOpen.value = false;
  if (row) requestRemoveOverride(row);
}
</script>
