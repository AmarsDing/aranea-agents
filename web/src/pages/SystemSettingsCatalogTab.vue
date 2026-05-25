<template>
  <div class="model-catalog-tab q-gutter-md">
    <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">模型目录（models.dev）</h2>
      <p class="app-settings-section__hint">
        从
        <a href="https://github.com/anomalyco/models.dev" target="_blank" rel="noopener">models.dev</a>
        同步 AI 模型规格到本地 JSON，供 Provider 默认值与定价（USD/1M）使用。
      </p>

      <div v-if="status" class="catalog-status-grid q-mb-md">
        <div class="catalog-stat">
          <span class="catalog-stat__label">状态</span>
          <span class="catalog-stat__value">{{ status.catalogLoaded ? "已加载" : "未加载" }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">Provider</span>
          <span class="catalog-stat__value">{{ status.providerCount ?? 0 }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">Models</span>
          <span class="catalog-stat__value">{{ status.modelCount ?? 0 }}</span>
        </div>
        <div class="catalog-stat">
          <span class="catalog-stat__label">上次同步</span>
          <span class="catalog-stat__value">{{ lastSyncLabel }}</span>
        </div>
        <div class="catalog-stat catalog-stat--wide">
          <span class="catalog-stat__label">本地路径</span>
          <span class="catalog-stat__value text-caption">{{ status.localPath || "—" }}</span>
        </div>
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">更新策略</h2>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input v-model="policyForm.sourceUrl" label="数据源 URL" outlined dense class="app-field-long" />
        <q-select
          v-model="policyForm.syncPolicy"
          label="同步策略"
          outlined
          dense
          emit-value
          map-options
          :options="syncPolicyOptions"
        />
        <q-input
          v-model.number="policyForm.syncIntervalHours"
          type="number"
          min="1"
          label="间隔（小时）"
          outlined
          dense
        />
        <q-select
          v-model="policyForm.autoApply"
          label="自动应用到 DB"
          outlined
          dense
          emit-value
          map-options
          :options="autoApplyOptions"
        />
      </div>
      <div class="app-actions-bar app-actions-bar--start q-mt-md">
        <q-btn color="primary" unelevated no-caps :loading="savingPolicy" label="保存策略" @click="savePolicy" />
        <q-btn outline color="primary" no-caps :loading="syncing" label="立即同步" @click="runSync(false)" />
        <q-btn flat color="secondary" no-caps :loading="syncing" label="Dry Run" @click="runSync(true)" />
        <q-btn flat color="primary" no-caps :loading="loading" label="刷新" @click="loadAll" />
      </div>
      <div v-if="status?.lastSyncSummary" class="text-caption text-grey-7 q-mt-sm">
        最近：{{ status.lastSyncSummary }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">Provider 命名对齐</h2>
      <p class="app-settings-section__hint">
        内置 legacy → models.dev id 映射（随版本发布，不可编辑）。同步时自动迁移绑定；也可手动立即对齐。
      </p>
      <div class="app-actions-bar app-actions-bar--start q-mb-md">
        <q-btn outline color="primary" no-caps :loading="loadingMigration" label="预览影响" @click="loadMigrationPreview" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :loading="applyingMigration"
          :disable="!migrationItems?.length"
          label="立即对齐"
          @click="runApplyMigration"
        />
      </div>
      <q-list v-if="migrationItems?.length" bordered class="q-mb-md rounded-borders">
        <q-item v-for="item in migrationItems" :key="item.legacyProvider">
          <q-item-section>
            <q-item-label>{{ item.legacyProvider }} → {{ item.catalogProvider }}</q-item-label>
            <q-item-label caption>
              LLM {{ item.llmRows ?? 0 }} · Agent {{ item.agents ?? 0 }} · Session {{ item.sessions ?? 0 }} · Eval {{ item.evalFields ?? 0 }}
              · Runtime {{ item.runtimeSettings ?? 0 }} · Skill {{ item.skills ?? 0 }} · Embed {{ item.knowledgeEmbed ?? 0 }}
              · Research {{ item.webResearch ?? 0 }}
            </q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else-if="migrationLoaded" class="text-caption text-grey-7 q-mb-md">无待迁移绑定</div>
      <div v-if="migrationRules.length" class="text-caption text-grey-7 q-mb-sm">
        内置规则 v{{ migrationVersion || "—" }}
        <span v-if="migrationLastApplied"> · 上次对齐 {{ migrationLastApplied }}</span>
      </div>
      <q-list v-if="migrationRules.length" bordered dense separator class="rounded-borders">
        <q-item v-for="rule in migrationRules" :key="rule.legacy">
          <q-item-section>
            <q-item-label>{{ rule.legacy }} → {{ rule.catalog }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">Catalog 浏览</h2>
      <p class="app-settings-section__hint">分页浏览 Provider；JSON 内容通过服务端搜索加载，避免全量下载。</p>
      <div class="row q-col-gutter-sm q-mb-md items-end">
        <div class="col-grow">
          <q-input
            v-model="providerBrowseQ"
            dense
            outlined
            clearable
            debounce="300"
            label="搜索 Provider"
            @update:model-value="loadProviderBrowse(true)"
          />
        </div>
        <div class="col-auto">
          <q-btn flat no-caps :disable="providerBrowseOffset <= 0" label="上一页" @click="providerBrowsePrev" />
          <q-btn
            flat
            no-caps
            :disable="providerBrowseOffset + providerBrowseItems.length >= providerBrowseTotal"
            label="下一页"
            @click="providerBrowseNext"
          />
        </div>
      </div>
      <q-list v-if="providerBrowseItems.length" bordered separator class="rounded-borders q-mb-md">
        <q-item v-for="p in providerBrowseItems" :key="p.id">
          <q-item-section>
            <q-item-label>{{ p.name || p.id }}</q-item-label>
            <q-item-label caption>{{ p.id }} · {{ p.modelCount ?? 0 }} models</q-item-label>
          </q-item-section>
          <q-item-section side>
            <a v-if="providerDocHref(p.doc)" :href="providerDocHref(p.doc)" target="_blank" rel="noopener" class="text-caption">文档 ↗</a>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="text-caption text-grey-7 q-mb-md">无匹配 Provider（请先同步 catalog）</div>
      <div class="text-caption text-grey-7 q-mb-sm">
        显示 {{ providerBrowseOffset + 1 }}–{{ providerBrowseOffset + providerBrowseItems.length }} / {{ providerBrowseTotal }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">Catalog JSON 浏览</h2>
      <p class="app-settings-section__hint">
        按 Provider 或模型关键词搜索；留空则按 Provider 分页浏览（每页 1 条完整 JSON）。
      </p>
      <q-input
        :model-value="jsonFilter"
        dense
        outlined
        clearable
        debounce="300"
        placeholder="搜索 Provider / 模型（如 openai、gpt-4o）..."
        class="q-mb-sm app-field-long"
        @update:model-value="onJsonFilterChange"
      />
      <q-banner v-if="jsonSearchLegacyMode" rounded class="bg-warning text-dark q-mb-sm">
        当前返回的是旧版「行片段」格式（非完整 JSON）。请重启 admin 服务（<code>go run ./cmd/admin</code>）后刷新本页。
      </q-banner>
      <q-banner v-if="jsonSearchTruncated" rounded class="bg-info text-white q-mb-sm">
        匹配结果过多，仅显示前 {{ jsonSearchCap }} 条，请缩小搜索关键词。
      </q-banner>
      <div class="row q-col-gutter-sm q-mb-sm items-center">
        <span class="text-caption text-grey-7">
          匹配 {{ jsonSearchTotal }} 条
          <template v-if="jsonSearchBlocks.length === 1"> · 当前第 {{ jsonSearchOffset + 1 }} 条</template>
        </span>
        <q-space />
        <q-btn flat dense no-caps :disable="jsonSearchOffset <= 0" label="上一页" @click="jsonSearchPrev" />
        <q-btn
          flat
          dense
          no-caps
          :disable="jsonSearchOffset + jsonSearchLimit >= jsonSearchTotal"
          label="下一页"
          @click="jsonSearchNext"
        />
      </div>
      <div v-if="jsonSearchLoading" class="row justify-center q-py-lg">
        <q-spinner color="primary" size="32px" />
      </div>
      <JsonCodeViewer
        v-else-if="jsonSearchDisplayText"
        :text="jsonSearchDisplayText"
        scroll-height="480px"
      />
      <div v-else class="text-caption text-grey-7 q-py-md">
        {{ jsonSearchError || (jsonSearchQuery ? "无匹配结果" : "暂无 catalog 数据，请先同步") }}
      </div>
    </section>

    <section class="app-settings-section">
      <h2 class="app-settings-section__title">更新日志</h2>
      <q-list v-if="logs.length" bordered separator class="rounded-borders">
        <q-item v-for="entry in logs" :key="entry.id">
          <q-item-section>
            <q-item-label>{{ entry.id }}</q-item-label>
            <q-item-label caption>{{ entry.message }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-badge :color="entry.status === 'ok' ? 'positive' : 'negative'" :label="entry.status || '?'" />
          </q-item-section>
        </q-item>
      </q-list>
      <div v-else class="text-caption text-grey-7">暂无同步日志</div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import {
  getModelCatalogPolicy,
  getModelCatalogStatus,
  listModelCatalogSyncLogs,
  syncModelCatalog,
  updateModelCatalogPolicy,
  previewModelCatalogMigration,
  getProviderMigrationRules,
  applyProviderMigration,
  listCatalogProviders,
  searchCatalogBlocks,
  type ModelCatalogPolicy,
  type ModelCatalogStatus
} from "../features/model-catalog/api";
import { clearProviderLogoCache } from "../features/model-catalog/providerLogo";
import { resetProviderMigrationCache } from "../features/model-catalog/providerMigration";
import JsonCodeViewer from "../components/common/JsonCodeViewer.vue";

const $q = useQuasar();
const loading = ref(false);
const savingPolicy = ref(false);
const syncing = ref(false);
const error = ref("");
const status = ref<ModelCatalogStatus | null>(null);
const jsonFilter = ref("");
const jsonSearchBlocks = ref<string[]>([]);
const jsonSearchTotal = ref(0);
const jsonSearchOffset = ref(0);
const jsonSearchLimit = 1;
const jsonSearchCap = 200;
const jsonSearchLoading = ref(false);
const jsonSearchError = ref("");
const jsonSearchLegacyMode = ref(false);
const jsonSearchTruncated = ref(false);

const jsonSearchDisplayText = computed(() => jsonSearchBlocks.value[0] ?? "");
const jsonSearchQuery = computed(() => (jsonFilter.value ?? "").trim());
const providerBrowseQ = ref("");
const providerBrowseItems = ref<Awaited<ReturnType<typeof listCatalogProviders>>["items"]>([]);
const providerBrowseTotal = ref(0);
const providerBrowseOffset = ref(0);
const providerBrowseLimit = 50;
const logs = ref<Awaited<ReturnType<typeof listModelCatalogSyncLogs>>>([]);
const loadingMigration = ref(false);
const applyingMigration = ref(false);
const migrationLoaded = ref(false);
const migrationItems = ref<Awaited<ReturnType<typeof previewModelCatalogMigration>>["items"]>([]);
const migrationRules = ref<{ legacy: string; catalog: string }[]>([]);
const migrationVersion = ref("");
const migrationLastApplied = ref("");

const policyForm = reactive<ModelCatalogPolicy>({
  sourceUrl: "https://models.dev/api.json",
  syncPolicy: "scheduled",
  syncIntervalHours: 24,
  autoApply: "metadata_and_pricing"
});

const syncPolicyOptions = [
  { label: "关闭", value: "off" },
  { label: "定时", value: "scheduled" }
];

const autoApplyOptions = [
  { label: "仅更新本地 JSON", value: "none" },
  { label: "元数据 + 定价", value: "metadata_and_pricing" },
  { label: "完整规格", value: "full_spec" },
  { label: "完整 + Runtime Overlay", value: "full_spec_and_runtime_overlay" }
];

const lastSyncLabel = computed(() => {
  const ts = status.value?.lastSyncAt;
  if (!ts) return "—";
  if (typeof ts === "string") return ts;
  if (typeof ts === "object" && ts !== null && "seconds" in ts) {
    const sec = Number((ts as { seconds?: number }).seconds ?? 0);
    if (sec > 0) return new Date(sec * 1000).toLocaleString();
  }
  return "—";
});

function providerDocHref(doc?: string) {
  const d = doc?.trim();
  if (!d) return "";
  return /^https?:\/\//i.test(d) ? d : `https://${d}`;
}

async function loadProviderBrowse(resetOffset = false) {
  if (resetOffset) providerBrowseOffset.value = 0;
  try {
    const res = await listCatalogProviders(providerBrowseQ.value.trim(), providerBrowseLimit, providerBrowseOffset.value);
    providerBrowseItems.value = res.items;
    providerBrowseTotal.value = res.total;
  } catch (e) {
    providerBrowseItems.value = [];
    providerBrowseTotal.value = 0;
    const msg = e instanceof Error ? e.message : "加载 Provider 列表失败";
    error.value = msg;
    $q.notify({ type: "warning", message: msg });
  }
}

function providerBrowsePrev() {
  providerBrowseOffset.value = Math.max(0, providerBrowseOffset.value - providerBrowseLimit);
  void loadProviderBrowse(false);
}

function providerBrowseNext() {
  providerBrowseOffset.value += providerBrowseLimit;
  void loadProviderBrowse(false);
}

function onJsonFilterChange(value: string | number | null) {
  jsonFilter.value = value == null ? "" : String(value);
  void loadJsonSearch(true);
}

async function loadJsonSearch(resetOffset = false) {
  if (resetOffset) jsonSearchOffset.value = 0;
  jsonSearchLoading.value = true;
  jsonSearchError.value = "";
  jsonSearchLegacyMode.value = false;
  jsonSearchTruncated.value = false;
  try {
    const q = jsonSearchQuery.value;
    const res = await searchCatalogBlocks(q, jsonSearchLimit, jsonSearchOffset.value);
    jsonSearchBlocks.value = res.blocks;
    jsonSearchTotal.value = res.total;
    jsonSearchOffset.value = res.offset;
    jsonSearchTruncated.value = res.truncated;
    jsonSearchLegacyMode.value = res.legacyLineMode;
    if (res.legacyLineMode) {
      jsonSearchError.value = "后端返回行片段格式，请重启 admin 服务";
    }
  } catch (e) {
    jsonSearchBlocks.value = [];
    jsonSearchTotal.value = 0;
    const msg = e instanceof Error ? e.message : "搜索 catalog 失败";
    jsonSearchError.value = msg;
    error.value = msg;
    $q.notify({ type: "warning", message: msg });
  } finally {
    jsonSearchLoading.value = false;
  }
}

function jsonSearchPrev() {
  jsonSearchOffset.value = Math.max(0, jsonSearchOffset.value - jsonSearchLimit);
  void loadJsonSearch(false);
}

function jsonSearchNext() {
  jsonSearchOffset.value += jsonSearchLimit;
  void loadJsonSearch(false);
}

async function loadAll() {
  loading.value = true;
  error.value = "";
  try {
    const [st, pol, logItems] = await Promise.all([
      getModelCatalogStatus(),
      getModelCatalogPolicy(),
      listModelCatalogSyncLogs(30)
    ]);
    status.value = st;
    Object.assign(policyForm, pol);
    logs.value = logItems;
    await Promise.all([loadProviderBrowse(true), loadJsonSearch(true)]);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    loading.value = false;
  }
}

async function savePolicy() {
  savingPolicy.value = true;
  error.value = "";
  try {
    await updateModelCatalogPolicy({ ...policyForm });
    $q.notify({ type: "positive", message: "策略已保存" });
    await loadAll();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "保存失败";
  } finally {
    savingPolicy.value = false;
  }
}

async function loadMigrationPreview() {
  loadingMigration.value = true;
  error.value = "";
  migrationItems.value = [];
  migrationLoaded.value = false;
  try {
    const res = await previewModelCatalogMigration();
    migrationItems.value = res.items ?? [];
    migrationLoaded.value = true;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "预览失败";
    migrationItems.value = [];
    migrationLoaded.value = false;
  } finally {
    loadingMigration.value = false;
  }
}

async function loadMigrationRules() {
  try {
    const res = await getProviderMigrationRules();
    migrationRules.value = (res.rules ?? []).map((r) => ({
      legacy: r.legacy ?? "",
      catalog: r.catalog ?? "",
    }));
    migrationVersion.value = res.version ?? "";
    migrationLastApplied.value = res.lastAppliedAt ?? "";
  } catch {
    migrationRules.value = [];
  }
}

async function runApplyMigration() {
  applyingMigration.value = true;
  error.value = "";
  try {
    const res = await applyProviderMigration();
    $q.notify({
      type: res.ok ? "positive" : "warning",
      message: res.message || (res.ok ? "命名对齐完成" : "对齐失败"),
    });
    await loadMigrationPreview();
    resetProviderMigrationCache();
    await loadMigrationRules();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "对齐失败";
  } finally {
    applyingMigration.value = false;
  }
}

async function runSync(dryRun: boolean) {
  syncing.value = true;
  error.value = "";
  try {
    const res = await syncModelCatalog(dryRun);
    const applyFailed = res.applyFailed || (res.applyErrors?.length ?? 0) > 0;
    if (res.ok && !applyFailed) {
      clearProviderLogoCache();
    }
    $q.notify({
      type: res.ok && !applyFailed ? "positive" : "warning",
      message:
        res.message ||
        (applyFailed ? `同步完成但应用失败：${(res.applyErrors ?? []).join("; ")}` : res.ok ? "同步完成" : "同步失败"),
    });
    await loadAll();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "同步失败";
  } finally {
    syncing.value = false;
  }
}

onMounted(() => {
  void loadAll();
  void loadMigrationRules();
});
</script>
