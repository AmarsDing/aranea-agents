<template>
  <div>
    <AppPageToolbar>
      <template #actions>
        <q-btn flat rounded no-caps icon="upload_file" label="入库文档" @click="$emit('open-ingest')" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </template>
    </AppPageToolbar>
    <AppRegistryTable
      :shell="false"
      :rows="pagedRows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-source="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="props.row.source" empty-label="—">
            <span class="app-registry-cell-primary ellipsis">{{ props.row.source }}</span>
          </AppRegistryHoverTip>
        </q-td>
      </template>
      <template #body-cell-mime_type="props">
        <q-td :props="props">
          <span class="text-caption">{{ props.row.mime_type || '—' }}</span>
        </q-td>
      </template>
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">{{
            props.row.status
          }}</q-chip>
          <q-tooltip v-if="props.row.status === 'error' && props.row.error_message" max-width="360px">
            {{ props.row.error_message }}
          </q-tooltip>
          <q-chip
            v-if="props.row.status === 'indexed' && props.row.extract_supported === false"
            dense
            color="warning"
            text-color="dark"
            size="sm"
            icon="warning"
            class="q-ml-xs"
          >
            <q-tooltip>该格式不在后端支持的解析类型中，检索内容可能为空</q-tooltip>
            不可检索
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-size_bytes="props">
        <q-td :props="props">{{ formatKnowledgeDocSize(props.row.size_bytes) }}</q-td>
      </template>
      <template #body-cell-created_at="props">
        <q-td :props="props">
          <span class="text-caption">{{ formatKnowledgeTime(props.row.created_at) }}</span>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
            <q-btn
              flat
              dense
              round
              color="negative"
              icon="delete"
              aria-label="删除"
              @click="$emit('delete-document', props.row)"
            />
          </div>
        </q-td>
      </template>
    </AppRegistryTable>
    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="documents.length"
      :loading="loading"
      label="个文档"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import {
  formatKnowledgeDocSize,
  formatKnowledgeTime,
  KNOWLEDGE_DOC_TABLE_COLUMNS,
  knowledgeStatusColor,
} from '../../features/knowledge/knowledgeUi';
import type { KnowledgeDocument } from '../../features/knowledge/types';

const props = defineProps<{
  documents: KnowledgeDocument[];
  loading: boolean;
}>();

defineEmits<{
  'open-ingest': [];
  refresh: [];
  'delete-document': [doc: KnowledgeDocument];
}>();

const columns = KNOWLEDGE_DOC_TABLE_COLUMNS;
const statusColor = knowledgeStatusColor;

const page = ref(1);
const pageSize = ref(10);
const pageMax = computed(() => Math.max(1, Math.ceil(props.documents.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.documents.slice(start, start + pageSize.value);
});

watch(
  () => props.documents.length,
  () => {
    if (page.value > pageMax.value) page.value = pageMax.value;
  },
);
</script>
