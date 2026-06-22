<template>
  <div>
    <AppPageToolbar>
      <q-input v-model="workspace" class="app-page-toolbar__field" dense outlined clearable label="Workspace" />
      <q-input v-model="capability" class="app-page-toolbar__field" dense outlined clearable label="Capability" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          rounded
          icon="search"
          label="发现"
          :loading="loading"
          @click="onDiscover"
        />
      </template>
    </AppPageToolbar>
    <AppRegistryTable
      :rows="pagedRows"
      :columns="columns"
      row-key="agent_id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-agent_id="slotProps">
        <q-td :props="slotProps">
          <div class="app-registry-cell-primary ellipsis" :title="slotProps.row.agent_id">
            {{ slotProps.row.agent_id }}
          </div>
        </q-td>
      </template>
      <template #body-cell-display_name="slotProps">
        <q-td :props="slotProps">
          <span class="app-registry-cell-sub ellipsis" :title="slotProps.row.display_name">{{
            slotProps.row.display_name || '—'
          }}</span>
        </q-td>
      </template>
      <template #body-cell-enabled="slotProps">
        <q-td :props="slotProps">
          <q-chip dense :color="slotProps.row.enabled ? 'positive' : 'grey'" text-color="white" size="sm">
            {{ slotProps.row.enabled ? '启用' : '禁用' }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-capabilities="slotProps">
        <q-td :props="slotProps">
          <div class="app-registry-chip-wrap">
            <q-chip v-for="c in slotProps.row.capabilities" :key="c.name" dense outline size="sm">{{ c.name }}</q-chip>
          </div>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="agents.length"
      :loading="loading"
      label="个 Agent"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import type { A2AAgentCard } from '../../features/a2a/types';
import type { RegistryTableColumn } from '../../features/ui/registryTableColumns';

const props = defineProps<{
  agents: A2AAgentCard[];
  loading: boolean;
  columns: RegistryTableColumn<A2AAgentCard>[];
}>();

const workspace = defineModel<string>('workspace', { default: '' });
const capability = defineModel<string>('capability', { default: '' });

watch(workspace, (v) => {
  if (v == null) workspace.value = '';
});
watch(capability, (v) => {
  if (v == null) capability.value = '';
});

const emit = defineEmits<{ discover: [] }>();

const page = ref(1);
const pageSize = ref(10);
const pageMax = computed(() => Math.max(1, Math.ceil(props.agents.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.agents.slice(start, start + pageSize.value);
});

function resetFilters() {
  workspace.value = '';
  capability.value = '';
  page.value = 1;
}

function onDiscover() {
  page.value = 1;
  emit('discover');
}
</script>
