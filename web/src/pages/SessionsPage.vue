<template>
  <q-page class="app-page-cream sessions-page q-pa-md">
    <SessionsPageHero :loading="loading" @refresh="loadRows" />

    <SessionsSummaryCards :cards="summaryCards" />

    <SessionsFilterBar
      :keyword="keyword"
      :owner-type="ownerType"
      :status="status"
      :context-status="contextStatus"
      :loading="loading"
      :owner-options="ownerFilterOptions"
      :status-options="statusFilterOptions"
      :context-options="contextFilterOptions"
      @update:keyword="onKeywordUpdate"
      @update:owner-type="ownerType = $event"
      @update:status="status = $event"
      @update:context-status="contextStatus = $event"
      @reset="resetFilters"
      @search="loadRows"
    />

    <SessionsErrorBanner v-if="error" :message="error" @retry="loadRows" />

    <SessionsSelectedDetail v-if="selected" :session="selected" @archive="archiveSelected" />

    <SessionsTableSection
      :rows="rows"
      :loading="loading"
      :page="page"
      :page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :page-size-options="pageSizeSelectOptions"
      @update:page="page = $event"
      @update:page-size="pageSize = $event"
      @archive-row="archiveRow"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { archiveSession, getSession, searchSessions, type Session } from "../features/chat/api";
import SessionsErrorBanner from "../components/sessions/SessionsErrorBanner.vue";
import SessionsFilterBar from "../components/sessions/SessionsFilterBar.vue";
import SessionsPageHero from "../components/sessions/SessionsPageHero.vue";
import SessionsSelectedDetail from "../components/sessions/SessionsSelectedDetail.vue";
import SessionsSummaryCards from "../components/sessions/SessionsSummaryCards.vue";
import SessionsTableSection from "../components/sessions/SessionsTableSection.vue";
import {
  buildSessionsSummaryCards,
  contextFilterOptions,
  ownerFilterOptions,
  pageSizeSelectOptions,
  statusFilterOptions
} from "../components/sessions/sessionUi";

const route = useRoute();

const rows = ref<Session[]>([]);
const selected = ref<Session | null>(null);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const keyword = ref("");
const ownerType = ref<string | null>(null);
const status = ref<string | null>(null);
const contextStatus = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(20);

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const summaryCards = computed(() => buildSessionsSummaryCards(rows.value, total.value));

onMounted(loadRows);

watch([keyword, ownerType, status, contextStatus], () => {
  page.value = 1;
  loadRows();
});

watch([page, pageSize], loadRows);

watch(
  () => route.params.sessionId,
  () => loadSelected()
);

function onKeywordUpdate(value: string | number | null) {
  keyword.value = value == null || value === "" ? "" : String(value);
}

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const result = await searchSessions({
      keyword: keyword.value || undefined,
      owner_type: ownerType.value || undefined,
      status: status.value || undefined,
      context_status: contextStatus.value || undefined,
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value
    });
    rows.value = result.items;
    total.value = result.total;
    await loadSelected();
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载 Session 失败";
  } finally {
    loading.value = false;
  }
}

async function loadSelected() {
  const id = String(route.params.sessionId || "");
  if (!id) {
    selected.value = null;
    return;
  }
  selected.value = rows.value.find((item) => item.id === id) ?? (await getSession(id));
}

function resetFilters() {
  keyword.value = "";
  ownerType.value = null;
  status.value = null;
  contextStatus.value = null;
  page.value = 1;
}

async function archiveRow(id: string) {
  await archiveSession(id);
  if (selected.value?.id === id) {
    selected.value = { ...selected.value, status: "archived" };
  }
  await loadRows();
}

async function archiveSelected() {
  if (!selected.value) return;
  await archiveRow(selected.value.id);
}
</script>

<style scoped>
.sessions-page {
  min-height: 100%;
}
</style>
