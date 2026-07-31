import { defineStore } from 'pinia';
import { reactive, ref } from 'vue';
import {
  getAsset,
  getCreator,
  getOrgPickTree,
  getStudioStats,
  installAsset,
  installEcosystemProduct,
  listCategories,
  listEcosystemProducts,
  listMyInstalls,
  listMyOrders,
  listStudioAssets,
  listStudioInbox,
  publishEcosystemProduct,
  rateAsset,
  replyReview,
  searchAssets,
  uninstallAsset,
} from '../../features/ecosystem/api';
import type {
  BrowseFilter,
  CategoryNode,
  EcosystemProduct,
  MarketAsset,
  MarketCreator,
  MyInstall,
  MyOrder,
  OrgPickNode,
  StudioAsset,
  StudioInboxItem,
  StudioStats,
} from '../../features/ecosystem/types';

export const useEcosystemStore = defineStore('ecosystem', () => {
  // ── M30 既有状态（保留兼容） ─────────────────────────────────
  const products = ref<EcosystemProduct[]>([]);
  const loading = ref(false);

  async function load(search = '') {
    loading.value = true;
    try {
      products.value = await listEcosystemProducts(search.trim());
      return products.value;
    } finally {
      loading.value = false;
    }
  }

  async function install(productId: string) {
    await installEcosystemProduct(productId);
    return load();
  }

  async function publish(input: Parameters<typeof publishEcosystemProduct>[0]) {
    await publishEcosystemProduct(input);
    return load();
  }

  // ── M57 商城：浏览页 ─────────────────────────────────────────
  const categories = ref<CategoryNode[]>([]);
  const filter = reactive<BrowseFilter>({ search: '', type: '', category: '', priceModel: '', sort: 'hot' });
  const assets = ref<MarketAsset[]>([]);
  const browseLoading = ref(false);

  async function loadCategories() {
    if (categories.value.length > 0) return categories.value;
    categories.value = await listCategories();
    return categories.value;
  }

  async function browse() {
    browseLoading.value = true;
    try {
      assets.value = await searchAssets({ ...filter });
      return assets.value;
    } finally {
      browseLoading.value = false;
    }
  }

  function resetFilter() {
    filter.search = '';
    filter.type = '';
    filter.category = '';
    filter.priceModel = '';
    filter.sort = 'hot';
    return browse();
  }

  // ── M57 商城：详情页 / 创作者主页 ────────────────────────────
  const assetDetail = ref<MarketAsset | null>(null);
  const detailLoading = ref(false);
  const creatorDetail = ref<{ creator: MarketCreator | null; assets: MarketAsset[] }>({ creator: null, assets: [] });
  const creatorLoading = ref(false);

  async function loadAsset(slug: string) {
    detailLoading.value = true;
    try {
      assetDetail.value = await getAsset(slug);
      return assetDetail.value;
    } finally {
      detailLoading.value = false;
    }
  }

  async function loadCreator(handle: string) {
    creatorLoading.value = true;
    try {
      creatorDetail.value = await getCreator(handle);
      return creatorDetail.value;
    } finally {
      creatorLoading.value = false;
    }
  }

  /** 安装/卸载：骨架期仅翻转本地状态 */
  async function installAssetById(assetId: string) {
    await installAsset(assetId);
    if (assetDetail.value?.id === assetId) assetDetail.value.installed = true;
    const card = assets.value.find((a) => a.id === assetId);
    if (card) card.installed = true;
  }

  async function uninstallAssetById(assetId: string) {
    await uninstallAsset(assetId);
    if (assetDetail.value?.id === assetId) assetDetail.value.installed = false;
    const card = assets.value.find((a) => a.id === assetId);
    if (card) card.installed = false;
  }

  async function submitReview(assetId: string, rating: number, content: string) {
    await rateAsset(assetId, rating, content);
  }

  // ── M57 商城：买家工作台 ─────────────────────────────────────
  const myInstalls = ref<MyInstall[]>([]);
  const myOrders = ref<MyOrder[]>([]);
  const meLoading = ref(false);

  async function loadMyInstalls() {
    meLoading.value = true;
    try {
      myInstalls.value = await listMyInstalls();
      return myInstalls.value;
    } finally {
      meLoading.value = false;
    }
  }

  async function loadMyOrders() {
    meLoading.value = true;
    try {
      myOrders.value = await listMyOrders();
      return myOrders.value;
    } finally {
      meLoading.value = false;
    }
  }

  // ── M57 商城：创作者中心 ─────────────────────────────────────
  const studioStats = ref<StudioStats | null>(null);
  const studioAssets = ref<StudioAsset[]>([]);
  const studioInbox = ref<StudioInboxItem[]>([]);
  const studioLoading = ref(false);

  async function loadStudio() {
    studioLoading.value = true;
    try {
      const [stats, assetList, inbox] = await Promise.all([getStudioStats(), listStudioAssets(), listStudioInbox()]);
      studioStats.value = stats;
      studioAssets.value = assetList;
      studioInbox.value = inbox;
    } finally {
      studioLoading.value = false;
    }
  }

  async function sendReply(reviewId: string, content: string) {
    await replyReview(reviewId, content);
    const item = studioInbox.value.find((i) => i.id === reviewId);
    if (item) item.replied = true;
  }

  // ── M57 商城：发布向导（组织整包节点树） ─────────────────────
  const orgPickTree = ref<OrgPickNode[]>([]);

  async function loadOrgPickTree() {
    orgPickTree.value = await getOrgPickTree();
    return orgPickTree.value;
  }

  return {
    // M30
    products,
    loading,
    load,
    install,
    publish,
    // 浏览
    categories,
    filter,
    assets,
    browseLoading,
    loadCategories,
    browse,
    resetFilter,
    // 详情 / 创作者
    assetDetail,
    detailLoading,
    creatorDetail,
    creatorLoading,
    loadAsset,
    loadCreator,
    installAssetById,
    uninstallAssetById,
    submitReview,
    // 买家
    myInstalls,
    myOrders,
    meLoading,
    loadMyInstalls,
    loadMyOrders,
    // 创作者
    studioStats,
    studioAssets,
    studioInbox,
    studioLoading,
    loadStudio,
    sendReply,
    // 发布向导
    orgPickTree,
    loadOrgPickTree,
  };
});
