import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useEcosystemStore } from '../../stores/ecosystem';
import type { MarketAsset } from './types';

/** 浏览首页编排：分类树 + 过滤 + 网格 + 榜单 */
export function useMarketBrowsePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const router = useRouter();
  const store = useEcosystemStore();
  const { categories, assets, browseLoading } = storeToRefs(store);
  // filter 是 reactive 对象：storeToRefs 会把它包成 Ref，脚本内直接用 store.filter 保持响应性
  const filter = store.filter;
  const installingId = ref('');

  /** 默认视图（无任何过滤）时展示榜单区 */
  const isDefaultView = computed(
    () => !filter.search.trim() && !filter.type && !filter.category && !filter.priceModel && filter.sort === 'hot',
  );

  const leaderboards = computed(() => {
    if (!isDefaultView.value) return null;
    const list = assets.value;
    return {
      hot: list
        .slice()
        .sort((a, b) => b.installCount - a.installCount)
        .slice(0, 5),
      fresh: list
        .slice()
        .sort((a, b) => b.publishedAt.localeCompare(a.publishedAt))
        .slice(0, 5),
      top: list
        .slice()
        .sort((a, b) => b.rating - a.rating)
        .slice(0, 5),
    };
  });

  let searchTimer: ReturnType<typeof setTimeout> | null = null;
  function debouncedBrowse() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void store.browse(), 250);
  }

  function selectCategory(key: string) {
    filter.category = key;
    void store.browse();
  }

  function resetAll() {
    void store.resetFilter();
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

  function goDetail(asset: MarketAsset) {
    void router.push({ name: 'shop-asset', params: { slug: asset.slug } });
  }

  function goCreator(asset: MarketAsset) {
    void router.push({ name: 'shop-creator', params: { handle: asset.creator.handle } });
  }

  onMounted(async () => {
    try {
      await Promise.all([store.loadCategories(), store.browse()]);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
    }
  });

  return {
    categories,
    filter,
    assets,
    browseLoading,
    installingId,
    isDefaultView,
    leaderboards,
    debouncedBrowse,
    selectCategory,
    resetAll,
    install,
    goDetail,
    goCreator,
  };
}
