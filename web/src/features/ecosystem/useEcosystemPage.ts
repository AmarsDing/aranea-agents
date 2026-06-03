import { onMounted, reactive, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useEcosystemStore } from '../../stores/ecosystem';
import type { EcosystemProduct } from './api';

export function useEcosystemPage() {
  const $q = useQuasar();
  const store = useEcosystemStore();
  const { products, loading } = storeToRefs(store);
  const search = ref('');
  const publishing = ref(false);
  const publishOpen = ref(false);
  const installingId = ref('');
  const draft = reactive({ name: '', display_name: '', description: '', type: 'skill_pack' });

  let debounceTimer: ReturnType<typeof setTimeout> | null = null;

  function debouncedLoad() {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => void load(), 300);
  }

  async function load() {
    try {
      await store.load(search.value);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '加载失败' });
    }
  }

  async function install(p: EcosystemProduct) {
    installingId.value = p.id;
    try {
      await store.install(p.id);
      $q.notify({ type: 'positive', message: '安装成功' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '安装失败' });
    } finally {
      installingId.value = '';
    }
  }

  async function publish() {
    publishing.value = true;
    try {
      await store.publish({ ...draft });
      publishOpen.value = false;
      $q.notify({ type: 'positive', message: '已发布' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '发布失败' });
    } finally {
      publishing.value = false;
    }
  }

  onMounted(() => void load());

  return {
    products,
    search,
    loading,
    publishing,
    publishOpen,
    installingId,
    draft,
    load,
    debouncedLoad,
    install,
    publish,
  };
}
