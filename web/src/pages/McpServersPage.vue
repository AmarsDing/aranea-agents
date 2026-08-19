<template>
  <q-page class="app-standard-page app-registry-page mcp-page">
    <AppPageHero
      kicker="Model Context Protocol"
      title="MCP 服务器"
      subtitle="管理 Model Context Protocol 服务器连接、传输配置与健康状态。列表支持服务端分页与搜索。"
    >
      <template #actions>
        <q-btn color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows(true)" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        placeholder="搜索服务器..."
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <div class="app-page-toolbar__meta">共 {{ total }} 个服务器，本页 {{ enabledCount }} 个已启用</div>
    </AppPageToolbar>

    <q-banner
      v-if="mcpGuideVisible"
      rounded
      dense
      class="bg-blue-1 text-grey-9 q-mb-md mcp-guide-banner"
    >
      <template #avatar>
        <q-icon name="hub" color="primary" />
      </template>
      MCP 工具需在各 Agent 的「工具设置」中启用 <code>mcp_tool_set</code> 后才会挂载到运行时；启用后所有「已启用」的服务器都会生效。服务器健康仅代表连通性，不代表已被 Agent 使用。
      <template #action>
        <q-btn flat dense round icon="close" aria-label="关闭提示" @click="dismissMcpGuide" />
      </template>
    </q-banner>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows(true)" />
      </template>
    </q-banner>

    <q-card v-if="!loading && filteredRows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-icon name="power" size="48px" color="grey-5" />
        <div class="text-subtitle1 text-weight-bold">暂无 MCP 服务器</div>
        <div class="text-caption text-grey-7">添加您的第一个 MCP 服务器以开始使用。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
      </q-card-section>
    </q-card>

    <template v-else>
      <McpServersTable
        :rows="pagedRows"
        :loading="loading"
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
        label="个 MCP 服务器"
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
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import McpServersTable from '../components/mcp/McpServersTable.vue';
import McpServerFormDialog from '../features/mcp/McpServerFormDialog.vue';
import McpUserCredentialDialog from '../features/mcp/McpUserCredentialDialog.vue';
import { useMcpServersPage } from '../features/mcp/useMcpServersPage';
import { useMcpGuide } from '../features/mcp/useMcpGuide';

const { mcpGuideVisible, dismissMcpGuide } = useMcpGuide();

const {
  search,
  loading,
  error,
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
</script>
