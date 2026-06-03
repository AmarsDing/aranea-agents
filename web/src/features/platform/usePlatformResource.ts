import { computed, ref } from 'vue';
import {
  createPlatformResource,
  deletePlatformResource,
  listPlatformResources,
  updatePlatformResource,
  type PlatformResource,
  type PlatformResourceInput,
  type PlatformResourceName,
} from './api';

export function usePlatformResource(resource: PlatformResourceName) {
  const rows = ref<PlatformResource[]>([]);
  const loading = ref(false);
  const keyword = ref('');

  const filteredRows = computed(() => {
    const q = keyword.value.trim().toLowerCase();
    if (!q) return rows.value;
    return rows.value.filter((row) =>
      [row.key, row.name, row.description, row.provider, row.model].some((value) => value.toLowerCase().includes(q)),
    );
  });

  async function load() {
    loading.value = true;
    try {
      rows.value = await listPlatformResources(resource);
    } finally {
      loading.value = false;
    }
  }

  async function create(payload: PlatformResourceInput) {
    const created = await createPlatformResource(resource, payload);
    rows.value = [created, ...rows.value];
    return created;
  }

  async function update(id: string, payload: Partial<PlatformResourceInput>) {
    const updated = await updatePlatformResource(resource, id, payload);
    rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    return updated;
  }

  async function remove(id: string) {
    await deletePlatformResource(resource, id);
    rows.value = rows.value.filter((row) => row.id !== id);
  }

  return {
    rows,
    filteredRows,
    keyword,
    loading,
    load,
    create,
    update,
    remove,
  };
}
