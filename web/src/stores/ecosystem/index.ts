import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  installEcosystemProduct,
  listEcosystemProducts,
  publishEcosystemProduct,
} from '../../features/ecosystem/api';
import type { EcosystemProduct } from '../../features/ecosystem/types';

export const useEcosystemStore = defineStore('ecosystem', () => {
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

  return { products, loading, load, install, publish };
});
