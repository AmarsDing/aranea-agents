<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { PRODUCT_METRICS, SHOPS, formatRate } from '../../features/ecommerce-loop/data';
import type { CaseId, DemoWorkflow, ShopId, WorkflowActionPayload } from '../../features/ecommerce-loop/types';
import AppRegistryMarkupTable from '../layout/AppRegistryMarkupTable.vue';
import WorkflowPanel from './WorkflowPanel.vue';
import ReferenceIcon from './ReferenceIcon.vue';
import { buildWorkbenchProductColumns } from './productTableUi';

const props = defineProps<{
  selectedShop: ShopId;
  selectedProduct: string;
  activeCase: CaseId;
  cases: Record<CaseId, DemoWorkflow>;
  editing: boolean;
  storageWarning: string;
}>();
const emit = defineEmits<{
  (event: 'select-shop', shop: ShopId): void;
  (event: 'select-product', product: string): void;
  (event: 'select-case', kind: CaseId): void;
  (event: 'action', payload: WorkflowActionPayload): void;
  (event: 'dirty', value: boolean): void;
  (event: 'focus-task'): void;
}>();
const { t } = useI18n();
const caseIds: CaseId[] = ['traffic', 'launch'];
const columns = computed(() => buildWorkbenchProductColumns(t));
const selected = computed(() => PRODUCT_METRICS.find((product) => product.id === props.selectedProduct)!);
const shopName = computed(() => t('ecommerceLoop.shop', { name: props.selectedShop.toUpperCase() }));
const pending = computed(
  () =>
    Object.values(props.cases).filter((flow) =>
      ['awaiting_decision', 'awaiting_publish_approval', 'rework_required'].includes(flow.phase),
    ).length,
);
const shopOptions = computed(() =>
  SHOPS.map((shop) => ({
    label: t('ecommerceLoop.shop', { name: shop.toUpperCase() }),
    value: shop,
  })),
);
</script>

<template>
  <section class="ec-workbench" :aria-label="t('ecommerceLoop.workbench.title')">
    <div class="ec-workbench-inner">
      <section class="ec-context-bar" :aria-label="t('ecommerceLoop.workbench.context')">
        <q-select
          :model-value="selectedShop"
          :options="shopOptions"
          :label="t('ecommerceLoop.selectShop')"
          emit-value
          map-options
          outlined
          dense
          @update:model-value="emit('select-shop', $event)"
        />
        <div class="ec-data-context">
          <span>{{ t('ecommerceLoop.workbench.period') }}</span>
          <small>{{ t('ecommerceLoop.workbench.source') }}</small>
        </div>
        <span class="ec-sample-badge"><q-icon name="science" />{{ t('ecommerceLoop.workbench.sample') }}</span>
      </section>
      <p v-if="storageWarning" class="ec-save-warning" role="status">
        {{ t(`ecommerceLoop.storage.${storageWarning}`) }}
      </p>

      <section class="ec-kpi-grid" :aria-label="t('ecommerceLoop.workbench.metrics')">
        <article class="ec-reading-card ec-kpi">
          <span>{{ t('ecommerceLoop.traffic.visitors') }}</span
          ><strong>1,240</strong>
          <small>{{ t('ecommerceLoop.traffic.change') }}</small>
        </article>
        <article class="ec-reading-card ec-kpi">
          <span>{{ t('ecommerceLoop.workbench.cartRatio') }}</span
          ><strong>6.0<small>%</small></strong>
          <small>74 / 1,240</small>
        </article>
        <article class="ec-reading-card ec-kpi">
          <span>{{ t('ecommerceLoop.traffic.orders') }}</span
          ><strong>31</strong>
          <small>{{ t('ecommerceLoop.workbench.orderRatio') }} 2.5%</small>
        </article>
        <article class="ec-reading-card ec-kpi ec-kpi--pending">
          <span>{{ t('ecommerceLoop.workbench.pending') }}</span
          ><strong
            >{{ pending }}<small>{{ t('ecommerceLoop.workbench.items') }}</small></strong
          >
          <q-btn
            flat
            dense
            no-caps
            icon-right="arrow_downward"
            :label="t('ecommerceLoop.workbench.viewTask')"
            @click="emit('focus-task')"
          />
        </article>
      </section>

      <nav class="ec-task-tabs" :aria-label="t('ecommerceLoop.workbench.tasks')">
        <button
          v-for="kind in caseIds"
          :key="kind"
          :aria-pressed="activeCase === kind"
          :class="{ 'is-active': activeCase === kind }"
          @click="emit('select-case', kind)"
        >
          <q-icon :name="kind === 'traffic' ? 'image_search' : 'inventory_2'" />
          <span
            ><b>{{ t(`ecommerceLoop.workflow.${kind}`) }}</b
            ><small>{{ t(`ecommerceLoop.phases.${cases[kind].phase}`) }}</small></span
          >
          <q-icon name="chevron_right" />
        </button>
      </nav>

      <div class="ec-workbench-grid">
        <div class="ec-evidence-column" :class="{ 'ec-evidence-column--traffic': activeCase === 'traffic' }">
          <template v-if="activeCase === 'traffic'">
            <section class="ec-reading-card">
              <div class="ec-reading-heading">
                <div>
                  <h2>{{ t('ecommerceLoop.workbench.storeSnapshot') }}</h2>
                  <p>{{ t('ecommerceLoop.workbench.metricNote') }}</p>
                </div>
                <q-icon name="filter_alt" />
              </div>
              <div class="ec-reading-funnel">
                <div>
                  <span>{{ t('ecommerceLoop.traffic.visitors') }}</span
                  ><b>1,240</b>
                </div>
                <p>{{ t('ecommerceLoop.workbench.cartRatio') }} · 6.0%</p>
                <div>
                  <span>{{ t('ecommerceLoop.traffic.cart') }}</span
                  ><b>74</b>
                </div>
                <p>{{ t('ecommerceLoop.workbench.orderCartRatio') }} · 41.9%</p>
                <div>
                  <span>{{ t('ecommerceLoop.traffic.orders') }}</span
                  ><b>31</b>
                </div>
              </div>
            </section>
            <section class="ec-reading-card">
              <div class="ec-reading-heading">
                <div>
                  <h2>{{ t('ecommerceLoop.workbench.products') }}</h2>
                  <p>{{ t('ecommerceLoop.workbench.productHint') }}</p>
                </div>
              </div>
              <div class="ec-readable-table">
                <AppRegistryMarkupTable :rows="PRODUCT_METRICS" :columns="columns">
                  <template #cell-name="{ row }"
                    ><button
                      class="ec-product-select"
                      :aria-pressed="selectedProduct === row.id"
                      @click="emit('select-product', String(row.id))"
                    >
                      {{ t(`ecommerceLoop.products.${row.nameKey}`)
                      }}<q-icon v-if="row.id === 'sea-salt'" name="trending_down" /></button
                  ></template>
                  <template #cell-rate="{ row }"
                    ><span :class="{ 'ec-negative-text': row.id === 'sea-salt' }">{{
                      formatRate(Number(row.addToCart), Number(row.visitors))
                    }}</span></template
                  >
                </AppRegistryMarkupTable>
              </div>
              <div class="ec-mobile-products">
                <button
                  v-for="product in PRODUCT_METRICS"
                  :key="product.id"
                  :aria-pressed="selectedProduct === product.id"
                  @click="emit('select-product', product.id)"
                >
                  <b>{{ t(`ecommerceLoop.products.${product.nameKey}`) }}</b>
                  <span>{{ t('ecommerceLoop.traffic.visitors') }} {{ product.visitors }}</span>
                  <span :class="{ 'ec-negative-text': product.id === 'sea-salt' }"
                    >{{ t('ecommerceLoop.traffic.rate') }} {{ formatRate(product.addToCart, product.visitors) }}</span
                  >
                  <span>{{ t('ecommerceLoop.traffic.orders') }} {{ product.orders }}</span>
                </button>
              </div>
              <div class="ec-product-detail" aria-live="polite">
                <b>{{ t(`ecommerceLoop.products.${selected.nameKey}`) }}</b>
                <span
                  >{{ t('ecommerceLoop.traffic.cart') }} {{ selected.addToCart }} /
                  {{ t('ecommerceLoop.traffic.visitors') }} {{ selected.visitors }}</span
                >
                <strong v-if="selected.id === 'sea-salt'" class="ec-negative-text">{{
                  t('ecommerceLoop.workbench.drop')
                }}</strong>
                <span v-else>{{ t('ecommerceLoop.workbench.noAnomaly') }}</span>
              </div>
            </section>
            <section class="ec-reading-card ec-evidence-card">
              <div class="ec-reading-heading">
                <div>
                  <h2>{{ t('ecommerceLoop.workbench.evidence') }}</h2>
                  <p>{{ t('ecommerceLoop.workbench.evidenceSubtitle') }}</p>
                </div>
                <q-icon name="fact_check" />
              </div>
              <dl class="ec-evidence-list">
                <div>
                  <dt>{{ t('ecommerceLoop.workbench.observation') }}</dt>
                  <dd>{{ t('ecommerceLoop.workbench.observationText') }}</dd>
                </div>
                <div>
                  <dt>{{ t('ecommerceLoop.workbench.hypothesis') }}</dt>
                  <dd>{{ t('ecommerceLoop.workbench.hypothesisText') }}</dd>
                </div>
              </dl>
              <details class="ec-evidence-details">
                <summary>{{ t('ecommerceLoop.workbench.inspectEvidence') }}</summary>
                <p>{{ t('ecommerceLoop.workbench.evidenceMissing') }}</p>
                <div class="ec-image-placeholders">
                  <div>
                    <q-icon name="image" /><b>{{ t('ecommerceLoop.workbench.currentImage') }}</b
                    ><small>{{ t('ecommerceLoop.workbench.imageUnavailable') }}</small>
                  </div>
                  <div>
                    <q-icon name="local_fire_department" /><b>{{ t('ecommerceLoop.workbench.proposedImage') }}</b
                    ><small>{{ t('ecommerceLoop.workbench.imageUnavailable') }}</small>
                  </div>
                </div>
              </details>
            </section>
          </template>
          <template v-else>
            <section class="ec-reading-card">
              <div class="ec-reading-heading">
                <div>
                  <h2>{{ t('ecommerceLoop.workbench.launchScope') }}</h2>
                  <p>{{ t('ecommerceLoop.workbench.launchReason') }}</p>
                </div>
                <q-icon name="inventory_2" />
              </div>
              <div class="ec-launch-scope">
                <article v-for="product in ['grapefruit', 'lime']" :key="product">
                  <ReferenceIcon kind="candle" :tone="product === 'lime' ? 'lime' : 'purple'" />
                  <h3>{{ t(`ecommerceLoop.products.${product}`) }}</h3>
                  <p>{{ t('ecommerceLoop.workbench.sampleIllustration') }}</p>
                </article>
              </div>
              <p class="ec-workflow-notice">{{ t('ecommerceLoop.workbench.draftBoundary') }}</p>
            </section>
            <section class="ec-reading-card">
              <div class="ec-reading-heading">
                <div>
                  <h2>{{ t('ecommerceLoop.workbench.reviewChecklist') }}</h2>
                  <p>{{ t('ecommerceLoop.workbench.reviewHint') }}</p>
                </div>
                <q-icon name="checklist" />
              </div>
              <ol class="ec-review-list">
                <li v-for="key in ['image', 'copy', 'scent', 'scope']" :key="key">
                  {{ t(`ecommerceLoop.workbench.checklist.${key}`) }}
                </li>
              </ol>
              <p class="ec-workflow-notice">
                {{
                  t(
                    cases.launch.phase === 'closed'
                      ? 'ecommerceLoop.workbench.feedbackLoaded'
                      : 'ecommerceLoop.workbench.feedbackWaiting',
                  )
                }}
              </p>
            </section>
          </template>
        </div>
        <WorkflowPanel
          :key="`${selectedShop}-${activeCase}`"
          embedded
          :workflow="cases[activeCase]"
          :editing="editing"
          :shop-name="shopName"
          @action="emit('action', $event)"
          @dirty="emit('dirty', $event)"
        />
      </div>
      <p class="ec-workbench-footnote">
        <q-icon name="save" />{{ t('ecommerceLoop.local') }} · {{ t('ecommerceLoop.demoNotice') }}
      </p>
    </div>
  </section>
</template>
