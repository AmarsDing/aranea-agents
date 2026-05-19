<template>
  <div>
    <div class="row q-mb-md q-gutter-sm">
      <q-btn color="primary" unelevated icon="upload_file" label="入库文档" @click="$emit('open-ingest')" />
      <q-btn outline icon="refresh" label="刷新文档" :loading="loading" @click="$emit('refresh')" />
    </div>
    <q-table flat :rows="documents" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 10 }">
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round color="negative" icon="delete" @click="$emit('delete-document', props.row)" />
        </q-td>
      </template>
    </q-table>
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
