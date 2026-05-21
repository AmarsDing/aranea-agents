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

const sampleText = ref("Aranea Agent Orchestrator");
const sampleToggle = ref(true);
const sampleSelect = ref("a");

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
