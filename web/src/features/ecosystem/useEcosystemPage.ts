import { onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import {
  installEcosystemProduct,
  listEcosystemProducts,
  publishEcosystemProduct,
  type EcosystemProduct,
} from "./api";

export function useEcosystemPage() {
  const $q = useQuasar();
  const products = ref<EcosystemProduct[]>([]);
  const search = ref("");
  const loading = ref(false);
  const publishing = ref(false);
  const publishOpen = ref(false);
  const installingId = ref("");
  const draft = reactive({ name: "", display_name: "", description: "", type: "skill_pack" });

  async function load() {
    loading.value = true;
    try {
      products.value = await listEcosystemProducts(search.value.trim());
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载失败" });
    } finally {
      loading.value = false;
    }
  }

  async function install(p: EcosystemProduct) {
    installingId.value = p.id;
    try {
      await installEcosystemProduct(p.id);
      $q.notify({ type: "positive", message: "安装成功" });
      await load();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "安装失败" });
    } finally {
      installingId.value = "";
    }
  }

  async function publish() {
    publishing.value = true;
    try {
      await publishEcosystemProduct({ ...draft });
      publishOpen.value = false;
      $q.notify({ type: "positive", message: "已发布" });
      await load();
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "发布失败" });
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
    install,
    publish,
  };
}
