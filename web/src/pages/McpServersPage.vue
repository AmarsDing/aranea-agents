<template>
  <q-page class="app-page-cream mcp-page q-pa-sm q-pa-md-md">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Model Context Protocol</div>
        <h1 class="app-page-title">MCP 服务器</h1>
        <p class="app-page-subtitle">管理 Model Context Protocol 服务器连接、传输配置与健康状态。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat bordered class="mcp-toolbar q-mb-md">
      <q-card-section class="app-form-field-grid items-end">
        <q-input v-model="search" class="app-field-md" dense outlined clearable debounce="200" placeholder="搜索服务器...">
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <div class="text-caption text-grey-7">
          共 {{ filteredRows.length }} 个服务器，{{ enabledCount }} 个已启用
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card flat bordered class="mcp-list-card">
      <q-card-section v-if="loading" class="q-gutter-md">
        <q-skeleton v-for="item in 4" :key="item" type="rect" height="116px" />
      </q-card-section>

      <q-list v-else-if="filteredRows.length" padding separator>
        <q-item v-for="server in filteredRows" :key="server.id" class="mcp-list-item">
          <q-item-section side top>
            <span class="health-dot" :class="`health-dot--${healthTone(server)}`">
              <q-tooltip>{{ healthTooltip(server) }}</q-tooltip>
            </span>
          </q-item-section>
          <q-item-section>
            <McpServerItem
              :server="server"
              :testing="testingId === server.id"
              @edit="openEdit"
              @delete="confirmDelete"
              @test="testRow"
              @credentials="openCredentials"
            />
          </q-item-section>
        </q-item>
      </q-list>

      <q-card-section v-else class="mcp-empty">
        <q-icon name="power" size="48px" color="grey-5" />
        <div class="text-subtitle1 text-weight-bold">暂无 MCP 服务器</div>
        <div class="text-caption text-grey-7">添加您的第一个 MCP 服务器以开始使用。</div>
        <q-btn color="primary" rounded unelevated icon="add" label="添加服务器" @click="openCreate" />
      </q-card-section>
    </q-card>

    <McpServerFormDialog v-model="editorOpen" :row="editingRow" @saved="onSaved" @tested="loadRows" />
    <McpUserCredentialDialog
      v-model="credDialogOpen"
      :mcp-server-id="credServer?.id ?? ''"
      :server-label="credServer?.name || credServer?.key || ''"
      :user-id="credUserId"
      @saved="loadRows"
    />
  </q-page>
</template>

<script setup lang="ts">
import McpServerFormDialog from "../features/mcp/McpServerFormDialog.vue";
import McpUserCredentialDialog from "../features/mcp/McpUserCredentialDialog.vue";
import McpServerItem from "../features/mcp/McpServerItem.vue";
import { useMcpServersPage } from "../features/mcp/useMcpServersPage";

const {
  search,
  loading,
  error,
  testingId,
  editorOpen,
  editingRow,
  credDialogOpen,
  credServer,
  credUserId,
  enabledCount,
  filteredRows,
  loadRows,
  openCreate,
  openEdit,
  openCredentials,
  onSaved,
  testRow,
  confirmDelete,
  healthTone,
  healthTooltip
} = useMcpServersPage();
</script>

<style scoped>
.mcp-toolbar,
.mcp-list-card {
  border-radius: 18px;
}

.mcp-list-item {
  align-items: flex-start;
  padding-left: 10px;
  padding-right: 10px;
}

.health-dot {
  border-radius: 999px;
  display: inline-block;
  height: 10px;
  margin-top: 22px;
  width: 10px;
}

.health-dot--ok {
  background: var(--color-quasar-positive);
}

.health-dot--error {
  background: var(--color-quasar-negative);
}

.health-dot--degraded {
  background: var(--color-quasar-warning);
}

.health-dot--unknown {
  background: var(--color-quasar-grey);
}

.mcp-empty {
  place-items: center center;
  color: var(--color-text-tertiary);
  display: grid;
  gap: 8px;
  min-height: 240px;
}
</style>
