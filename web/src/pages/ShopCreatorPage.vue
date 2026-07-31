<template>
  <q-page class="app-standard-page shop-creator-page">
    <div class="row items-center q-gutter-sm q-mb-md">
      <q-btn flat round dense icon="arrow_back" @click="$router.back()" />
      <q-breadcrumbs class="text-caption">
        <q-breadcrumbs-el :label="t('shopPage.title')" to="/shop" />
        <q-breadcrumbs-el v-if="creator" :label="creator.name" />
      </q-breadcrumbs>
    </div>

    <q-card v-if="creatorLoading" flat class="app-glass-panel q-pa-lg">
      <q-skeleton type="circle" size="64px" class="q-mb-md" />
      <q-skeleton type="rect" height="24px" width="30%" class="q-mb-sm" />
      <q-skeleton type="text" width="60%" />
    </q-card>

    <template v-else-if="creator">
      <!-- 创作者头部 -->
      <q-card flat class="app-glass-panel q-pa-lg q-mb-md">
        <div class="row items-center q-gutter-lg">
          <q-avatar size="72px" :style="{ background: creator.avatarColor }" class="shop-creator-page__avatar">
            <span class="shop-creator-page__initial">{{ creator.name.slice(0, 1) }}</span>
          </q-avatar>
          <div class="col">
            <div class="row items-center q-gutter-xs">
              <span class="text-h5 text-weight-bold">{{ creator.name }}</span>
              <q-chip v-if="creator.verified" dense size="sm" color="primary" text-color="white" icon="verified">
                {{ t('shopPage.verifiedCreator') }}
              </q-chip>
            </div>
            <div class="text-body2 text-grey-7 q-mt-xs">@{{ creator.handle }}</div>
            <div class="text-body2 q-mt-sm">{{ creator.bio }}</div>
          </div>
          <div class="col-12 col-md-auto">
            <q-btn color="primary" unelevated rounded no-caps icon="person_add" :label="t('shopPage.follow')" />
          </div>
        </div>

        <div class="row q-col-gutter-md q-mt-md">
          <div v-for="stat in stats" :key="stat.label" class="col-6 col-sm-3">
            <div class="shop-creator-page__stat">
              <div class="shop-creator-page__stat-value">{{ stat.value }}</div>
              <div class="shop-creator-page__stat-label text-caption">{{ stat.label }}</div>
            </div>
          </div>
        </div>
      </q-card>

      <!-- 作品网格 -->
      <div class="text-h6 text-weight-bold q-mb-md">{{ t('shopPage.creatorWorks', { count: assets.length }) }}</div>
      <div class="row q-col-gutter-md">
        <div v-for="asset in assets" :key="asset.id" class="col-12 col-sm-6 col-xl-4">
          <asset-card
            :asset="asset"
            :installing="installingId === asset.id"
            @open="goDetail(asset)"
            @open-creator="() => {}"
            @install="install(asset)"
          />
        </div>
      </div>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import AssetCard from '../components/ecosystem/AssetCard.vue';
import { formatInstalls } from '../features/ecosystem/marketUi';
import type { MarketAsset } from '../features/ecosystem/types';
import { useEcosystemStore } from '../stores/ecosystem';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const $q = useQuasar();
const store = useEcosystemStore();
const { creatorDetail, creatorLoading } = storeToRefs(store);
const installingId = ref('');

const creator = computed(() => creatorDetail.value.creator);
const assets = computed(() => creatorDetail.value.assets);

const stats = computed(() => {
  const c = creator.value;
  if (!c) return [];
  return [
    { label: t('shopPage.statAssets'), value: String(c.assetCount) },
    { label: t('shopPage.statInstalls'), value: formatInstalls(c.totalInstalls) },
    { label: t('shopPage.statRating'), value: c.avgRating.toFixed(1) },
    { label: t('shopPage.statFollowers'), value: formatInstalls(c.followers) },
  ];
});

function goDetail(asset: MarketAsset) {
  void router.push({ name: 'shop-asset', params: { slug: asset.slug } });
}

async function install(asset: MarketAsset) {
  installingId.value = asset.id;
  try {
    await store.installAssetById(asset.id);
    $q.notify({ type: 'positive', message: t('shopPage.notifyInstallSuccess', { name: asset.name }) });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyInstallFailed') });
  } finally {
    installingId.value = '';
  }
}

onMounted(async () => {
  try {
    await store.loadCreator(String(route.params.handle ?? ''));
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
  }
});
</script>

<style scoped>
.shop-creator-page__avatar {
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.15);
}
.shop-creator-page__initial {
  color: var(--color-on-accent);
  font-size: 30px;
  font-weight: 800;
}
.shop-creator-page__stat {
  text-align: center;
  padding: 12px;
  border-radius: 12px;
  background: var(--interaction-surface-hover);
}
body.body--dark .shop-creator-page__stat {
  background: rgba(255, 255, 255, 0.05);
}
.shop-creator-page__stat-value {
  font-size: 22px;
  font-weight: 800;
  color: var(--color-text-primary);
}
.shop-creator-page__stat-label {
  color: var(--color-text-secondary);
  margin-top: 2px;
}
</style>
