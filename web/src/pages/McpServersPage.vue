<template>
  <q-page class="app-standard-page cap-page mcp-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadRows(true)"
      />
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('cap.mcp.create')" @click="openCreate" />
    </Teleport>

    <template v-if="total > 0 || search">
      <CapStats :stats="stats" :label="t('cap.common.statsLabel')" />

      <!-- 有可用服务器却没有 Agent 开 MCP：服务器不会被挂到任何运行时 -->
      <p v-if="showUnusedWarning" class="cap-note" role="note">
        <q-icon name="warning_amber" size="16px" aria-hidden="true" />
        <span class="cap-note__body">{{ t('shell.mcp.unusedWarning') }}</span>
      </p>
      <p v-else-if="mcpGuideVisible" class="cap-note cap-note--info" role="note">
        <q-icon name="info" size="16px" aria-hidden="true" />
        <span class="cap-note__body">{{ t('shell.mcp.guide') }}</span>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="close"
          :aria-label="t('shell.mcp.dismissGuide')"
          @click="dismissMcpGuide"
        />
      </p>

      <div class="cap-bar" role="search">
        <q-input
          v-model="search"
          class="cap-bar__search"
          dense
          outlined
          clearable
          debounce="200"
          :placeholder="t('shell.mcp.searchPlaceholder')"
          :aria-label="t('shell.mcp.searchPlaceholder')"
        >
          <template #prepend><q-icon name="search" size="18px" /></template>
        </q-input>
      </div>
    </template>

    <AppStatePanel v-if="pageState === 'loading'" variant="loading" :rows="5" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      variant="error"
      :title="error"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="loadRows(true)"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel
      v-else-if="pageState === 'empty' && search"
      variant="empty"
      icon="hub"
      :title="t('cap.mcp.emptyFiltered')"
      :action-label="t('cap.common.reset')"
      action-icon="filter_alt_off"
      @action="search = ''"
    />
    <!-- 还没有服务器：讲清楚 MCP 是什么、接进来之后 Agent 怎么用上 -->
    <section v-else-if="pageState === 'empty'" class="cap-section" :aria-label="t('cap.mcp.emptyTitle')">
      <div class="cap-section__head">
        <span class="cap-card__icon" aria-hidden="true"><q-icon name="hub" size="18px" /></span>
        <div class="cap-section__text">
          <h2 class="cap-section__title">{{ t('cap.mcp.emptyTitle') }}</h2>
          <p class="cap-section__hint">{{ t('cap.mcp.emptyDesc') }}</p>
        </div>
      </div>
      <ol class="cap-ways" role="list">
        <li>
          <q-icon name="add_link" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.mcp.step1') }}</strong>
          <span>{{ t('cap.mcp.step1Desc') }}</span>
          <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('cap.mcp.create')" @click="openCreate" />
        </li>
        <li>
          <q-icon name="science" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.mcp.step2') }}</strong>
          <span>{{ t('cap.mcp.step2Desc') }}</span>
        </li>
        <li>
          <q-icon name="smart_toy" size="22px" aria-hidden="true" />
          <strong>{{ t('cap.mcp.step3') }}</strong>
          <span>{{ t('cap.mcp.step3Desc') }}</span>
          <!-- 有 Agent 已经开了 MCP 但还没有服务器：说清现在的实际效果，免得把这个数当成「在用」 -->
          <span v-if="usageEnabledAgents" class="cap-ways__path">
            {{ t('cap.mcp.optedInNoServer', { n: usageEnabledAgents, total: usageTotalAgents ?? '—' }) }}
          </span>
        </li>
      </ol>
    </section>

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        variant="partial"
        dense
        :description="error"
        :action-label="t('shell.common.retry')"
        action-icon="refresh"
        @action="loadRows(true)"
      />
      <McpServersTable
        :rows="pagedRows"
        :loading="loading"
        :total="total"
        :testing-id="testingId"
        :toggling-id="togglingId"
        :health-tone="healthTone"
        :health-tooltip="healthTooltip"
        @edit="openEdit"
        @delete="confirmDelete"
        @test="testRow"
        @credentials="openCredentials"
        @toggle-enabled="toggleEnabled"
      />

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="total"
        :loading="loading"
        :label="t('shell.mcp.paginationUnit')"
        :page-size-options="[10, 20, 50]"
      />
    </template>

    <McpServerFormDialog v-model="editorOpen" :row="editingRow" @saved="onSaved" @tested="loadRows" />
    <McpUserCredentialDialog
      v-model="credDialogOpen"
      :mcp-server-id="credServer?.id ?? ''"
      :server-label="credServer?.name || credServer?.key || ''"
      :user-id="credUserId"
      :user-label="credUserLabel"
      @saved="loadRows"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapTone } from '../components/capability/capTypes';
import McpServersTable from '../components/mcp/McpServersTable.vue';
import McpServerFormDialog from '../features/mcp/McpServerFormDialog.vue';
import McpUserCredentialDialog from '../features/mcp/McpUserCredentialDialog.vue';
import { useMcpServersPage } from '../features/mcp/useMcpServersPage';
import { useMcpGuide } from '../features/mcp/useMcpGuide';
import { parseJSON } from '../features/mcp/utils';
import type { McpServerMetadata } from '../features/mcp/types';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const { mcpGuideVisible, dismissMcpGuide } = useMcpGuide();

const {
  search,
  loading,
  error,
  forbidden,
  testingId,
  togglingId,
  editorOpen,
  editingRow,
  credDialogOpen,
  credServer,
  credUserId,
  credUserLabel,
  enabledCount,
  filteredRows,
  usageEnabledAgents,
  usageTotalAgents,
  usageSummaryText,
  showUnusedWarning,
  total,
  page,
  pageSize,
  pageMax,
  pagedRows,
  loadRows,
  openCreate,
  openEdit,
  openCredentials,
  onSaved,
  testRow,
  toggleEnabled,
  confirmDelete,
  healthTone,
  healthTooltip,
} = useMcpServersPage();

// 健康与工具数按当前页统计；采纳数（多少 Agent 开了 MCP）来自后端汇总
const stats = computed(() => {
  const metas = pagedRows.value.map((row) => parseJSON<McpServerMetadata>(row.metadata_json, {}));
  const healthy = metas.filter((m) => m.health_status === 'ok').length;
  const broken = metas.filter((m) => m.health_status === 'error').length;
  const tools = metas.reduce((n, m) => n + (typeof m.tool_count === 'number' ? m.tool_count : 0), 0);
  const adoption =
    usageEnabledAgents.value == null || usageTotalAgents.value == null
      ? '—'
      : `${usageEnabledAgents.value} / ${usageTotalAgents.value}`;
  return [
    { key: 'total', label: t('cap.mcp.stats.total'), value: total.value },
    { key: 'enabled', label: t('cap.mcp.stats.enabled'), value: enabledCount.value },
    {
      key: 'health',
      label: t('cap.mcp.stats.health'),
      value: `${healthy} / ${pagedRows.value.length}`,
      tone: (broken > 0 ? 'alert' : healthy === pagedRows.value.length ? 'ok' : 'pending') as CapTone,
    },
    { key: 'tools', label: t('cap.mcp.stats.tools'), value: tools },
    {
      key: 'agents',
      label: t('cap.mcp.stats.agents'),
      value: adoption,
      hint: usageSummaryText.value,
      tone: (showUnusedWarning.value ? 'watch' : undefined) as CapTone | undefined,
    },
  ];
});

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: filteredRows.value.length,
    forbidden: forbidden.value,
  }),
);
</script>
