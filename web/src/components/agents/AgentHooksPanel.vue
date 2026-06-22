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

    <HooksTable variant="agent" :rows="scopedRows" :loading="loading" @edit="openEdit" />

    <q-expansion-item
      v-model="editorExpanded"
      dense-toggle
      icon="add"
      :label="t('hooksPage.agentPanel.expansionCreate')"
      default-opened
    >
      <div class="app-dialog-section q-pa-md q-mt-sm">
        <callback-editor v-model="draftRule" v-model:sort-order="draftSort" :agent-id="agentId" :agent-key="agentKey" />
        <div class="row justify-end q-mt-md">
          <q-btn
            color="primary"
            unelevated
            :label="t('hooksPage.agentPanel.btnCreate')"
            :loading="saving"
            @click="createScopedHook"
          />
        </div>
      </div>
    </q-expansion-item>

    <q-dialog v-model="editOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between">
          <div class="app-glass-dialog__title">{{ t('hooksPage.agentPanel.dialogTitleEdit') }}</div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <callback-editor
              v-if="editRow"
              v-model="editRule"
              v-model:sort-order="editSort"
              :agent-id="agentId"
              :agent-key="agentKey"
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

const props = defineProps<{
  agentId: string;
  agentKey: string;
}>();

const {
  loading,
  saving,
  loadError,
  scopedRows,
  editorExpanded,
  draftRule,
  draftSort,
  editOpen,
  editRow,
  editRule,
  editSort,
  createScopedHook,
  openEdit,
  saveEdit,
  reload,
} = useAgentHooksPanel(
  () => props.agentId,
  () => props.agentKey,
);
</script>
