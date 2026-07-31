<template>
  <q-page class="app-standard-page shop-studio-page">
    <AppPageHero
      :kicker="t('shopPage.kicker')"
      :title="t('shopPage.studioTitle')"
      :subtitle="t('shopPage.studioSubtitle')"
    >
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
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          icon="publish"
          :label="t('shopPage.publishEntry')"
          to="/shop/publish"
        />
      </template>
    </AppPageHero>

    <!-- 统计卡片 -->
    <div class="row q-col-gutter-md q-mb-md">
      <div v-for="card in statCards" :key="card.label" class="col-6 col-lg-3">
        <q-card flat class="app-glass-panel app-metrics-card q-pa-md">
          <div class="app-metrics-card__label">{{ card.label }}</div>
          <div class="app-metrics-card__value">{{ card.value }}</div>
          <trend-sparkline v-if="card.trend" :values="card.trend" class="q-mt-sm" />
          <div v-else class="app-metrics-card__hint">{{ card.hint }}</div>
        </q-card>
      </div>
    </div>

    <q-card flat class="app-glass-panel shop-studio-page__tabs">
      <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary" class="q-px-sm">
        <q-tab name="assets" :label="t('shopPage.studioTabAssets', { count: studioAssets.length })" no-caps />
        <q-tab name="inbox" no-caps>
          {{ t('shopPage.studioTabInbox') }}
          <q-badge v-if="unrepliedCount > 0" color="negative" floating>{{ unrepliedCount }}</q-badge>
        </q-tab>
      </q-tabs>
      <q-separator />

      <q-tab-panels v-model="tab" animated class="shop-studio-page__panels">
        <!-- 我的资产 -->
        <q-tab-panel name="assets" class="q-pa-none">
          <AppRegistryTable
            :shell="false"
            :rows="studioAssets"
            :columns="assetColumns"
            :loading="studioLoading"
            row-key="id"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
            column-persist-key="shop-studio-assets"
          >
            <template #body-cell-name="props">
              <q-td :props="props">
                <div class="row items-center q-gutter-sm no-wrap">
                  <asset-type-icon :type="props.row.type" size="30px" />
                  <div>
                    <div class="app-registry-cell-primary">{{ props.row.name }}</div>
                    <div class="app-registry-cell-sub">v{{ props.row.version }}</div>
                  </div>
                </div>
              </q-td>
            </template>
            <template #body-cell-reviewStatus="props">
              <q-td :props="props">
                <q-chip dense :color="reviewStatusColor(props.row.reviewStatus)" text-color="white" size="sm">
                  {{ t(`shopPage.reviewStatus.${props.row.reviewStatus}`) }}
                </q-chip>
              </q-td>
            </template>
            <template #body-cell-rating="props">
              <q-td :props="props">
                <rating-stars v-if="props.row.rating > 0" :rating="props.row.rating" show-value size="13px" />
                <span v-else class="text-grey-6">—</span>
              </q-td>
            </template>
            <template #body-cell-revenueCents="props">
              <q-td :props="props">
                {{ props.row.revenueCents > 0 ? `¥${formatCents(props.row.revenueCents)}` : '—' }}
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props" class="app-registry-cell-actions">
                <q-btn
                  flat
                  dense
                  no-caps
                  size="sm"
                  color="primary"
                  :label="t('shopPage.actionNewVersion')"
                  @click="notifyTodo"
                />
                <q-btn
                  flat
                  dense
                  no-caps
                  size="sm"
                  color="grey-7"
                  :label="t('shopPage.actionEdit')"
                  @click="notifyTodo"
                />
              </q-td>
            </template>
          </AppRegistryTable>
        </q-tab-panel>

        <!-- 评论收件箱 -->
        <q-tab-panel name="inbox" class="q-pa-md">
          <div v-for="item in studioInbox" :key="item.id" class="shop-studio-page__inbox-item">
            <div class="row items-center q-gutter-sm q-mb-xs">
              <q-avatar size="26px" color="primary" text-color="white" class="shop-studio-page__inbox-avatar">
                {{ item.author.slice(0, 1) }}
              </q-avatar>
              <span class="text-weight-medium">{{ item.author }}</span>
              <rating-stars :rating="item.rating" size="13px" />
              <q-chip dense size="sm" outline>{{ item.assetName }}</q-chip>
              <q-space />
              <q-chip v-if="item.replied" dense size="sm" color="positive" text-color="white">{{
                t('shopPage.replied')
              }}</q-chip>
              <span class="text-caption text-grey-7">{{ item.createdAt }}</span>
            </div>
            <div class="shop-studio-page__inbox-content">{{ item.content }}</div>
            <div class="row justify-end">
              <q-btn
                flat
                dense
                no-caps
                size="sm"
                color="primary"
                icon="reply"
                :label="item.replied ? t('shopPage.replyAgain') : t('shopPage.reply')"
                @click="openReply(item)"
              />
            </div>
          </div>
          <div v-if="studioInbox.length === 0" class="text-center text-grey-6 q-pa-lg">
            {{ t('shopPage.inboxEmpty') }}
          </div>
        </q-tab-panel>
      </q-tab-panels>
    </q-card>

    <reply-review-dialog v-model:open="replyOpen" :item="replyTarget" :sending="replySending" @submit="sendReply" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AssetTypeIcon from '../components/ecosystem/AssetTypeIcon.vue';
import RatingStars from '../components/ecosystem/RatingStars.vue';
import ReplyReviewDialog from '../components/ecosystem/ReplyReviewDialog.vue';
import TrendSparkline from '../components/ecosystem/TrendSparkline.vue';
import { formatCents, formatInstalls } from '../features/ecosystem/marketUi';
import { buildStudioAssetColumns } from '../features/ecosystem/marketTableUi';
import type { StudioInboxItem } from '../features/ecosystem/types';
import { useEcosystemStore } from '../stores/ecosystem';

const { t } = useI18n();
const $q = useQuasar();
const store = useEcosystemStore();
const { studioStats, studioAssets, studioInbox, studioLoading } = storeToRefs(store);
const tab = ref('assets');

const statCards = computed(() => {
  const s = studioStats.value;
  if (!s) return [];
  return [
    { label: t('shopPage.statTotalInstalls'), value: formatInstalls(s.totalInstalls), trend: s.installTrend, hint: '' },
    { label: t('shopPage.statRevenue'), value: `¥${formatCents(s.revenueCents)}`, trend: s.revenueTrend, hint: '' },
    {
      label: t('shopPage.statAvgRating'),
      value: s.avgRating.toFixed(1),
      trend: undefined,
      hint: t('shopPage.statAvgRatingHint'),
    },
    {
      label: t('shopPage.statActiveAssets'),
      value: String(s.activeAssets),
      trend: undefined,
      hint: t('shopPage.statActiveAssetsHint'),
    },
  ];
});

const unrepliedCount = computed(() => studioInbox.value.filter((i) => !i.replied).length);

const assetColumns = computed(() => buildStudioAssetColumns(t));

const REVIEW_STATUS_COLORS: Record<string, string> = {
  published: 'positive',
  scanning: 'info',
  manual: 'warning',
  needs_fix: 'negative',
  rejected: 'negative',
};

function reviewStatusColor(s: string): string {
  return REVIEW_STATUS_COLORS[s] ?? 'grey-6';
}

const replyOpen = ref(false);
const replyTarget = ref<StudioInboxItem | null>(null);
const replySending = ref(false);

function openReply(item: StudioInboxItem) {
  replyTarget.value = item;
  replyOpen.value = true;
}

async function sendReply(content: string) {
  if (!replyTarget.value) return;
  replySending.value = true;
  try {
    await store.sendReply(replyTarget.value.id, content);
    replyOpen.value = false;
    $q.notify({ type: 'positive', message: t('shopPage.replySent') });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyReplyFailed') });
  } finally {
    replySending.value = false;
  }
}

/** 骨架期动作占位 */
function notifyTodo() {
  $q.notify({ type: 'info', message: t('shopPage.skeletonActionHint') });
}

onMounted(async () => {
  try {
    await store.loadStudio();
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
  }
});
</script>

<style scoped>
.shop-studio-page__tabs {
  border-radius: 14px;
}
.shop-studio-page__panels {
  background: transparent;
}
.shop-studio-page__inbox-item {
  padding: 12px 0;
  border-bottom: 1px solid var(--glass-border);
}
.shop-studio-page__inbox-avatar {
  font-size: 11px;
  font-weight: 700;
}
.shop-studio-page__inbox-content {
  font-size: 13px;
  line-height: 1.6;
  color: var(--color-text-primary);
  padding-left: 34px;
}
</style>
