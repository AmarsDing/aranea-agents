<template>
  <q-page class="app-standard-page shop-browse-page">
    <AppPageHero :kicker="t('shopPage.kicker')" :title="t('shopPage.title')" :subtitle="t('shopPage.subtitle')">
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="inventory_2"
          :label="t('shopPage.myWorkbench')"
          to="/shop/me"
        />
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="store"
          :label="t('shopPage.studioEntry')"
          to="/shop/studio"
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
      <div class="shop-browse-page__hero-search row justify-center">
        <q-input
          v-model="filter.search"
          standout
          rounded
          dense
          clearable
          class="shop-browse-page__search"
          :placeholder="t('shopPage.searchPlaceholder')"
          @update:model-value="debouncedBrowse"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </div>
    </AppPageHero>

    <div class="row q-col-gutter-md">
      <!-- 左侧：分类树 -->
      <div class="col-12 col-md-3 col-lg-2">
        <q-card flat class="app-glass-panel shop-browse-page__side q-pa-sm">
          <category-tree :nodes="categories" :selected="filter.category" @select="selectCategory" />
        </q-card>
      </div>

      <!-- 右侧：过滤栏 + 榜单 + 网格 -->
      <div class="col-12 col-md-9 col-lg-10">
        <q-card flat class="app-glass-panel q-pa-md q-mb-md">
          <market-filter-bar
            v-model:type="filter.type"
            v-model:price-model="filter.priceModel"
            v-model:sort="filter.sort"
            :total="assets.length"
            @update:type="store.browse"
            @update:price-model="store.browse"
            @update:sort="store.browse"
            @reset="resetAll"
          />
        </q-card>

        <!-- 榜单区（默认视图） -->
        <div v-if="leaderboards" class="row q-col-gutter-md q-mb-md">
          <div class="col-12 col-md-4">
            <market-leaderboard
              :title="t('shopPage.boardHot')"
              icon="local_fire_department"
              :items="leaderboards.hot"
              :metric="(a) => formatInstalls(a.installCount)"
              @open="goDetail"
            />
          </div>
          <div class="col-12 col-md-4">
            <market-leaderboard
              :title="t('shopPage.boardNew')"
              icon="new_releases"
              :items="leaderboards.fresh"
              :metric="(a) => a.publishedAt.slice(5)"
              @open="goDetail"
            />
          </div>
          <div class="col-12 col-md-4">
            <market-leaderboard
              :title="t('shopPage.boardTop')"
              icon="emoji_events"
              :items="leaderboards.top"
              :metric="(a) => a.rating.toFixed(1)"
              @open="goDetail"
            />
          </div>
        </div>

        <!-- 加载骨架 -->
        <div v-if="browseLoading" class="row q-col-gutter-md">
          <div v-for="i in 6" :key="i" class="col-12 col-sm-6 col-xl-4">
            <q-card flat class="app-glass-panel q-pa-md">
              <q-skeleton type="rect" height="20px" width="60%" class="q-mb-sm" />
              <q-skeleton type="text" />
              <q-skeleton type="text" width="80%" />
              <q-skeleton type="QBtn" class="q-mt-md full-width" />
            </q-card>
          </div>
        </div>

        <!-- 空态 -->
        <q-card v-else-if="assets.length === 0" flat class="app-registry-empty app-empty-state-center">
          <q-card-section class="column items-center text-center q-pa-xl">
            <q-avatar size="72px" color="primary" text-color="white" icon="search_off" />
            <div class="text-h6 q-mt-md">{{ t('shopPage.emptyTitle') }}</div>
            <div class="text-body2 text-grey-7 q-mt-sm">{{ t('shopPage.emptyHint') }}</div>
            <q-btn
              outline
              rounded
              no-caps
              color="primary"
              class="q-mt-md"
              :label="t('shopPage.filterReset')"
              @click="resetAll"
            />
          </q-card-section>
        </q-card>

        <!-- 资产网格 -->
        <div v-else class="row q-col-gutter-md">
          <div v-for="asset in assets" :key="asset.id" class="col-12 col-sm-6 col-xl-4">
            <asset-card
              :asset="asset"
              :installing="installingId === asset.id"
              @open="goDetail(asset)"
              @open-creator="goCreator(asset)"
              @install="install(asset)"
            />
          </div>
        </div>
      </div>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AssetCard from '../components/ecosystem/AssetCard.vue';
import CategoryTree from '../components/ecosystem/CategoryTree.vue';
import MarketFilterBar from '../components/ecosystem/MarketFilterBar.vue';
import MarketLeaderboard from '../components/ecosystem/MarketLeaderboard.vue';
import { formatInstalls } from '../features/ecosystem/marketUi';
import { useMarketBrowsePage } from '../features/ecosystem/useMarketBrowsePage';
import { useEcosystemStore } from '../stores/ecosystem';

const { t } = useI18n();
const store = useEcosystemStore();
const { filter } = storeToRefs(store);

const {
  categories,
  assets,
  browseLoading,
  installingId,
  leaderboards,
  debouncedBrowse,
  selectCategory,
  resetAll,
  install,
  goDetail,
  goCreator,
} = useMarketBrowsePage();
</script>

<style scoped>
.shop-browse-page__hero-search {
  margin-top: 18px;
}

.shop-browse-page__search {
  width: min(560px, 100%);
}

.shop-browse-page__side {
  position: sticky;
  top: 76px;
  max-height: calc(100vh - 100px);
  overflow-y: auto;
  border-radius: 14px;
}

@media (width <= 1023px) {
  .shop-browse-page__side {
    position: static;
    max-height: none;
  }
}
</style>
