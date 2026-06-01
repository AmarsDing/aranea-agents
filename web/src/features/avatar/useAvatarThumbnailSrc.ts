import { ref, watch, type ComputedRef, type Ref } from "vue";
import { useAvatarCatalogStore } from "../../stores/avatar";
import { resolveAvatarAssetFetchId } from "./resolveAvatarAssetFetchId";

type IconRef = Ref<string | undefined | null> | ComputedRef<string | undefined | null>;

export function useAvatarThumbnailSrc(iconRef: IconRef) {
  const store = useAvatarCatalogStore();
  const src = ref("");

  watch(
    iconRef,
    async (raw, _old, onCleanup) => {
      const trimmed = raw?.trim() ?? "";
      if (!trimmed) {
        src.value = "";
        return;
      }
      if (/^(https?:|data:|blob:)/i.test(trimmed)) {
        src.value = trimmed;
        return;
      }
      let cancelled = false;
      onCleanup(() => {
        cancelled = true;
      });
      const fetchId = await resolveAvatarAssetFetchId(store, trimmed);
      if (cancelled) return;
      if (!fetchId) {
        src.value = "";
        return;
      }
      await store.ensureThumbnail(fetchId);
      if (cancelled) return;
      src.value = store.thumbnailById[fetchId] ?? "";
    },
    { immediate: true },
  );

  return src;
}
