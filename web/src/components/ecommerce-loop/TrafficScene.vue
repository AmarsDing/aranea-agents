<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatRate, PRODUCT_METRICS } from '../../features/ecommerce-loop/data';
import type { DemoWorkflow, ReferenceFocus } from '../../features/ecommerce-loop/types';
import AppRegistryMarkupTable from '../layout/AppRegistryMarkupTable.vue';
import ReferenceIcon from './ReferenceIcon.vue';
import ReferenceCopy from './ReferenceCopy.vue';
import { buildProductColumns } from './productTableUi';
const props = defineProps<{ workflow: DemoWorkflow; selectedProduct: string; referenceFocus: ReferenceFocus | null }>();
const emit = defineEmits<{
  (event: 'decide', decision: 'approved' | 'rejected' | 'revise'): void;
  (event: 'select-product', id: string): void;
  (event: 'operate'): void;
}>();
const { t } = useI18n();
const columns = computed(() => buildProductColumns(t));
const rows = [
  { id: 'yesterday', tone: 'ai' },
  { id: 'best', tone: 'ai' },
  { id: 'normal', tone: 'muted' },
  { id: 'anomaly', tone: 'danger' },
  { id: 'suggestion', tone: 'working' },
];
function panelDimmed(panel: 'funnel' | 'table' | 'report') {
  const focus = props.referenceFocus;
  if (!focus) return false;
  if (focus === 'funnel-overview' || focus === 'funnel') return panel !== 'funnel';
  if (focus === 'table') return panel !== 'table';
  return panel !== 'report';
}
function rowDimmed(id: string) {
  const focus = props.referenceFocus;
  if (focus === 'report') return ['anomaly', 'suggestion'].includes(id);
  if (focus === 'anomaly' || focus === 'suggestion') return id !== focus;
  return false;
}
</script>

<template>
  <section class="ec-traffic-scene">
    <div class="ec-scene-title">
      <span class="ec-outline-tag">{{ t('ecommerceLoop.traffic.tag') }}</span>
      <h1>
        {{ t('ecommerceLoop.traffic.title') }}<strong>{{ t('ecommerceLoop.traffic.accent') }}</strong>
      </h1>
    </div>
    <span class="ec-example-notice">{{ t('ecommerceLoop.traffic.notice') }}</span>
    <section
      class="ec-funnel ec-panel ec-tone-data"
      :class="{ 'ec-traffic-dim': panelDimmed('funnel') }"
      :inert="panelDimmed('funnel')"
    >
      <h2>
        <q-icon name="filter_alt" />{{ t('ecommerceLoop.traffic.funnel')
        }}<small>{{ t('ecommerceLoop.traffic.yesterday') }}</small>
      </h2>
      <div class="ec-funnel-bars">
        <div class="ec-funnel-bar ec-tone-data">
          <span>{{ t('ecommerceLoop.traffic.visitors') }}</span
          ><small>{{ t('ecommerceLoop.traffic.change') }}</small
          ><b>1,240</b>
        </div>
        <p>{{ t('ecommerceLoop.traffic.cartRate') }}</p>
        <div class="ec-funnel-bar ec-tone-ai">
          <span>{{ t('ecommerceLoop.traffic.cart') }}</span
          ><b>74</b>
        </div>
        <p>{{ t('ecommerceLoop.traffic.orderRate') }}</p>
        <div class="ec-funnel-bar ec-tone-owner">
          <span>{{ t('ecommerceLoop.traffic.orders') }}</span
          ><b>31</b>
        </div>
      </div>
    </section>
    <section
      class="ec-breakdown ec-panel ec-tone-ai"
      :class="{ 'ec-traffic-dim': panelDimmed('table') }"
      :inert="panelDimmed('table')"
    >
      <h2>
        <q-icon name="format_list_bulleted" />{{ t('ecommerceLoop.traffic.breakdown')
        }}<small>{{ t('ecommerceLoop.traffic.yesterday') }}</small>
      </h2>
      <AppRegistryMarkupTable :rows="PRODUCT_METRICS" :columns="columns" table-class="ec-product-table">
        <template #cell-name="{ row }"
          ><button
            :class="{ 'ec-product-anomaly': row.id === 'sea-salt', 'is-selected': row.id === selectedProduct }"
            @click="emit('select-product', String(row.id))"
          >
            {{ t(`ecommerceLoop.products.${row.nameKey}`) }}
          </button></template
        >
        <template #cell-rate="{ row }"
          ><span :class="{ 'ec-anomaly-rate': row.id === 'sea-salt' }">{{
            formatRate(Number(row.addToCart), Number(row.visitors))
          }}</span
          ><small v-if="row.id === 'sea-salt'">{{ t('ecommerceLoop.traffic.previous') }}</small></template
        >
      </AppRegistryMarkupTable>
    </section>
    <section
      class="ec-report ec-panel ec-tone-ai"
      :class="{ 'ec-traffic-dim': panelDimmed('report') }"
      :inert="panelDimmed('report')"
    >
      <h2>
        <q-icon name="format_align_left" />{{ t('ecommerceLoop.traffic.report')
        }}<small>{{ t('ecommerceLoop.traffic.automatic') }}</small
        ><span class="ec-ai-badge">AI</span>
      </h2>
      <div class="ec-report-rows">
        <div
          v-for="row in rows"
          :key="row.id"
          :data-report-row="row.id"
          class="ec-report-row"
          :class="[`ec-tone-${row.tone}`, { 'ec-traffic-dim': rowDimmed(row.id) }]"
        >
          <b>{{ t(`ecommerceLoop.report.${row.id}.label`) }}</b>
          <p><ReferenceCopy :text="t(`ecommerceLoop.report.${row.id}.text`)" :anomaly="row.id === 'anomaly'" /></p>
        </div>
      </div>
      <div
        class="ec-report-decision"
        :class="{ 'ec-traffic-dim': referenceFocus === 'report' || referenceFocus === 'anomaly' }"
        :inert="referenceFocus === 'report' || referenceFocus === 'anomaly'"
      >
        <ReferenceIcon /><span>{{ t('ecommerceLoop.traffic.ceo') }}</span
        ><template v-if="workflow.phase === 'awaiting_decision'"
          ><button
            v-for="decision in ['approve', 'reject', 'revise']"
            :key="decision"
            @click="emit('decide', decision === 'approve' ? 'approved' : decision === 'reject' ? 'rejected' : 'revise')"
          >
            {{ t(`ecommerceLoop.traffic.${decision}`) }}
          </button></template
        ><button v-else @click="emit('operate')">{{ t(`ecommerceLoop.phases.${workflow.phase}`) }}</button>
      </div>
    </section>
  </section>
</template>
