<template>
  <q-page class="theme-preview-page q-pa-md">
    <div class="theme-preview-page__shell">
      <header class="theme-preview-hero q-mb-lg">
        <div class="theme-preview-kicker">Design System</div>
        <h1 class="theme-preview-title">Theme Preview</h1>
        <p class="theme-preview-subtitle">
          开发专用：验证昼夜 token、排版阶梯与 Quasar 组件映射。权威规范见
          <code>aranea-frontend-guide SKILL §6</code>。
        </p>
      </header>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Canvas &amp; Surfaces</h2>
        <div class="row q-col-gutter-md">
          <div v-for="swatch in surfaceSwatches" :key="swatch.label" class="col-6 col-sm-4 col-md-3">
            <div class="theme-swatch" :class="swatch.class">
              <span class="theme-swatch__label">{{ swatch.label }}</span>
            </div>
          </div>
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Typography</h2>
        <q-card flat class="theme-preview-card q-pa-md">
          <p class="theme-type theme-type--xl">XL — 25px 页面大标题</p>
          <p class="theme-type theme-type--lg">LG — 20px 区块标题</p>
          <p class="theme-type theme-type--md">MD — 16px 小节标题</p>
          <p class="theme-type theme-type--base">Base — 14px 正文默认</p>
          <p class="theme-type theme-type--sm">SM — 13px 次要说明</p>
          <p class="theme-type theme-type--xs">XS — 12px 标签 / 时间戳</p>
          <p class="theme-type theme-type--secondary q-mt-md">Secondary — var(--color-text-secondary)</p>
        </q-card>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Buttons</h2>
        <div class="row q-gutter-sm items-center">
          <q-btn unelevated no-caps color="primary" label="Primary" />
          <q-btn outline no-caps color="primary" label="Outline" />
          <q-btn flat no-caps color="primary" label="Flat" />
          <q-btn unelevated no-caps disable color="primary" label="Disabled" />
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Semantic Colors</h2>
        <div class="row q-col-gutter-sm">
          <q-chip
            v-for="chip in semanticChips"
            :key="chip.label"
            :color="chip.color"
            text-color="white"
            :label="chip.label"
          />
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Form Controls</h2>
        <q-card flat class="theme-preview-card q-pa-md">
          <div class="row q-col-gutter-md">
            <div class="col-12 col-md-6">
              <q-input v-model="sampleText" outlined dense label="Outlined input" />
            </div>
            <div class="col-12 col-md-6">
              <q-select
                v-model="sampleSelect"
                outlined
                dense
                emit-value
                map-options
                :options="selectOptions"
                label="Select"
              />
            </div>
            <div class="col-12">
              <q-toggle v-model="sampleToggle" color="primary" label="Toggle primary" />
            </div>
          </div>
        </q-card>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Glass Card</h2>
        <q-card flat class="theme-preview-card q-pa-md">
          <div class="text-subtitle2 text-weight-medium">PanelCard 风格</div>
          <p class="theme-type theme-type--base q-mt-sm q-mb-none">
            浮层使用 backdrop-filter + 半透明底 + 细边框，不靠重投影分层层级。
          </p>
        </q-card>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Form Layout（模板 A）</h2>
        <div class="app-settings-section">
          <h3 class="app-settings-section__title">配置区块示例</h3>
          <div class="app-form-field-grid q-mb-md">
            <q-input v-model="sampleText" outlined dense label="字段 1" />
            <q-input v-model="sampleText" outlined dense label="字段 2" />
            <q-input v-model="sampleText" outlined dense label="字段 3" />
          </div>
          <div class="app-actions-bar app-actions-bar--start">
            <q-btn outline no-caps color="primary" label="取消" />
            <q-btn unelevated no-caps color="primary" label="保存" />
          </div>
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Dialog Glass（模板 C）</h2>
        <q-btn outline no-caps color="primary" label="打开 Dialog 预览" @click="dialogOpen = true" />
        <q-dialog v-model="dialogOpen">
          <q-card class="app-dialog-card app-dialog-card--sm">
            <q-card-section>
              <div class="text-h6">Dialog 玻璃卡</div>
              <p class="theme-type theme-type--sm q-mt-sm q-mb-none">--glass-elevated + blur-elevated + 右对齐操作</p>
            </q-card-section>
            <q-card-actions align="right" class="app-actions-bar">
              <q-btn v-close-popup flat no-caps label="取消" />
              <q-btn v-close-popup unelevated no-caps color="primary" label="确认" />
            </q-card-actions>
          </q-card>
        </q-dialog>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Dashboard KPI Grid</h2>
        <div class="app-metrics-grid">
          <q-card
            v-for="kpi in dashboardKpis"
            :key="kpi.label"
            flat
            class="theme-preview-card app-metrics-grid__item q-pa-md"
          >
            <div class="text-caption text-grey-7">{{ kpi.label }}</div>
            <div class="text-h5 text-weight-bold q-mt-xs">{{ kpi.value }}</div>
            <div class="text-caption" :class="kpi.tone">{{ kpi.caption }}</div>
          </q-card>
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Chat Template B</h2>
        <q-card flat class="theme-preview-card q-pa-md">
          <q-banner rounded dense class="app-banner-warning q-mb-md">
            <template #avatar><q-icon name="gpp_maybe" color="warning" /></template>
            工具调用需用户确认 — Composer 限宽与警告条 token 预览
          </q-banner>
          <div class="chat-composer-inner">
            <q-input v-model="sampleText" outlined dense placeholder="输入消息…（max-width: composer-max-width）" />
          </div>
        </q-card>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Registry List（模板 C）</h2>
        <p class="theme-type theme-type--sm q-mb-md">
          Hero → 筛选面板 → 玻璃表格壳；列宽见 <code>registryTableColumns.ts</code>
        </p>
        <div class="app-registry-page theme-preview-embed">
          <section class="app-page-hero q-mb-md">
            <div>
              <div class="app-page-kicker">Registry preview</div>
              <h3 class="app-page-title theme-preview-embed__title">实体列表示例</h3>
              <p class="app-page-subtitle">紧凑 dense 表格 + 固定列宽 + 单元格 typography token</p>
            </div>
            <div class="app-actions-bar">
              <q-btn outline rounded no-caps color="primary" icon="history" label="运行记录" />
              <q-btn color="primary" unelevated rounded no-caps icon="refresh" label="刷新" />
            </div>
          </section>
          <q-card flat class="app-registry-panel">
            <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
              <q-input v-model="sampleText" class="app-field-md" outlined dense label="搜索" />
              <q-select
                v-model="sampleSelect"
                class="app-field-sm"
                outlined
                dense
                emit-value
                map-options
                :options="selectOptions"
                label="状态"
              />
            </q-card-section>
          </q-card>
          <AppRegistryTable
            class="q-mt-md"
            :rows="registryPreviewRows"
            :columns="registryPreviewColumns"
            row-key="id"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-name="props">
              <q-td :props="props">
                <AppRegistryHoverTip :text="props.row.desc" empty-label="暂无说明">
                  <div class="min-width-0">
                    <div class="app-registry-cell-primary">{{ props.row.name }}</div>
                    <div class="app-registry-cell-sub">{{ props.row.key }}</div>
                  </div>
                </AppRegistryHoverTip>
              </q-td>
            </template>
            <template #body-cell-status="props">
              <q-td :props="props">
                <q-chip dense :color="props.row.statusColor" text-color="white">{{ props.row.status }}</q-chip>
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props">
                <div class="app-registry-cell-actions">
                  <q-btn flat dense round color="primary" icon="visibility" />
                  <q-btn flat dense round color="primary" icon="settings" />
                </div>
              </q-td>
            </template>
          </AppRegistryTable>
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Data Table Shell（legacy alias）</h2>
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          :rows="tablePreviewRows"
          :columns="tablePreviewColumns"
          row-key="id"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        />
      </section>

      <section class="theme-preview-section">
        <h2 class="theme-preview-section__title">Spacing Scale</h2>
        <div class="theme-spacing-row">
          <div
            v-for="space in spacingScale"
            :key="space.token"
            class="theme-spacing-block"
            :style="{ width: space.value }"
          >
            {{ space.token }}
          </div>
        </div>
      </section>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../components/layout/AppRegistryHoverTip.vue';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../features/ui/registryTableColumns';

const sampleText = ref('Aranea Agent Orchestrator');
const sampleToggle = ref(true);
const sampleSelect = ref('a');
const dialogOpen = ref(false);

const selectOptions = [
  { label: 'Option A', value: 'a' },
  { label: 'Option B', value: 'b' },
];

const surfaceSwatches = [
  { label: 'canvas-base', class: 'theme-swatch--canvas' },
  { label: 'glass-surface', class: 'theme-swatch--glass' },
  { label: 'glass-elevated', class: 'theme-swatch--elevated' },
  { label: 'accent', class: 'theme-swatch--accent' },
];

const semanticChips = [
  { label: 'success', color: 'positive' },
  { label: 'warning', color: 'warning' },
  { label: 'danger', color: 'negative' },
  { label: 'info', color: 'info' },
];

const spacingScale = [
  { token: '4', value: 'var(--space-1)' },
  { token: '8', value: 'var(--space-2)' },
  { token: '12', value: 'var(--space-3)' },
  { token: '16', value: 'var(--space-4)' },
  { token: '24', value: 'var(--space-6)' },
  { token: '32', value: 'var(--space-8)' },
];

const dashboardKpis = [
  { label: '今日调用', value: '1,284', caption: '较昨日 +12.3%', tone: 'text-positive' },
  { label: '今日 Token', value: '2.4M', caption: '输入 1.8M / 输出 0.6M', tone: 'text-grey-7' },
  { label: '今日费用', value: '$0.0421', caption: '较昨日 -5.1%', tone: 'text-negative' },
  { label: '平均延迟', value: '842 ms', caption: '成功率 98%', tone: 'text-grey-7' },
];

const registryPreviewColumns = [
  registryCol('name', '名称', 'name', 'left', '20%'),
  registryCol('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryColActions(),
];

const registryPreviewRows = [
  {
    id: '1',
    name: 'Audit Log',
    key: 'audit_log',
    desc: '记录 Agent 生命周期事件与工具调用摘要',
    status: 'active',
    statusColor: 'positive',
  },
  {
    id: '2',
    name: 'Rate Limiter',
    key: 'rate_limit',
    desc: '按 Agent 维度限制并发与 QPS',
    status: 'paused',
    statusColor: 'grey-7',
  },
];

const tablePreviewColumns = [
  registryCol('name', '名称', 'name', 'left', REGISTRY_COL_W.name),
  registryCol('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol('updated', '更新时间', 'updated', 'left', REGISTRY_COL_W.time),
];

const tablePreviewRows = [
  { id: '1', name: 'Agent Alpha', status: 'active', updated: '2026-05-21 10:30' },
  { id: '2', name: 'Team Research', status: 'running', updated: '2026-05-21 09:15' },
];
</script>
