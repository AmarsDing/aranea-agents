import { ref, type Ref } from 'vue';
import { useAvatarCatalogStore } from '../../stores/avatar';
import { resolveAvatarAssetFetchId } from '../avatar/resolveAvatarAssetFetchId';

/** Agent settings avatar: picker dialog + thumbnail cache warmup after load/save. */
export function useAgentAvatarIcon(icon: Ref<string | undefined | null>) {
  const avatarCatalogStore = useAvatarCatalogStore();
  const avatarPickerOpen = ref(false);

  /** Forget cached thumb and refetch (settings page open / post-save). */
  async function primeThumbnailCache() {
    const fetchId = await resolveAvatarAssetFetchId(avatarCatalogStore, icon.value);
    if (!fetchId) return;
    avatarCatalogStore.forgetThumbnail(fetchId);
    await avatarCatalogStore.ensureThumbnail(fetchId);
  }

  return { avatarPickerOpen, primeThumbnailCache };
}
