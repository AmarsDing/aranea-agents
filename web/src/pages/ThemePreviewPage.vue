<template>
  <q-page class="theme-preview-page q-pa-md">
    <div class="theme-preview-page__shell">
      <header class="theme-preview-hero q-mb-lg">
        <div class="theme-preview-kicker">Design System</div>
        <h1 class="theme-preview-title">Theme Preview</h1>
        <p class="theme-preview-subtitle">
          开发专用：验证昼夜 token、排版阶梯与 Quasar 组件映射。权威规范见
          <code>docs/frontend/UX.md</code>。
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
          <q-chip v-for="chip in semanticChips" :key="chip.label" :color="chip.color" text-color="white" :label="chip.label" />
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
              <q-select v-model="sampleSelect" outlined dense emit-value map-options :options="selectOptions" label="Select" />
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
              <q-btn flat no-caps label="取消" v-close-popup />
              <q-btn unelevated no-caps color="primary" label="确认" v-close-popup />
            </q-card-actions>
          </q-card>
        </q-dialog>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Dashboard KPI Grid</h2>
        <div class="app-metrics-grid">
          <q-card v-for="kpi in dashboardKpis" :key="kpi.label" flat class="theme-preview-card app-metrics-grid__item q-pa-md">
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
        <p class="theme-type theme-type--sm q-mb-md">Hero → 筛选面板 → 玻璃表格壳；列宽见 <code>registryTableColumns.ts</code></p>
        <div class="app-registry-page" style="padding: 0">
          <section class="app-page-hero q-mb-md">
            <div>
              <div class="app-page-kicker">Registry preview</div>
              <h3 class="app-page-title" style="font-size: var(--text-lg)">实体列表示例</h3>
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
              <q-select v-model="sampleSelect" class="app-field-sm" outlined dense emit-value map-options :options="selectOptions" label="状态" />
            </q-card-section>
          </q-card>
          <div class="app-registry-table-shell q-mt-md">
            <q-table flat dense class="app-registry-table" :rows="registryPreviewRows" :columns="registryPreviewColumns" row-key="id" hide-pagination>
              <template #body-cell-name="props">
                <q-td :props="props">
                  <div class="app-registry-cell-primary">{{ props.row.name }}</div>
                  <div class="app-registry-cell-sub">{{ props.row.key }}</div>
                </q-td>
              </template>
              <template #body-cell-desc="props">
                <q-td :props="props">
                  <div class="app-registry-cell-desc">{{ props.row.desc }}</div>
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
            </q-table>
          </div>
        </div>
      </section>

      <section class="theme-preview-section q-mb-lg">
        <h2 class="theme-preview-section__title">Data Table Shell（legacy alias）</h2>
        <div class="app-data-table-shell">
          <q-table flat dense class="app-registry-table" :rows="tablePreviewRows" :columns="tablePreviewColumns" row-key="id" hide-pagination />
        </div>
      </section>

      <section class="theme-preview-section">
        <h2 class="theme-preview-section__title">Spacing Scale</h2>
        <div class="theme-spacing-row">
          <div v-for="space in spacingScale" :key="space.token" class="theme-spacing-block" :style="{ width: space.value }">
            {{ space.token }}
          </div>
        </div>
      </section>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { registryCol } from "../features/ui/registryTableColumns";

const sampleText = ref("Aranea Agent Orchestrator");
const sampleToggle = ref(true);
const sampleSelect = ref("a");
const dialogOpen = ref(false);

const selectOptions = [
  { label: "Option A", value: "a" },
  { label: "Option B", value: "b" }
];

const surfaceSwatches = [
  { label: "canvas-base", class: "theme-swatch--canvas" },
  { label: "glass-surface", class: "theme-swatch--glass" },
  { label: "glass-elevated", class: "theme-swatch--elevated" },
  { label: "accent", class: "theme-swatch--accent" }
];

const semanticChips = [
  { label: "success", color: "positive" },
  { label: "warning", color: "warning" },
  { label: "danger", color: "negative" },
  { label: "info", color: "info" }
];

const spacingScale = [
  { token: "4", value: "var(--space-1)" },
  { token: "8", value: "var(--space-2)" },
  { token: "12", value: "var(--space-3)" },
  { token: "16", value: "var(--space-4)" },
  { token: "24", value: "var(--space-6)" },
  { token: "32", value: "var(--space-8)" }
];

const dashboardKpis = [
  { label: "今日调用", value: "1,284", caption: "较昨日 +12.3%", tone: "text-positive" },
  { label: "今日 Token", value: "2.4M", caption: "输入 1.8M / 输出 0.6M", tone: "text-grey-7" },
  { label: "今日费用", value: "$0.0421", caption: "较昨日 -5.1%", tone: "text-negative" },
  { label: "平均延迟", value: "842 ms", caption: "成功率 98%", tone: "text-grey-7" }
];

const registryPreviewColumns = [
  { name: "name", label: "名称", field: "name", align: "left" as const, ...registryCol.name },
  { name: "desc", label: "说明", field: "desc", align: "left" as const, ...registryCol.desc },
  { name: "status", label: "状态", field: "status", align: "left" as const, ...registryCol.status },
  { name: "actions", label: "操作", field: "id", align: "right" as const, ...registryCol.actions }
];

const registryPreviewRows = [
  { id: "1", name: "Audit Log", key: "audit_log", desc: "记录 Agent 生命周期事件与工具调用摘要", status: "active", statusColor: "positive" },
  { id: "2", name: "Rate Limiter", key: "rate_limit", desc: "按 Agent 维度限制并发与 QPS", status: "paused", statusColor: "grey-7" }
];

const tablePreviewColumns = [
  { name: "name", label: "名称", field: "name", align: "left" as const },
  { name: "status", label: "状态", field: "status", align: "left" as const },
  { name: "updated", label: "更新时间", field: "updated", align: "left" as const }
];

const tablePreviewRows = [
  { id: "1", name: "Agent Alpha", status: "active", updated: "2026-05-21 10:30" },
  { id: "2", name: "Team Research", status: "running", updated: "2026-05-21 09:15" }
];
</script>

<style scoped lang="sass">
.theme-preview-page
  color: var(--color-text-primary)
  min-height: 100%

.theme-preview-page__shell
  max-width: var(--layout-max-width)
  margin: 0 auto
  padding: var(--space-3) var(--space-3) var(--space-8)

.theme-preview-kicker
  display: inline-flex
  padding: 5px 11px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 700
  letter-spacing: 0.08em
  text-transform: uppercase
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  color: var(--color-text-secondary)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.theme-preview-title
  margin: 10px 0 0
  font-size: var(--text-xl)
  font-weight: 700
  letter-spacing: -0.03em
  line-height: 1.15
  color: var(--color-text-primary)

body.body--dark .theme-preview-title
  text-shadow: 0 0 12px rgba(0, 229, 255, 0.12)

.theme-preview-subtitle
  margin: var(--space-2) 0 0
  font-size: var(--text-base)
  line-height: 1.55
  color: var(--color-text-secondary)

.theme-preview-section__title
  margin: 0 0 var(--space-3)
  font-size: var(--text-lg)
  font-weight: 600
  color: var(--color-text-primary)

.theme-preview-card
  border-radius: 18px !important
  background: var(--glass-surface) !important
  border: 1px solid var(--glass-border) !important
  box-shadow: var(--glass-inner-highlight) !important
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.theme-swatch
  min-height: 72px
  border-radius: 14px
  border: 1px solid var(--glass-border)
  display: flex
  align-items: flex-end
  padding: var(--space-2)

.theme-swatch__label
  font-size: var(--text-xs)
  color: var(--color-text-secondary)

.theme-swatch--canvas
  background: var(--canvas-base)

.theme-swatch--glass
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.theme-swatch--elevated
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))

.theme-swatch--accent
  background: var(--color-accent)
  color: var(--color-on-accent)

  .theme-swatch__label
    color: var(--color-on-accent)

.theme-type--xl
  font-size: var(--text-xl)
  line-height: 1.25

.theme-type--lg
  font-size: var(--text-lg)
  line-height: 1.25

.theme-type--md
  font-size: var(--text-md)
  line-height: 1.35

.theme-type--base
  font-size: var(--text-base)
  line-height: 1.5

.theme-type--sm
  font-size: var(--text-sm)
  line-height: 1.5

.theme-type--xs
  font-size: var(--text-xs)
  line-height: 1.5

.theme-type--secondary
  color: var(--color-text-secondary)

.theme-spacing-row
  display: flex
  flex-wrap: wrap
  gap: var(--space-2)
  align-items: flex-end

.theme-spacing-block
  height: 32px
  min-width: 32px
  border-radius: 8px
  background: var(--glass-surface)
  border: 1px solid var(--glass-border)
  display: flex
  align-items: center
  justify-content: center
  font-size: var(--text-xs)
  color: var(--color-text-secondary)
</style>
