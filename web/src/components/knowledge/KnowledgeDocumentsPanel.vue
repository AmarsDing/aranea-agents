<template>
  <div>
    <div class="app-actions-bar app-actions-bar--start q-mb-md">
      <q-btn color="primary" unelevated no-caps icon="upload_file" label="入库文档" @click="$emit('open-ingest')" />
      <q-btn outline no-caps icon="refresh" label="刷新文档" :loading="loading" @click="$emit('refresh')" />
    </div>
    <div class="app-registry-table-shell">
      <q-table
        flat
        dense
        class="app-registry-table"
        :rows="documents"
        :columns="columns"
        row-key="id"
        :loading="loading"
        :pagination="{ rowsPerPage: 10 }"
      >
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round color="negative" icon="delete" @click="$emit('delete-document', props.row)" />
            </div>
          </q-td>
        </template>
      </q-table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { knowledgeDocColumns, knowledgeStatusColor } from "../../features/knowledge/knowledgeUi";
import type { KnowledgeDocument } from "../../features/knowledge/types";

defineProps<{
  documents: KnowledgeDocument[];
  loading: boolean;
}>();

defineEmits<{
  "open-ingest": [];
  refresh: [];
  "delete-document": [doc: KnowledgeDocument];
}>();

const columns = knowledgeDocColumns;
const statusColor = knowledgeStatusColor;
</script>
