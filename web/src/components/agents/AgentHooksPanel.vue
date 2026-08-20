<template>
  <div class="agent-hooks-panel">
    <div class="row items-center justify-between q-mb-md">
      <!-- eslint-disable-next-line vue/no-v-html -- trusted i18n HTML hint -->
      <p class="agent-hooks-panel__hint q-ma-none" v-html="t('hooksPage.agentPanel.hint')" />
      <q-btn
        flat
        rounded
        dense
        no-caps
        icon="open_in_new"
        :label="t('hooksPage.agentPanel.btnGlobalHooks')"
        :to="{ name: 'hooks' }"
      />
    </div>

    <q-banner v-if="loadError" rounded class="app-page-error-banner q-mb-md">
      {{ loadError }}
      <template #action>
        <q-btn flat dense :label="t('hooksPage.agentPanel.retry')" class="text-white" @click="reload" />
      </template>
    </q-banner>

    <HooksTable
      variant="agent"
      :rows="scopedRows"
      :loading="loading"
      :toggling-id="togglingId"
      @edit="openEdit"
      @toggle-enabled="toggleEnabled"
      @remove="confirmRemove"
    />

    <q-expansion-item
      v-model="editorExpanded"
      dense-toggle
      icon="add"
      :label="t('hooksPage.agentPanel.expansionCreate')"
      default-opened
    >
      <div class="app-dialog-section agent-hooks-panel__create q-pa-md q-mt-sm">
        <div class="app-form-field-grid app-form-field-grid--2col q-mb-md">
          <q-input
            v-model="draftName"
            dense
            outlined
            clearable
            :label="t('hooksPage.fieldName')"
            :hint="t('hooksPage.agentPanel.nameHint')"
          />
          <q-toggle v-model="draftEnabled" :label="t('hooksPage.fieldEnabled')" />
        </div>
        <callback-editor
          v-model="draftRule"
          v-model:sort-order="draftSort"
          v-model:valid="draftValid"
          :agent-id="agentId"
          :agent-key="agentKey"
          lock-agent-id
          :tool-options="toolOptions"
          :loading-tool-options="loadingToolOptions"
          :enabled-tool-keys="enabledToolKeys"
        />
        <div class="row justify-end q-mt-md">
          <q-btn
            color="primary"
            unelevated
            :label="t('hooksPage.agentPanel.btnCreate')"
            :loading="saving"
            :disable="!draftValid"
            @click="createScopedHook"
          />
        </div>
      </div>
    </q-expansion-item>

    <template v-if="globalRows.length">
      <div class="text-subtitle2 q-mt-lg q-mb-xs">{{ t('hooksPage.agentPanel.globalSectionTitle') }}</div>
      <p class="text-caption text-grey q-mb-sm">{{ t('hooksPage.agentPanel.globalSectionCaption') }}</p>
      <HooksTable variant="agent" readonly :rows="globalRows" :loading="loading" />
    </template>

    <q-dialog v-model="editOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('hooksPage.agentPanel.dialogTitleEdit') }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <div class="app-form-field-grid app-form-field-grid--2col q-mb-md">
              <q-input v-model="editName" dense outlined :label="t('hooksPage.fieldName')" />
              <q-toggle v-model="editEnabled" :label="t('hooksPage.fieldEnabled')" />
            </div>
            <callback-editor
              v-if="editRow"
              v-model="editRule"
              v-model:sort-order="editSort"
              v-model:valid="editValid"
              :agent-id="agentId"
              :agent-key="agentKey"
              lock-agent-id
              :tool-options="toolOptions"
              :loading-tool-options="loadingToolOptions"
              :enabled-tool-keys="enabledToolKeys"
            />
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat no-caps :label="t('hooksPage.btnCancel')" />
          <q-btn
            color="primary"
            unelevated
            no-caps
            :label="t('hooksPage.btnSave')"
            :loading="saving"
            :disable="!editValid"
            @click="saveEdit"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import CallbackEditor from '../hooks/CallbackEditor.vue';
import HooksTable from '../hooks/HooksTable.vue';
import { useAgentHooksPanel } from '../../features/agents/useAgentHooksPanel';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    agentId: string;
    agentKey: string;
    toolOptions?: { label: string; value: string }[];
    loadingToolOptions?: boolean;
  }>(),
  {
    toolOptions: undefined,
    loadingToolOptions: false,
  },
);

const {
  loading,
  saving,
  loadError,
  scopedRows,
  globalRows,
  editorExpanded,
  draftRule,
  draftSort,
  draftName,
  draftEnabled,
  editOpen,
  editRow,
  editRule,
  editSort,
  editName,
  editEnabled,
  togglingId,
  draftValid,
  editValid,
  enabledToolKeys,
  createScopedHook,
  openEdit,
  saveEdit,
  toggleEnabled,
  confirmRemove,
  reload,
} = useAgentHooksPanel(
  () => props.agentId,
  () => props.agentKey,
);
</script>
