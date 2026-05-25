<template>
  <div>
    <q-card flat bordered class="monitor-card">
      <q-card-section class="row items-center q-col-gutter-md">
        <div class="col-12 col-md">
          <div class="text-h6 text-weight-bold">活动日志</div>
          <div class="text-caption text-grey-7">管理与配置变更审计记录</div>
        </div>
        <div class="app-form-field-grid items-center">
          <q-select
            v-model="actionFilter"
            dense
            outlined
            emit-value
            map-options
            clearable
            label="事件类型"
            :options="actionOptions"
          />
          <q-select
            v-model="resourceFilter"
            dense
            outlined
            emit-value
            map-options
            clearable
            label="实体类型"
            :options="resourceOptions"
          />
          <q-input
            v-model="keyword"
            class="app-field-md"
            dense
            outlined
            clearable
            debounce="200"
            label="搜索事件 / 资源 / 详情"
          >
            <template #prepend>
              <q-icon name="search" />
            </template>
          </q-input>
        </div>
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
      </q-card-section>
      <q-separator />
      <q-table
        flat
        :rows="filteredRows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        :pagination="pagination"
        :rows-per-page-options="[12, 25, 50, 100]"
        @request="onPageRequest"
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
        <template #body-cell-actor="props">
          <q-td :props="props">
            <div>{{ props.row.actor || "system" }}</div>
            <div v-if="props.row.ip" class="text-caption text-grey-7">{{ props.row.ip }}</div>
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
      <q-card-section v-if="total > 0" class="text-caption text-grey-7 text-right">
        共 {{ total }} 条记录
      </q-card-section>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card class="monitor-detail-card app-dialog-card app-dialog-card--lg">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">Audit 详情</div>
            <div class="text-caption text-grey-7">{{ selected?.action }}.{{ selected?.resource }}</div>
          </div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-list dense>
            <q-item v-if="selected?.actor">
              <q-item-section>操作者</q-item-section>
              <q-item-section side>{{ selected?.actor }}</q-item-section>
            </q-item>
            <q-item v-if="selected?.ip">
              <q-item-section>IP</q-item-section>
              <q-item-section side>{{ selected?.ip }}</q-item-section>
            </q-item>
            <q-item v-if="selected?.severity">
              <q-item-section>严重级别</q-item-section>
              <q-item-section side>
                <q-badge :color="severityColor(selected!.severity)">{{ selected?.severity }}</q-badge>
              </q-item-section>
            </q-item>
          </q-list>
          <pre class="monitor-json app-code-block">{{ selectedJSON }}</pre>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="复制 JSON" icon="content_copy" @click="copyJSON" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { copyToClipboard, Notify, type QTableColumn, type QTableProps } from "quasar";
import type { AuditLog } from "../../features/monitor/types";
import { compactJSON, formatDate } from "../../features/monitor/utils";

const props = defineProps<{
  rows: AuditLog[];
  total: number;
  loading: boolean;
}>();

defineEmits<{
  reload: [];
}>();

const keyword = ref("");
const actionFilter = ref<string | null>(null);
const resourceFilter = ref<string | null>(null);
const selected = ref<AuditLog | null>(null);
const detailOpen = ref(false);

const pagination = ref({ page: 1, rowsPerPage: 12, rowsNumber: props.total });

const actionOptions = computed(() => {
  const actions = new Set(props.rows.map((r) => r.action).filter(Boolean));
  return [...actions].map((a) => ({ label: a, value: a }));
});

const resourceOptions = computed(() => {
  const resources = new Set(props.rows.map((r) => r.resource).filter(Boolean));
  return [...resources].map((r) => ({ label: r, value: r }));
});

const columns: QTableColumn<AuditLog>[] = [
  { name: "event", label: "事件", field: "action", align: "left" },
  { name: "resource", label: "实体", field: "resource", align: "left" },
  { name: "actor", label: "操作者", field: "actor", align: "left" },
  { name: "request", label: "Request ID", field: "request_id", align: "left" },
  { name: "detail", label: "详情", field: "detail", align: "left" },
  { name: "created", label: "时间", field: "created_at", align: "left" }
];

const filteredRows = computed(() => {
  let result = props.rows;
  if (actionFilter.value) {
    result = result.filter((row) => row.action === actionFilter.value);
  }
  if (resourceFilter.value) {
    result = result.filter((row) => row.resource === resourceFilter.value);
  }
  const q = keyword.value.trim().toLowerCase();
  if (q) {
    result = result.filter((row) =>
      [row.action, row.resource, row.resource_id, row.request_id, row.detail, row.actor].some((value) =>
        String(value || "")
          .toLowerCase()
          .includes(q)
      )
    );
  }
  return result;
});

const selectedJSON = computed(() => compactJSON(selected.value ?? {}));

function onPageRequest(requestProp: { pagination: { sortBy: string; descending: boolean; page: number; rowsPerPage: number; rowsNumber?: number }; filter?: unknown; getCellValue: (col: unknown, row: unknown) => unknown }) {
  pagination.value = {
    ...pagination.value,
    page: requestProp.pagination.page,
    rowsPerPage: requestProp.pagination.rowsPerPage
  };
}

function eventColor(action: string) {
  if (action.includes("delete")) return "negative";
  if (action.includes("create")) return "positive";
  if (action.includes("toggle")) return "orange";
  if (action.includes("credentials")) return "purple";
  if (action.includes("update")) return "primary";
  return "grey";
}

function severityColor(severity: string) {
  if (severity === "critical" || severity === "high") return "negative";
  if (severity === "warning" || severity === "medium") return "orange";
  return "positive";
}

async function copyJSON() {
  await copyToClipboard(selectedJSON.value);
  Notify.create({ message: "已复制", color: "positive", position: "top" });
}
</script>
