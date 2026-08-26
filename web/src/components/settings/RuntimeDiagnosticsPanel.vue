<template>
  <div class="runtime-diagnostics-panel">
    <div class="section-heading">
      <div class="section-heading__main">
        <div class="section-title">
          <q-icon name="health_and_safety" size="sm" color="primary" />
          <span class="section-title__text">{{ t('settingsPage.diagnostics.title') }}</span>
        </div>
        <p class="settings-section__hint">{{ t('settingsPage.diagnostics.hint') }}</p>
      </div>
      <q-btn
        outline
        color="primary"
        no-caps
        dense
        icon="refresh"
        :label="t('settingsPage.diagnostics.refresh')"
        :loading="loading"
        @click="load"
      />
    </div>

    <q-banner v-if="error" rounded dense class="bg-negative text-white q-mb-md">
      {{ t('settingsPage.diagnostics.loadFailed') }}: {{ error }}
    </q-banner>

    <div v-if="loading && items.length === 0" class="row items-center q-py-md">
      <q-spinner-dots color="primary" size="28px" />
      <span class="q-ml-sm text-grey-7">{{ t('settingsPage.diagnostics.loading') }}</span>
    </div>

    <q-banner v-else-if="items.length === 0 && !error" dense rounded class="settings-info-banner">
      {{ t('settingsPage.diagnostics.empty') }}
    </q-banner>

    <q-list v-else separator class="diagnostics-list">
      <q-item
        v-for="item in items"
        :key="item.key"
        :clickable="item.status !== 'pass'"
        :disable="item.status === 'pass'"
        class="diagnostics-item"
        @click="goDetail(item)"
      >
        <q-item-section avatar>
          <q-icon :name="statusIcon(item.status)" :color="statusColor(item.status)" size="sm" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="text-weight-medium">{{ itemLabel(item.key) }}</q-item-label>
          <q-item-label caption lines="2">{{ item.summary }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <div class="row items-center no-wrap q-gutter-sm">
            <q-badge :color="statusColor(item.status)" outline :label="statusText(item.status)" />
            <q-btn
              v-if="item.status !== 'pass'"
              flat
              dense
              no-caps
              color="primary"
              icon="arrow_forward"
              :label="t('settingsPage.diagnostics.goto')"
              @click.stop="goDetail(item)"
            />
          </div>
        </q-item-section>
        <q-tooltip v-if="item.status !== 'pass'">{{ item.detail_ref }}</q-tooltip>
      </q-item>
    </q-list>

    <div v-if="lastRunAt" class="text-caption text-grey-6 q-mt-sm">
      {{ t('settingsPage.diagnostics.lastRun') }}: {{ lastRunAt }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { getDiagnostics } from '../../features/system-settings/api';
import type { DiagnosticsItem, DiagnosticsStatus } from '../../features/system-settings/types';

const { t } = useI18n();
const router = useRouter();

const items = ref<DiagnosticsItem[]>([]);
const loading = ref(false);
const error = ref('');
const lastRunAt = ref('');

// key → i18n 标签映射；未知 key 回退原文（新检查项后端先上线时前端不破版）。
const ITEM_LABEL_KEYS: Record<string, string> = {
  model_providers: 'settingsPage.diagnostics.itemModelProviders',
  mcp_servers: 'settingsPage.diagnostics.itemMcpServers',
  tool_assembly: 'settingsPage.diagnostics.itemToolAssembly',
  memory_stack: 'settingsPage.diagnostics.itemMemoryStack',
  cache_baseline: 'settingsPage.diagnostics.itemCacheBaseline',
  config_graph: 'settingsPage.diagnostics.itemConfigGraph',
};

function itemLabel(key: string): string {
  const labelKey = ITEM_LABEL_KEYS[key];
  return labelKey ? t(labelKey) : key;
}

function statusIcon(status: DiagnosticsStatus): string {
  switch (status) {
    case 'fail':
      return 'error';
    case 'warn':
      return 'warning';
    default:
      return 'check_circle';
  }
}

function statusColor(status: DiagnosticsStatus): string {
  switch (status) {
    case 'fail':
      return 'negative';
    case 'warn':
      return 'warning';
    default:
      return 'positive';
  }
}

function statusText(status: DiagnosticsStatus): string {
  switch (status) {
    case 'fail':
      return t('settingsPage.diagnostics.statusFail');
    case 'warn':
      return t('settingsPage.diagnostics.statusWarn');
    default:
      return t('settingsPage.diagnostics.statusPass');
  }
}

function goDetail(item: DiagnosticsItem) {
  const ref = (item.detail_ref || '').trim();
  if (ref.startsWith('/')) {
    void router.push(ref);
  }
}

async function load() {
  loading.value = true;
  error.value = '';
  try {
    const report = await getDiagnostics();
    items.value = report.items ?? [];
    lastRunAt.value = new Date().toLocaleString();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

onMounted(load);
</script>

<style scoped lang="scss">
.runtime-diagnostics-panel {
  .section-heading {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }

  .diagnostics-list {
    border: 1px solid var(--glass-border);
    border-radius: 8px;
  }

  .diagnostics-item {
    min-height: 56px;
  }
}
</style>
