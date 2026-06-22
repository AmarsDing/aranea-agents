<template>
  <div>
    <AppPageToolbar>
      <q-input v-model="workspace" class="app-page-toolbar__field" dense outlined clearable label="Workspace" />
      <q-input v-model="capability" class="app-page-toolbar__field" dense outlined clearable label="Capability" />
      <q-toggle v-model="checkHealth" dense label="检查健康" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          rounded
          icon="hub"
          label="联邦发现"
          :loading="loading"
          @click="onDiscover"
        />
      </template>
    </AppPageToolbar>
    <AppRegistryTable
      :rows="pagedRows"
      :columns="columns"
      row-key="registry_id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-source="props">
        <q-td :props="props">
          <q-chip dense :color="props.row.source === 'local' ? 'primary' : 'orange'" text-color="white" size="sm">
            {{ props.row.source === 'local' ? '本地' : '远程' }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-display_name="props">
        <q-td :props="props">
          <span class="app-registry-cell-primary ellipsis">{{
            props.row.card?.display_name || props.row.card?.agent_id || '—'
          }}</span>
        </q-td>
      </template>
      <template #body-cell-workspace="props">
        <q-td :props="props">
          <span class="app-registry-cell-sub ellipsis">{{ props.row.card?.workspace || '—' }}</span>
        </q-td>
      </template>
      <template #body-cell-capabilities="props">
        <q-td :props="props">
          <div class="app-registry-chip-wrap">
            <q-chip v-for="c in props.row.card?.capabilities ?? []" :key="c.name" dense outline size="sm">{{
              c.name
            }}</q-chip>
          </div>
        </q-td>
      </template>
      <template #body-cell-healthy="props">
        <q-td :props="props">
          <q-badge :color="props.row.healthy ? 'positive' : 'negative'" :label="props.row.healthy ? '健康' : '异常'" />
        </q-td>
      </template>
      <template #body-cell-endpoint_url="props">
        <q-td :props="props">
          <AppRegistryHoverTip
            v-if="props.row.endpoint_url || props.row.remote_url"
            :text="props.row.endpoint_url || props.row.remote_url"
            empty-label="暂无 URL"
          >
            <span class="app-registry-cell-sub ellipsis">{{
              props.row.endpoint_url || props.row.remote_url || '—'
            }}</span>
          </AppRegistryHoverTip>
          <span v-else class="text-grey-6">—</span>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="entries.length"
      :loading="loading"
      label="个端点"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import type { A2AGatewayEntry } from '../../features/a2a/types';
import type { RegistryTableColumn } from '../../features/ui/registryTableColumns';

const props = defineProps<{
  entries: A2AGatewayEntry[];
  loading: boolean;
  columns: RegistryTableColumn<A2AGatewayEntry>[];
}>();

const workspace = defineModel<string>('workspace', { default: '' });
const capability = defineModel<string>('capability', { default: '' });
const checkHealth = defineModel<boolean>('checkHealth', { default: false });

watch(workspace, (v) => {
  if (v == null) workspace.value = '';
});
watch(capability, (v) => {
  if (v == null) capability.value = '';
});

const emit = defineEmits<{ discover: [] }>();

const page = ref(1);
const pageSize = ref(10);
const pageMax = computed(() => Math.max(1, Math.ceil(props.entries.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.entries.slice(start, start + pageSize.value);
});

function resetFilters() {
  workspace.value = '';
  capability.value = '';
  checkHealth.value = false;
  page.value = 1;
}

function onDiscover() {
  page.value = 1;
  emit('discover');
}
</script>
