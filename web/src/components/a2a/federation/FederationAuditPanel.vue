<template>
  <div>
    <AppPageToolbar dense>
      <q-select
        v-model="calleeOrgId"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :options="calleeFilterOptions"
        :label="t('a2a.federation.auditFilterCallee')"
      />
      <q-select
        v-model="decision"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :options="decisionOptions"
        :label="t('a2a.federation.auditFilterDecision')"
      />
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :options="statusOptions"
        :label="t('a2a.federation.auditFilterStatus')"
      />
      <template #actions>
        <q-btn
          outline
          no-caps
          color="primary"
          icon="search"
          :label="t('a2a.federation.auditSearch')"
          :loading="loading"
          @click="emit('search')"
        />
      </template>
    </AppPageToolbar>

    <AppRegistryTable
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
      :no-data-label="t('a2a.federation.auditEmpty')"
    >
      <template #body-cell-caller="slotProps">
        <q-td :props="props">
          <div class="app-registry-cell-primary">{{ federationOrgDisplay(t, slotProps.row.caller_org_id) }}</div>
          <div class="text-caption text-grey-6">{{ slotProps.row.caller_agent_id || '—' }}</div>
        </q-td>
      </template>
      <template #body-cell-callee="slotProps">
        <q-td :props="props">
          <div class="app-registry-cell-primary">{{ federationOrgDisplay(t, slotProps.row.callee_org_id) }}</div>
          <div class="text-caption text-grey-6">{{ slotProps.row.callee_agent_id || '—' }}</div>
        </q-td>
      </template>
      <template #body-cell-decision="slotProps">
        <q-td :props="props">
          <q-badge
            :color="federationDecisionColor(slotProps.row.decision)"
            :label="federationDecisionLabel(t, slotProps.row.decision)"
          />
        </q-td>
      </template>
      <template #body-cell-status="slotProps">
        <q-td :props="props">
          <q-chip dense :color="federationAuditStatusColor(slotProps.row.status)" text-color="white" size="sm">
            {{ federationAuditStatusLabel(t, slotProps.row.status) }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-error_message="slotProps">
        <q-td :props="props">
          <span class="ellipsis">{{ slotProps.row.error_message || '—' }}</span>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      :page="page"
      :page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :label="t('a2a.federation.auditRowsLabel')"
      @update:page="emit('page-change', $event)"
      @update:page-size="emit('page-size-change', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../../layout/AppPageToolbar.vue';
import AppRegistryTable from '../../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../../layout/AppRegistryPagination.vue';
import type { FederationAuditEntry, FederationOrg } from '../../../features/a2a/federationTypes';
import {
  federationAuditStatusColor,
  federationAuditStatusFilterOptions,
  federationAuditStatusLabel,
  federationDecisionColor,
  federationDecisionFilterOptions,
  federationDecisionLabel,
  federationOrgDisplay,
} from '../../../features/a2a/federationUi';
import type { RegistryTableColumn } from '../../../features/ui/registryTableColumns';

const props = defineProps<{
  rows: FederationAuditEntry[];
  orgs: FederationOrg[];
  total: number;
  loading: boolean;
  columns: RegistryTableColumn<FederationAuditEntry>[];
  page: number;
  pageSize: number;
}>();

const emit = defineEmits<{
  search: [];
  'page-change': [page: number];
  'page-size-change': [pageSize: number];
}>();

const calleeOrgId = defineModel<string>('calleeOrgId', { default: '' });
const decision = defineModel<string>('decision', { default: '' });
const status = defineModel<string>('status', { default: '' });

const { t } = useI18n();

const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, props.total) / props.pageSize)));

const calleeFilterOptions = computed(() => [
  { label: t('a2a.federation.auditFilterAll'), value: '' },
  ...props.orgs.map((o) => ({ label: `${o.name || o.domain} (${o.domain})`, value: o.id })),
]);
const decisionOptions = computed(() => federationDecisionFilterOptions(t));
const statusOptions = computed(() => federationAuditStatusFilterOptions(t));
</script>
