<template>
  <q-page class="app-page-cream hook-deliveries-page q-pa-md">
    <section class="row items-center justify-between q-mb-md">
      <div>
        <div class="text-caption text-primary text-weight-bold">Hook notify</div>
        <h1 class="text-h4 q-my-xs">Webhook 投递队列</h1>
        <p class="text-grey-7 q-mb-none">
          查看 Hook <code>notify</code> 动作的异步投递状态（queued → pending/success/failed）。
        </p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn outline rounded icon="rule" label="Hook 规则" to="/hooks" />
        <q-btn outline rounded icon="history" label="阻断/错误记录" to="/plugins/runs" />
      </div>
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <q-input v-model="hookKey" class="col-12 col-md-3" dense outlined clearable debounce="350" label="Hook Key" @update:model-value="onFilterChange" />
        <q-select
          v-model="status"
          class="col-12 col-md-2"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="状态"
          :options="statusOptions"
          @update:model-value="onFilterChange"
        />
        <q-input v-model="from" class="col-12 col-md-3" dense outlined clearable type="datetime-local" label="起始时间" @update:model-value="onFilterChange" />
        <q-input v-model="to" class="col-12 col-md-3" dense outlined clearable type="datetime-local" label="结束时间" @update:model-value="onFilterChange" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="() => loadRows()" />
      </template>
    </q-banner>

    <q-card flat bordered>
      <q-table
        flat
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        v-model:pagination="tablePagination"
        :rows-per-page-options="[10, 20, 50]"
        @request="onTableRequest"
      >
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-chip dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-payload_json="props">
          <q-td :props="props">
            <q-btn flat dense size="sm" label="Payload" @click="openDetail(props.row)" />
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card style="min-width: 420px; max-width: 90vw">
        <q-card-section class="text-h6">投递详情</q-card-section>
        <q-card-section>
          <div class="text-caption text-grey-7">{{ detailUrl }}</div>
          <pre class="hook-delivery-detail">{{ detailText }}</pre>
          <div v-if="detailError" class="text-negative q-mt-sm">{{ detailError }}</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute } from "vue-router";
import type { QTableColumn, QTableProps } from "quasar";
import { listHookDeliveries, type HookDeliveryRow } from "../features/hooks/deliveries";

const route = useRoute();
const hookKey = ref("");
const status = ref("");
const from = ref("");
const to = ref("");
const rows = ref<HookDeliveryRow[]>([]);
const loading = ref(false);
const error = ref("");
const tablePagination = ref({ page: 1, rowsPerPage: 20, rowsNumber: 0 });
const detailOpen = ref(false);
const detailText = ref("");
const detailUrl = ref("");
const detailError = ref("");

const statusOptions = [
  { label: "pending", value: "pending" },
  { label: "success", value: "success" },
  { label: "failed", value: "failed" }
];

const columns: QTableColumn<HookDeliveryRow>[] = [
  { name: "created_at", label: "时间", field: "created_at", align: "left" },
  { name: "hook_key", label: "Hook", field: "hook_key", align: "left" },
  { name: "webhook_url", label: "URL", field: "webhook_url", align: "left" },
  { name: "status", label: "状态", field: "status", align: "left" },
  { name: "attempt_count", label: "尝试", field: "attempt_count", align: "right" },
  { name: "max_attempts", label: "上限", field: "max_attempts", align: "right" },
  { name: "payload_json", label: "Payload", field: "payload_json", align: "left" }
];

function statusColor(st: string) {
  if (st === "failed") return "negative";
  if (st === "success") return "positive";
  return "grey-7";
}

function toRFC3339(local: string): string | undefined {
  const t = local.trim();
  if (!t) return undefined;
  const d = new Date(t);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toISOString();
}

async function loadRows(page = tablePagination.value.page, pageSize = tablePagination.value.rowsPerPage) {
  loading.value = true;
  error.value = "";
  try {
    const data = await listHookDeliveries({
      hook_key: hookKey.value.trim() || undefined,
      status: status.value || undefined,
      from: toRFC3339(from.value),
      to: toRFC3339(to.value),
      page,
      page_size: pageSize
    });
    rows.value = data.items;
    tablePagination.value = { ...tablePagination.value, page, rowsPerPage: pageSize, rowsNumber: data.total };
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载投递记录失败";
  } finally {
    loading.value = false;
  }
}

const onTableRequest: QTableProps["onRequest"] = (props) => {
  void loadRows(props.pagination.page, props.pagination.rowsPerPage);
};

function onFilterChange() {
  tablePagination.value.page = 1;
  void loadRows(1, tablePagination.value.rowsPerPage);
}

function openDetail(row: HookDeliveryRow) {
  detailUrl.value = row.webhook_url;
  detailError.value = row.last_error;
  detailText.value = row.payload_json?.trim() ? row.payload_json : JSON.stringify(row, null, 2);
  detailOpen.value = true;
}

onMounted(() => {
  const qk = route.query.hook_key;
  if (typeof qk === "string" && qk.trim()) hookKey.value = qk.trim();
  void loadRows();
});
</script>

<style scoped lang="sass">
.hook-delivery-detail
  margin: 8px 0 0
  white-space: pre-wrap
  word-break: break-word
  font-size: 12px
</style>
