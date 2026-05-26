<template>
  <q-page class="app-standard-page app-registry-page hook-deliveries-page">
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

    <AppPageToolbar>
      <q-input v-model="hookKey" class="app-page-toolbar__field" dense outlined clearable debounce="350" label="Hook Key" @update:model-value="onFilterChange" />
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="状态"
        :options="statusOptions"
        @update:model-value="onFilterChange"
      />
      <q-input v-model="from" class="app-page-toolbar__field" dense outlined clearable type="datetime-local" label="起始时间" @update:model-value="onFilterChange" />
      <q-input v-model="to" class="app-page-toolbar__field" dense outlined clearable type="datetime-local" label="结束时间" @update:model-value="onFilterChange" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="() => loadRows()" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="() => loadRows()" />
      </template>
    </q-banner>

    <AppRegistryTable
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-hook_key="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="props.row.webhook_url" empty-label="暂无 URL">
            <span class="app-registry-cell-primary ellipsis">{{ props.row.hook_key }}</span>
          </AppRegistryHoverTip>
        </q-td>
      </template>
      <template #body-cell-status="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="props.row.payload_json" :indicator="Boolean(String(props.row.payload_json ?? '').trim())">
            <q-chip
              dense
              :color="statusColor(props.row.status)"
              text-color="white"
              class="cursor-pointer"
              @click="openDetail(props.row)"
            >
              {{ props.row.status }}
            </q-chip>
          </AppRegistryHoverTip>
        </q-td>
      </template>
    </AppRegistryTable>

    <AppRegistryPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      label="条投递"
    />

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">投递详情</q-card-section>
        <q-card-section class="app-dialog-body q-pt-none">
          <div class="text-caption text-grey-7">{{ detailUrl }}</div>
          <pre class="hook-delivery-detail app-code-block">{{ detailText }}</pre>
          <div v-if="detailError" class="text-negative q-mt-sm">{{ detailError }}</div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import type { QTableColumn } from "quasar";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../components/layout/AppRegistryHoverTip.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import { registryColWidth } from "../features/ui/registryTableColumns";
import { listHookDeliveries, type HookDeliveryRow } from "../features/hooks/deliveries";

const route = useRoute();
const hookKey = ref("");
const status = ref("");
const from = ref("");
const to = ref("");
const rows = ref<HookDeliveryRow[]>([]);
const loading = ref(false);
const error = ref("");
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
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
  { name: "created_at", label: "时间", field: "created_at", align: "left", ...registryColWidth("11%") },
  { name: "hook_key", label: "Hook", field: "hook_key", align: "left", ...registryColWidth("16%") },
  { name: "status", label: "状态", field: "status", align: "left", ...registryColWidth("9%") },
  { name: "attempt_count", label: "尝试", field: "attempt_count", align: "right", ...registryColWidth("72px") },
  { name: "max_attempts", label: "上限", field: "max_attempts", align: "right", ...registryColWidth("72px") }
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

async function loadRows(nextPage = page.value, nextPageSize = pageSize.value) {
  loading.value = true;
  error.value = "";
  try {
    const data = await listHookDeliveries({
      hook_key: hookKey.value.trim() || undefined,
      status: status.value || undefined,
      from: toRFC3339(from.value),
      to: toRFC3339(to.value),
      page: nextPage,
      page_size: nextPageSize
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载投递记录失败";
  } finally {
    loading.value = false;
  }
}

function onFilterChange() {
  page.value = 1;
  void loadRows(1, pageSize.value);
}

watch([page, pageSize], () => void loadRows());

function resetFilters() {
  hookKey.value = "";
  status.value = "";
  from.value = "";
  to.value = "";
  page.value = 1;
  void loadRows(1, pageSize.value);
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
