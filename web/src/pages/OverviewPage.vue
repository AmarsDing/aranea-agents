<template>
  <!--
    Harness · 大屏显示（2026-11-19 由「基座总览」改成）：一屏放下，主看各模型 / 各工具的消耗、费用、积分与额度，
    次看执行任务量。设计见 docs/aranea-agents/design/2026-11-17-三大模块重设计/04-Harness系统.md §4.1（v3）。
  -->
  <q-page class="app-standard-page ov-page hs-page">
    <div class="hs-bar">
      <q-btn-toggle
        :model-value="range"
        dense
        no-caps
        unelevated
        toggle-color="primary"
        :options="rangeOptions"
        :aria-label="t('harnessScreen.rangeAria')"
        @update:model-value="setRange"
      />
      <span class="hs-bar__note">{{ t('harnessScreen.utcNote', { time: dayStartLocal }) }}</span>
      <span class="hs-bar__updated" aria-live="polite">
        {{ updatedAt ? t('harnessScreen.updated', { time: clock(updatedAt) }) : t('harnessScreen.loading') }}
      </span>
      <q-btn
        flat
        round
        dense
        icon="refresh"
        :loading="loading"
        :aria-label="t('harnessScreen.refresh')"
        @click="refresh"
      />
    </div>

    <q-banner v-if="error" class="ov-error hs-error" role="alert">
      {{ t('harnessScreen.loadFailed') }}: {{ error }}
    </q-banner>

    <div class="hs-kpis">
      <OverviewStatTile
        v-for="k in kpis"
        :key="k.key"
        class="hs-kpi"
        :label="k.label"
        :value="k.value"
        :caption="k.caption"
        :delta="k.delta"
        :delta-good-when-negative="k.deltaGoodWhenNegative"
        :tone="k.tone"
        :spark="k.spark"
        :clickable="!!k.to"
        @drill="k.to && router.push(k.to)"
      />
    </div>

    <div class="hs-grid">
      <HsShareBoard
        v-model:mode="mode"
        :to="links.usage"
        class="hs-grid__share"
        :rows="shares.rows"
        :by-cost="shares.byCost"
        :total="totalCost"
      />
      <HsToolBoard class="hs-grid__tools" :rows="tools" :to="links.tools" />
      <HsCreditBoard
        class="hs-grid__quota"
        :rows="creditList"
        :loading="credits === undefined"
        :unavailable="credits === null"
        @edit="onEditCredit"
      />
      <HsTrendBoard
        :to="links.usage"
        class="hs-grid__trend"
        :labels="trendLabels"
        :calls="trendCalls"
        :costs-usd="trendCosts"
        :hourly="range === 'today'"
      />
      <HsTaskBoard class="hs-grid__task" :stats="runStats" :bars="taskBars" :total="taskTotal" :to="links.tasks" />
    </div>

    <HsCreditDialog v-model="editOpen" :form="editForm" :saving="saving" :error="saveError" @save="onSaveCredit" />
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useHarnessScreen } from '../features/usage/useHarnessScreen';
import OverviewStatTile from '../components/overview/OverviewStatTile.vue';
import HsShareBoard from '../components/screen/HsShareBoard.vue';
import HsToolBoard from '../components/screen/HsToolBoard.vue';
import HsCreditBoard from '../components/screen/HsCreditBoard.vue';
import HsCreditDialog from '../components/screen/HsCreditDialog.vue';
import HsTrendBoard from '../components/screen/HsTrendBoard.vue';
import HsTaskBoard from '../components/screen/HsTaskBoard.vue';
import type { HsCreditForm } from '../components/screen/hsTypes';

const {
  t,
  error,
  loading,
  updatedAt,
  range,
  rangeOptions,
  setRange,
  refresh,
  mode,
  kpis,
  shares,
  totalCost,
  tools,
  creditList,
  credits,
  creditForm,
  saveCredit,
  trendLabels,
  dayStartLocal,
  links,
  trendCalls,
  trendCosts,
  runStats,
  taskBars,
  taskTotal,
} = useHarnessScreen();
const router = useRouter();

const clock = (d: Date) => d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });

// —— 手填平台积分 ——
const editOpen = ref(false);
const editForm = ref<HsCreditForm>(creditForm('claude'));
const saving = ref(false);
const saveError = ref('');
function onEditCredit(platform: string) {
  editForm.value = creditForm(platform);
  saveError.value = '';
  editOpen.value = true;
}
async function onSaveCredit(form: HsCreditForm) {
  saving.value = true;
  saveError.value = '';
  try {
    await saveCredit(form);
    editOpen.value = false;
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : String(err);
  } finally {
    saving.value = false;
  }
}
</script>
