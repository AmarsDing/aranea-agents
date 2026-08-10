<template>
  <q-page class="app-standard-page shop-asset-page">
    <div class="row items-center q-gutter-sm q-mb-md">
      <q-btn flat round dense icon="arrow_back" @click="$router.back()" />
      <q-breadcrumbs class="text-caption">
        <q-breadcrumbs-el :label="t('shopPage.title')" to="/shop" />
        <q-breadcrumbs-el v-if="asset" :label="asset.name" />
      </q-breadcrumbs>
    </div>

    <!-- 加载骨架 -->
    <template v-if="detailLoading">
      <q-card flat class="app-glass-panel q-pa-lg q-mb-md">
        <q-skeleton type="rect" height="28px" width="40%" class="q-mb-sm" />
        <q-skeleton type="text" width="70%" />
        <q-skeleton type="text" width="50%" />
      </q-card>
    </template>

    <!-- 未找到 -->
    <q-card v-else-if="notFound" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="inventory_2" />
        <div class="text-h6 q-mt-md">{{ t('shopPage.assetNotFound') }}</div>
        <q-btn outline rounded no-caps color="primary" class="q-mt-md" :label="t('shopPage.backToMarket')" to="/shop" />
      </q-card-section>
    </q-card>

    <template v-else-if="asset">
      <!-- 头部信息卡 -->
      <q-card flat class="app-glass-panel q-pa-lg q-mb-md">
        <div class="row q-col-gutter-md items-start">
          <div class="col-12 col-md">
            <div class="row items-start no-wrap q-gutter-md">
              <asset-type-icon :type="asset.type" size="56px" />
              <div class="col">
                <div class="row items-center q-gutter-xs">
                  <span class="text-h5 text-weight-bold">{{ asset.name }}</span>
                  <q-chip dense size="sm" color="primary" text-color="white">{{
                    t(`shopPage.type.${typeMeta.labelKey}`)
                  }}</q-chip>
                </div>
                <div class="text-body2 text-grey-7 q-mt-xs">{{ asset.tagline }}</div>
                <div class="row items-center q-gutter-md q-mt-sm">
                  <span class="shop-asset-page__creator row items-center q-gutter-xs" @click="goCreator">
                    <q-avatar size="20px" :style="{ background: asset.creator.avatarColor }">
                      <span class="shop-asset-page__creator-initial">{{ asset.creator.name.slice(0, 1) }}</span>
                    </q-avatar>
                    <span class="text-weight-medium">{{ asset.creator.name }}</span>
                    <q-icon v-if="asset.creator.verified" name="verified" size="14px" color="primary" />
                  </span>
                  <rating-stars :rating="asset.rating" :count="asset.ratingCount" show-value />
                  <span class="text-caption text-grey-7">
                    <q-icon name="download" size="13px" /> {{ formatInstalls(asset.installCount) }}
                  </span>
                  <span class="text-caption text-grey-7"
                    >v{{ asset.version }} · {{ t('shopPage.updatedAt') }} {{ asset.updatedAt }}</span
                  >
                </div>
              </div>
            </div>
          </div>
          <div class="col-12 col-md-auto shop-asset-page__cta">
            <price-tag :price-model="asset.priceModel" :price-cents="asset.priceCents" class="shop-asset-page__price" />
            <q-btn
              v-if="!asset.installed"
              color="primary"
              unelevated
              rounded
              no-caps
              icon="download"
              :label="asset.priceModel === 'free' ? t('shopPage.install') : t('shopPage.buyAndInstall')"
              :loading="installing"
              @click="requestInstall"
            />
            <q-btn
              v-else
              outline
              rounded
              no-caps
              icon="delete_outline"
              :label="t('shopPage.uninstall')"
              :loading="installing"
              @click="uninstall"
            />
          </div>
        </div>
      </q-card>

      <div class="row q-col-gutter-md">
        <!-- 主内容区 -->
        <div class="col-12 col-lg-8">
          <q-card flat class="app-glass-panel q-pa-md q-mb-md">
            <div class="text-weight-bold q-mb-sm">{{ t('shopPage.screenshots') }}</div>
            <asset-screenshots :asset="asset" :count="asset.screenshotCount" />
          </q-card>

          <q-card flat class="app-glass-panel shop-asset-page__tabs">
            <q-tabs v-model="tab" dense align="left" active-color="primary" indicator-color="primary" class="q-px-sm">
              <q-tab name="readme" :label="t('shopPage.tabReadme')" no-caps />
              <q-tab v-if="asset.orgBundle" name="org" :label="t('shopPage.tabOrgPreview')" no-caps />
              <q-tab name="versions" :label="t('shopPage.tabVersions')" no-caps />
              <q-tab name="reviews" :label="t('shopPage.tabReviews', { count: asset.ratingCount })" no-caps />
            </q-tabs>
            <q-separator />
            <q-tab-panels v-model="tab" animated class="shop-asset-page__panels">
              <q-tab-panel name="readme">
                <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
                <div class="chat-message-prose shop-asset-page__md" v-html="readmeHtml"></div>
              </q-tab-panel>
              <q-tab-panel v-if="asset.orgBundle" name="org">
                <org-bundle-tree :bundle="asset.orgBundle" />
              </q-tab-panel>
              <q-tab-panel name="versions">
                <q-timeline color="primary" layout="dense">
                  <q-timeline-entry
                    v-for="(v, i) in asset.versions"
                    :key="v.version"
                    :title="`v${v.version}`"
                    :subtitle="v.date"
                    :icon="i === 0 ? 'new_releases' : undefined"
                  >
                    <div class="text-body2">{{ v.note }}</div>
                  </q-timeline-entry>
                </q-timeline>
              </q-tab-panel>
              <q-tab-panel name="reviews">
                <review-section :asset="asset" :submitting="submittingReview" @submit="submitReview" />
              </q-tab-panel>
            </q-tab-panels>
          </q-card>
        </div>

        <!-- 右侧栏 -->
        <div class="col-12 col-lg-4">
          <q-card flat class="app-glass-panel q-pa-md q-mb-md">
            <div class="row items-center q-gutter-xs q-mb-sm">
              <span class="text-weight-bold">{{ t('shopPage.permissions') }}</span>
              <q-badge v-if="asset.permissions.length" color="grey-6" :label="asset.permissions.length" />
            </div>
            <permission-list :permissions="asset.permissions" />
          </q-card>

          <q-card v-if="asset.deps.length" flat class="app-glass-panel q-pa-md q-mb-md">
            <div class="text-weight-bold q-mb-sm">{{ t('shopPage.dependencies') }}</div>
            <div class="column q-gutter-xs">
              <div v-for="dep in asset.deps" :key="dep.id" class="row items-center q-gutter-sm">
                <asset-type-icon :type="dep.kind" size="26px" />
                <div class="col ellipsis">
                  <div class="text-body2 ellipsis">{{ dep.name }}</div>
                  <div class="text-caption text-grey-7">{{ dep.range }}</div>
                </div>
              </div>
            </div>
          </q-card>

          <q-card flat class="app-glass-panel q-pa-md">
            <div class="text-weight-bold q-mb-sm">{{ t('shopPage.metaInfo') }}</div>
            <q-list dense class="shop-asset-page__meta">
              <q-item
                ><q-item-section side>{{ t('shopPage.metaVersion') }}</q-item-section
                ><q-item-section class="text-right">v{{ asset.version }}</q-item-section></q-item
              >
              <q-item
                ><q-item-section side>{{ t('shopPage.metaCompat') }}</q-item-section
                ><q-item-section class="text-right">{{ asset.compatibility }}</q-item-section></q-item
              >
              <q-item
                ><q-item-section side>{{ t('shopPage.metaPublished') }}</q-item-section
                ><q-item-section class="text-right">{{ asset.publishedAt }}</q-item-section></q-item
              >
              <q-item
                ><q-item-section side>{{ t('shopPage.metaHealth') }}</q-item-section
                ><q-item-section class="text-right">{{ asset.health7d.toFixed(1) }}%</q-item-section></q-item
              >
            </q-list>
            <div class="q-mt-sm">
              <div class="text-caption text-grey-7 q-mb-xs">{{ t('shopPage.metaCategories') }}</div>
              <q-chip v-for="c in asset.categories" :key="c" dense size="sm" class="q-mr-xs">{{ c }}</q-chip>
            </div>
            <div class="q-mt-sm">
              <div class="text-caption text-grey-7 q-mb-xs">{{ t('shopPage.metaTags') }}</div>
              <q-chip v-for="tag in asset.tags" :key="tag" dense size="sm" outline class="q-mr-xs">{{ tag }}</q-chip>
            </div>
          </q-card>
        </div>
      </div>

      <install-confirm-dialog v-model:open="confirmOpen" :asset="asset" :installing="installing" @confirm="doInstall" />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import MarkdownIt from 'markdown-it';
import AssetScreenshots from '../components/ecosystem/AssetScreenshots.vue';
import AssetTypeIcon from '../components/ecosystem/AssetTypeIcon.vue';
import InstallConfirmDialog from '../components/ecosystem/InstallConfirmDialog.vue';
import OrgBundleTree from '../components/ecosystem/OrgBundleTree.vue';
import PermissionList from '../components/ecosystem/PermissionList.vue';
import PriceTag from '../components/ecosystem/PriceTag.vue';
import RatingStars from '../components/ecosystem/RatingStars.vue';
import ReviewSection from '../components/ecosystem/ReviewSection.vue';
import { ASSET_TYPE_META, formatInstalls } from '../features/ecosystem/marketUi';
import { useMarketAssetDetail } from '../features/ecosystem/useMarketAssetDetail';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const {
  assetDetail: asset,
  detailLoading,
  notFound,
  installing,
  confirmOpen,
  submittingReview,
  requestInstall,
  doInstall,
  uninstall,
  submitReview,
} = useMarketAssetDetail(() => String(route.params.slug ?? ''));

const tab = ref('readme');
const md = new MarkdownIt({ html: false, linkify: true, breaks: false });
const readmeHtml = computed(() => (asset.value ? md.render(asset.value.readme) : ''));
const typeMeta = computed(() => (asset.value ? ASSET_TYPE_META[asset.value.type] : ASSET_TYPE_META.skill));

function goCreator() {
  if (asset.value) void router.push({ name: 'shop-creator', params: { handle: asset.value.creator.handle } });
}
</script>

<style scoped>
.shop-asset-page__creator {
  cursor: pointer;
  color: var(--color-text-secondary);
}

.shop-asset-page__creator:hover {
  color: var(--color-accent);
}

.shop-asset-page__creator-initial {
  color: var(--color-on-accent);
  font-size: 11px;
  font-weight: 700;
}

.shop-asset-page__cta {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 10px;
}

.shop-asset-page__price {
  font-size: 20px;
}

.shop-asset-page__tabs {
  border-radius: 14px;
}

.shop-asset-page__panels {
  background: transparent;
}

.shop-asset-page__md {
  font-size: 14px;
  line-height: 1.75;
}

.shop-asset-page__meta :deep(.q-item) {
  min-height: 32px;
  padding: 0;
}

.shop-asset-page__meta :deep(.q-item__section--side) {
  color: var(--color-text-secondary);
  font-size: 12px;
}
</style>
