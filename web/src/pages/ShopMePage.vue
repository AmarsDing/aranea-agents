<template>
  <q-page class="app-standard-page shop-me-page">
    <AppPageHero :kicker="t('shopPage.kicker')" :title="t('shopPage.meTitle')" :subtitle="t('shopPage.meSubtitle')">
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="storefront"
          :label="t('shopPage.backToMarket')"
          to="/shop"
        />
      </template>
    </AppPageHero>

    <q-card flat class="app-glass-panel shop-me-page__tabs">
      <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary" class="q-px-sm">
        <q-tab name="installs" :label="t('shopPage.meTabInstalls', { count: myInstalls.length })" no-caps />
        <q-tab name="orders" :label="t('shopPage.meTabOrders', { count: myOrders.length })" no-caps />
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="shop-me-page__panels">
        <!-- 已安装 -->
        <q-tab-panel name="installs" class="q-pa-none">
          <q-table
            flat
            :rows="myInstalls"
            :columns="installColumns"
            :loading="meLoading"
            row-key="assetId"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
            class="app-registry-table"
          >
            <template #body-cell-name="props">
              <q-td :props="props">
                <div class="row items-center q-gutter-sm no-wrap">
                  <asset-type-icon :type="props.row.type" size="30px" />
                  <span class="app-registry-cell-primary">{{ props.row.name }}</span>
                </div>
              </q-td>
            </template>
            <template #body-cell-health7d="props">
              <q-td :props="props">
                <div class="row items-center q-gutter-xs no-wrap">
                  <q-circular-progress
                    :value="props.row.health7d"
                    size="26px"
                    :thickness="0.25"
                    :color="healthColor(props.row.health7d)"
                    track-color="grey-3"
                  />
                  <span>{{ props.row.health7d.toFixed(1) }}%</span>
                </div>
              </q-td>
            </template>
            <template #body-cell-status="props">
              <q-td :props="props">
                <q-chip dense :color="statusColor(props.row.status)" text-color="white" size="sm">
                  {{ t(`shopPage.installStatus.${props.row.status}`) }}
                </q-chip>
                <q-tooltip v-if="props.row.status !== 'healthy'">{{ t('shopPage.installStatusHint') }}</q-tooltip>
              </q-td>
            </template>
            <template #body-cell-version="props">
              <q-td :props="props">
                <span>v{{ props.row.version }}</span>
                <q-chip
                  v-if="props.row.updateAvailable"
                  dense
                  size="sm"
                  color="warning"
                  text-color="white"
                  class="q-ml-xs"
                >
                  → v{{ props.row.updateAvailable }}
                </q-chip>
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props" class="app-registry-cell-actions">
                <q-btn
                  v-if="props.row.updateAvailable"
                  flat
                  dense
                  no-caps
                  size="sm"
                  color="primary"
                  :label="t('shopPage.actionUpdate')"
                  @click="notifyTodo"
                />
                <q-btn
                  flat
                  dense
                  no-caps
                  size="sm"
                  color="grey-7"
                  :label="t('shopPage.uninstall')"
                  @click="notifyTodo"
                />
              </q-td>
            </template>
          </q-table>
        </q-tab-panel>

        <!-- 订单 -->
        <q-tab-panel name="orders" class="q-pa-none">
          <q-table
            flat
            :rows="myOrders"
            :columns="orderColumns"
            :loading="meLoading"
            row-key="id"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
            class="app-registry-table"
          >
            <template #body-cell-name="props">
              <q-td :props="props">
                <div class="row items-center q-gutter-sm no-wrap">
                  <asset-type-icon :type="props.row.type" size="30px" />
                  <span class="app-registry-cell-primary">{{ props.row.name }}</span>
                </div>
              </q-td>
            </template>
            <template #body-cell-amountCents="props">
              <q-td :props="props">
                <price-tag :price-model="props.row.priceModel" :price-cents="props.row.amountCents" />
              </q-td>
            </template>
            <template #body-cell-status="props">
              <q-td :props="props">
                <q-chip dense :color="orderStatusColor(props.row.status)" text-color="white" size="sm">
                  {{ t(`shopPage.orderStatus.${props.row.status}`) }}
                </q-chip>
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props" class="app-registry-cell-actions">
                <q-btn
                  v-if="props.row.status === 'paid'"
                  flat
                  dense
                  no-caps
                  size="sm"
                  color="grey-7"
                  :label="t('shopPage.actionRefund')"
                  @click="notifyTodo"
                />
              </q-td>
            </template>
          </q-table>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import type { QTableColumn } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AssetTypeIcon from '../components/ecosystem/AssetTypeIcon.vue';
import PriceTag from '../components/ecosystem/PriceTag.vue';
import { useEcosystemStore } from '../stores/ecosystem';

const { t } = useI18n();
const $q = useQuasar();
const store = useEcosystemStore();
const { myInstalls, myOrders, meLoading } = storeToRefs(store);
const tab = ref('installs');

const installColumns = computed<QTableColumn[]>(() => [
  { name: 'name', label: t('shopPage.colAsset'), field: 'name', align: 'left' },
  { name: 'version', label: t('shopPage.colVersion'), field: 'version', align: 'left' },
  { name: 'installedAt', label: t('shopPage.colInstalledAt'), field: 'installedAt', align: 'left' },
  { name: 'health7d', label: t('shopPage.colHealth'), field: 'health7d', align: 'left' },
  { name: 'status', label: t('shopPage.colStatus'), field: 'status', align: 'left' },
  { name: 'actions', label: '', field: 'assetId', align: 'right' },
]);

const orderColumns = computed<QTableColumn[]>(() => [
  { name: 'id', label: t('shopPage.colOrderId'), field: 'id', align: 'left' },
  { name: 'name', label: t('shopPage.colAsset'), field: 'name', align: 'left' },
  { name: 'amountCents', label: t('shopPage.colAmount'), field: 'amountCents', align: 'left' },
  { name: 'status', label: t('shopPage.colStatus'), field: 'status', align: 'left' },
  { name: 'createdAt', label: t('shopPage.colOrderDate'), field: 'createdAt', align: 'left' },
  { name: 'actions', label: '', field: 'id', align: 'right' },
]);

function healthColor(v: number): string {
  if (v >= 95) return 'positive';
  if (v >= 80) return 'warning';
  return 'negative';
}

function statusColor(s: string): string {
  return s === 'healthy' ? 'positive' : s === 'degraded' ? 'warning' : 'negative';
}

function orderStatusColor(s: string): string {
  return s === 'paid' ? 'positive' : s === 'refunding' ? 'warning' : 'grey-6';
}

/** 骨架期动作占位：更新/卸载/退款的后端流程未接入 */
function notifyTodo() {
  $q.notify({ type: 'info', message: t('shopPage.skeletonActionHint') });
}

onMounted(async () => {
  try {
    await Promise.all([store.loadMyInstalls(), store.loadMyOrders()]);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
  }
});
</script>

<style scoped>
.shop-me-page__tabs {
  border-radius: 14px;
}
.shop-me-page__panels {
  background: transparent;
}
</style>
