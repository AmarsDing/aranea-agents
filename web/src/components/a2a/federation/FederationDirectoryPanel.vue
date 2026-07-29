<template>
  <div>
    <AppPageToolbar dense>
      <q-input
        v-model="capability"
        class="app-page-toolbar__field"
        dense
        outlined
        :label="t('a2a.federation.dirCapability')"
        :hint="t('a2a.federation.dirCapabilityHint')"
      />
      <q-select
        v-model="orgId"
        class="app-page-toolbar__field"
        dense
        outlined
        emit-value
        map-options
        :options="orgFilterOptions"
        :label="t('a2a.federation.dirOrg')"
      />
      <template #actions>
        <q-btn
          outline
          no-caps
          color="primary"
          icon="search"
          :label="t('a2a.federation.dirSearch')"
          :loading="loading"
          @click="emit('search')"
        />
      </template>
    </AppPageToolbar>

    <AppRegistryTable
      row-key="remote_agent.id"
      :rows="entries"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
      :no-data-label="t('a2a.federation.dirEmpty')"
    >
      <template #body-cell-agent="slotProps">
        <q-td :props="props">
          <div class="app-registry-cell-primary">
            {{ slotProps.row.card.display_name || slotProps.row.card.agent_id }}
          </div>
          <div class="text-caption text-grey-6">{{ slotProps.row.card.agent_id }}</div>
        </q-td>
      </template>
      <template #body-cell-org="slotProps">
        <q-td :props="props">
          <div class="app-registry-cell-primary">{{ slotProps.row.org.name || slotProps.row.org.domain }}</div>
          <div class="text-caption text-grey-6">{{ slotProps.row.org.domain }}</div>
        </q-td>
      </template>
      <template #body-cell-capabilities="slotProps">
        <q-td :props="props">
          <q-chip v-for="c in slotProps.row.card.capabilities" :key="c.name" dense outline size="sm">{{
            c.name
          }}</q-chip>
          <span v-if="!slotProps.row.card.capabilities.length" class="text-grey-6">—</span>
        </q-td>
      </template>
      <template #body-cell-remote_url="slotProps">
        <q-td :props="props">
          <span class="ellipsis">{{ slotProps.row.remote_agent.remote_url || '—' }}</span>
        </q-td>
      </template>
    </AppRegistryTable>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../../layout/AppPageToolbar.vue';
import AppRegistryTable from '../../layout/AppRegistryTable.vue';
import type { FederationAgentEntry, FederationOrg } from '../../../features/a2a/federationTypes';
import type { RegistryTableColumn } from '../../../features/ui/registryTableColumns';

const props = defineProps<{
  entries: FederationAgentEntry[];
  orgs: FederationOrg[];
  loading: boolean;
  columns: RegistryTableColumn<FederationAgentEntry>[];
}>();

const emit = defineEmits<{ search: [] }>();

const capability = defineModel<string>('capability', { default: '' });
const orgId = defineModel<string>('orgId', { default: '' });

const { t } = useI18n();

const orgFilterOptions = computed(() => [
  { label: t('a2a.federation.dirOrgAll'), value: '' },
  ...props.orgs.map((o) => ({ label: `${o.name || o.domain} (${o.domain})`, value: o.id })),
]);
</script>
