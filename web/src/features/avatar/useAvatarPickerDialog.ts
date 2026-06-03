import { computed, ref, watch, type Ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { avatarUploadErrorMessage, validateAvatarFileForUpload } from '../../features/avatar/api';
import type { AvatarAsset } from '../../features/avatar/types';
import { prepareAvatarUploadFile } from '../../features/avatar/prepareAvatarUpload';
import { useAvatarCatalogStore } from '../../stores/avatar';

export type AvatarPickerScope = 'agent' | 'channel';

export type AvatarAssetGroup = {
  key: 'agent' | 'channel';
  label: string;
  items: AvatarAsset[];
};

function filterByScope(assets: AvatarAsset[], scope: AvatarPickerScope): AvatarAsset[] {
  if (scope === 'channel') {
    return assets.filter((a) => a.category === 'channel');
  }
  return assets.filter((a) => a.category !== 'channel');
}

function groupSystemAssets(assets: AvatarAsset[]): AvatarAssetGroup[] {
  const agentItems = assets.filter((a) => a.category !== 'channel');
  const channelItems = assets.filter((a) => a.category === 'channel');
  const groups: AvatarAssetGroup[] = [];
  if (agentItems.length > 0) {
    groups.push({ key: 'agent', label: 'Agent 头像', items: agentItems });
  }
  if (channelItems.length > 0) {
    groups.push({ key: 'channel', label: 'Channel 头像', items: channelItems });
  }
  return groups;
}

/** 头像选择弹层：组合 Store + 本地 UI 状态；供 AgentAvatarPicker / ChannelIconPicker 使用 */
export function useAvatarPickerDialog(options: {
  modelValue: Ref<string>;
  open: Ref<boolean>;
  scope?: AvatarPickerScope;
}) {
  const scope = options.scope ?? 'agent';
  const store = useAvatarCatalogStore();
  const { pickerSystem, pickerMine } = storeToRefs(store);
  const $q = useQuasar();

  const tab = ref<'system' | 'mine'>('system');
  const loading = ref(false);
  const uploading = ref(false);
  const selectedId = ref(options.modelValue.value);
  const fileInput = ref<HTMLInputElement | null>(null);

  const visibleAssets = computed(() => {
    const list = tab.value === 'system' ? pickerSystem.value : pickerMine.value;
    return tab.value === 'system' ? filterByScope(list, scope) : list;
  });

  const systemGroups = computed(() => groupSystemAssets(pickerSystem.value));

  watch(
    () => options.open.value,
    (isOpen) => {
      if (isOpen) {
        selectedId.value = options.modelValue.value;
        void loadPicker();
      }
    },
    { immediate: true },
  );

  watch(
    () => options.modelValue.value,
    (v) => {
      selectedId.value = v;
    },
  );

  async function loadPicker() {
    loading.value = true;
    try {
      await store.ensurePickerAssets();
      if (!selectedId.value) {
        const first = visibleAssets.value[0];
        if (first) selectedId.value = first.id;
      }
      const allSystem = systemGroups.value.flatMap((g) => g.items);
      const thumbs = tab.value === 'system' ? allSystem : visibleAssets.value;
      await Promise.all(thumbs.slice(0, 60).map((a) => store.ensureThumbnail(a.id)));
    } finally {
      loading.value = false;
    }
  }

  async function uploadFromFile(file: File) {
    const invalid = validateAvatarFileForUpload(file);
    if (invalid) {
      $q.notify({ type: 'negative', message: invalid });
      return;
    }
    uploading.value = true;
    try {
      const prepared = await prepareAvatarUploadFile(file);
      const uploaded = await store.uploadAvatarFromFile(prepared);
      selectedId.value = uploaded.id;
      tab.value = 'mine';
      $q.notify({ type: 'positive', message: scope === 'channel' ? '图标已上传' : '头像已上传' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : avatarUploadErrorMessage(err) });
    } finally {
      uploading.value = false;
    }
  }

  return {
    tab,
    loading,
    uploading,
    selectedId,
    fileInput,
    visibleAssets,
    systemGroups,
    loadPicker,
    uploadFromFile,
    scope,
  };
}
