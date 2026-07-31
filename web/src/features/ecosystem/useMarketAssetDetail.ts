import { computed, onMounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useEcosystemStore } from '../../stores/ecosystem';

/** 商品详情页编排：加载、安装确认弹窗、评分提交 */
export function useMarketAssetDetail(slug: () => string) {
  const $q = useQuasar();
  const { t } = useI18n();
  const store = useEcosystemStore();
  const { assetDetail, detailLoading } = storeToRefs(store);

  const installing = ref(false);
  const confirmOpen = ref(false);

  const notFound = computed(() => !detailLoading.value && assetDetail.value === null);

  async function load() {
    try {
      await store.loadAsset(slug());
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
    }
  }

  function requestInstall() {
    const asset = assetDetail.value;
    if (!asset) return;
    if (asset.permissions.length > 0) {
      confirmOpen.value = true;
    } else {
      void doInstall();
    }
  }

  async function doInstall() {
    const asset = assetDetail.value;
    if (!asset) return;
    installing.value = true;
    try {
      await store.installAssetById(asset.id);
      confirmOpen.value = false;
      $q.notify({ type: 'positive', message: t('shopPage.notifyInstallSuccess', { name: asset.name }) });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyInstallFailed') });
    } finally {
      installing.value = false;
    }
  }

  async function uninstall() {
    const asset = assetDetail.value;
    if (!asset) return;
    installing.value = true;
    try {
      await store.uninstallAssetById(asset.id);
      $q.notify({ type: 'info', message: t('shopPage.notifyUninstallSuccess', { name: asset.name }) });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyUninstallFailed') });
    } finally {
      installing.value = false;
    }
  }

  const submittingReview = ref(false);
  async function submitReview(rating: number, content: string) {
    const asset = assetDetail.value;
    if (!asset) return;
    submittingReview.value = true;
    try {
      await store.submitReview(asset.id, rating, content);
      asset.reviews.unshift({
        id: `local-${Date.now()}`,
        author: t('shopPage.reviewAuthorMe'),
        rating,
        content,
        createdAt: new Date().toISOString().slice(0, 10),
        likes: 0,
      });
      $q.notify({ type: 'positive', message: t('shopPage.notifyReviewSubmitted') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyReviewFailed') });
    } finally {
      submittingReview.value = false;
    }
  }

  onMounted(() => void load());

  return {
    assetDetail,
    detailLoading,
    notFound,
    installing,
    confirmOpen,
    submittingReview,
    requestInstall,
    doInstall,
    uninstall,
    submitReview,
  };
}
