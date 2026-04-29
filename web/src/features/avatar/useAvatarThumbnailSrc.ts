import { ref, watch, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { useAvatarCatalogStore } from "../../stores/avatar";
import { isAvatarAssetRef } from "./iconModel";

/**
 * 响应式头像展示用 data URL（库内 asset 走 Store 缓存 + ensureThumbnail）。
 */
export function useAvatarThumbnailSrc(iconRef: Ref<string | undefined | null>) {
  const store = useAvatarCatalogStore();
  const { thumbnailById } = storeToRefs(store);
  const src = ref("");
  watch(
    iconRef,
    async (raw) => {
      const v = raw?.trim() ?? "";
      if (!v) {
        src.value = "";
        return;
      }
      if (/^(https?:|data:|blob:)/i.test(v)) {
        src.value = v;
        return;
      }
      if (!isAvatarAssetRef(v)) {
        src.value = "";
        return;
      }
      await store.ensureThumbnail(v);
      src.value = thumbnailById.value[v] ?? "";
    },
    { immediate: true }
  );
  return src;
}
