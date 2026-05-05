import { computed, ref, watch, type Ref } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import { avatarUploadErrorMessage, validateAvatarFileForUpload } from "../../features/avatar/api";
import { useAvatarCatalogStore } from "../../stores/avatar";

/** 头像选择弹层：组合 Store + 本地 UI 状态；供 AgentAvatarPicker 使用，组件内不直接调 api/store */
export function useAvatarPickerDialog(options: { modelValue: Ref<string>; open: Ref<boolean> }) {
  const store = useAvatarCatalogStore();
  const { pickerSystem, pickerMine } = storeToRefs(store);
  const $q = useQuasar();

  const tab = ref<"system" | "mine">("system");
  const loading = ref(false);
  const uploading = ref(false);
  const selectedId = ref(options.modelValue.value);
  const fileInput = ref<HTMLInputElement | null>(null);

  const visibleAssets = computed(() => (tab.value === "system" ? pickerSystem.value : pickerMine.value));

  watch(
    () => options.open.value,
    (isOpen) => {
      if (isOpen) {
        selectedId.value = options.modelValue.value;
        void loadPicker();
      }
    },
    { immediate: true }
  );

  watch(
    () => options.modelValue.value,
    (v) => {
      selectedId.value = v;
    }
  );

  async function loadPicker() {
    loading.value = true;
    try {
      await store.ensurePickerAssets();
      if (!selectedId.value && pickerSystem.value[0]) selectedId.value = pickerSystem.value[0].id;
    } finally {
      loading.value = false;
    }
  }

  async function uploadFromFile(file: File) {
    const invalid = validateAvatarFileForUpload(file);
    if (invalid) {
      $q.notify({ type: "negative", message: invalid });
      return;
    }
    uploading.value = true;
    try {
      const uploaded = await store.uploadAvatarFromFile(file);
      selectedId.value = uploaded.id;
      tab.value = "mine";
      $q.notify({ type: "positive", message: "头像已上传" });
    } catch (err) {
      $q.notify({ type: "negative", message: avatarUploadErrorMessage(err) });
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
    loadPicker,
    uploadFromFile
  };
}
