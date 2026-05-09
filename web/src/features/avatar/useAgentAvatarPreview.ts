import { computed, type ComputedRef, type Ref } from "vue";
import { quasarAvatarIconForAgentField } from "./iconModel";
import { useAvatarThumbnailSrc } from "./useAvatarThumbnailSrc";

type IconRef = Ref<string | undefined | null> | ComputedRef<string | undefined | null>;

/** 将 agents.icon 映射为 QAvatar 的 `icon` + `<img src>` */
export function useAgentAvatarPreview(iconRef: IconRef) {
  const thumb = useAvatarThumbnailSrc(iconRef);
  const avatarSrc = computed(() => {
    const v = iconRef.value?.trim() ?? "";
    if (/^(https?:|data:|blob:)/i.test(v)) return v;
    return thumb.value;
  });
  const avatarIcon = computed(() => {
    if (avatarSrc.value) return undefined;
    return quasarAvatarIconForAgentField(iconRef.value ?? "");
  });
  return { avatarSrc, avatarIcon };
}
