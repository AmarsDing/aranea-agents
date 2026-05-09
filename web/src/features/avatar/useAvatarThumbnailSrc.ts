import { ref, watchEffect, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { useAvatarCatalogStore } from "../../stores/avatar";
import { isAvatarAssetRef } from "./iconModel";

/**
 * 响应式头像展示用 data URL（库内 asset 走 Store 缓存 + ensureThumbnail）。
 * 订阅 `thumbnailById`；支持 agent.icon 存 **asset id** 或 **asset_key**（经目录解析）。
 */
export function useAvatarThumbnailSrc(iconRef: Ref<string | undefined | null>) {
  const store = useAvatarCatalogStore();
  const { thumbnailById } = storeToRefs(store);
  const src = ref("");
  watchEffect((onCleanup) => {
    void thumbnailById.value;
    const raw = iconRef.value?.trim() ?? "";
    if (!raw) {
      src.value = "";
      return;
    }
    if (/^(https?:|data:|blob:)/i.test(raw)) {
      src.value = raw;
      return;
    }
    let cancelled = false;
    onCleanup(() => {
      cancelled = true;
    });
    void (async () => {
      let fetchId = raw;
      if (!isAvatarAssetRef(raw)) {
        await store.ensureAgentsCatalog();
        if (cancelled) return;
        const hit = store.agentsCatalog.find((a) => a.id === raw || (a.key && a.key === raw));
        if (!hit?.id) {
          src.value = "";
          return;
        }
        fetchId = hit.id;
      }
      await store.ensureThumbnail(fetchId);
      if (cancelled) return;
      src.value = thumbnailById.value[fetchId] ?? "";
    })();
  });
  return src;
}
