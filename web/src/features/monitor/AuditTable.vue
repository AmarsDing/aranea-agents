<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="text-h6 text-weight-bold">活动日志</div>
        <div class="text-caption text-grey-7">管理与配置变更审计记录</div>
      </div>
      <q-input v-model="keyword" dense outlined clearable debounce="200" class="col-12 col-md-4" label="搜索事件 / 资源 / 详情">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
    </q-card-section>
    <q-separator />
    <q-table
      flat
      :rows="filteredRows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      :pagination="{ rowsPerPage: 12 }"
      @row-click="(_, row) => selectRow(row)"
    >
      <template #body-cell-event="props">
        <q-td :props="props">
          <q-chip dense square :color="eventColor(props.row.action)" text-color="white">
            {{ props.row.action }}.{{ props.row.resource }}
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-resource="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.resource }}</div>
          <div class="text-caption text-grey-7 ellipsis">{{ props.row.resource_id || "-" }}</div>
        </q-td>
      </template>
      <template #body-cell-request="props">
        <q-td :props="props">
          <code class="monitor-code">{{ props.row.request_id || "-" }}</code>
        </q-td>
      </template>
      <template #body-cell-created="props">
        <q-td :props="props">{{ formatDate(props.row.created_at) }}</q-td>
      </template>
    </q-table>
  </q-card>

  <q-dialog v-model="detailOpen">
    <q-card class="monitor-detail-card">
      <q-card-section class="row items-start justify-between">
        <div>
          <div class="text-h6">Audit 详情</div>
          <div class="text-caption text-grey-7">{{ selected?.action }}.{{ selected?.resource }}</div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <pre class="monitor-json">{{ selectedJSON }}</pre>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="复制 JSON" icon="content_copy" @click="copyJSON" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { copyToClipboard, Notify, type QTableColumn } from "quasar";
import type { AuditLog } from "./types";
import { compactJSON, formatDate } from "./utils";

const props = defineProps<{
  rows: AuditLog[];
  loading: boolean;
}>();

defineEmits<{
  reload: [];
}>();

const keyword = ref("");
const selected = ref<AuditLog | null>(null);
const detailOpen = ref(false);

const columns: QTableColumn<AuditLog>[] = [
  { name: "event", label: "事件", field: "action", align: "left" },
  { name: "resource", label: "实体", field: "resource", align: "left" },
  { name: "request", label: "Request ID", field: "request_id", align: "left" },
  { name: "detail", label: "详情", field: "detail", align: "left" },
  { name: "created", label: "时间", field: "created_at", align: "left" }
];

const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return props.rows;
  return props.rows.filter((row) =>
    [row.action, row.resource, row.resource_id, row.request_id, row.detail]
      .some((value) => String(value || "").toLowerCase().includes(q))
  );
});

const selectedJSON = computed(() => compactJSON(selected.value ?? {}));

function selectRow(row: AuditLog) {
  selected.value = row;
  detailOpen.value = true;
}

async function copyJSON() {
  await copyToClipboard(selectedJSON.value);
  Notify.create({ message: "已复制", color: "positive", position: "top" });
}

function eventColor(action: string) {
  if (action.includes("delete")) return "negative";
  if (action.includes("create")) return "positive";
  if (action.includes("toggle")) return "orange";
  if (action.includes("credentials")) return "purple";
  return "primary";
}
</script>
