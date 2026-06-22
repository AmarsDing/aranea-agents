import { computed, ref } from 'vue';
import { useQuasar } from 'quasar';
import { errorMessage } from './providerUtils';
import { usePlatformStore } from '../../stores/platform';
import { findModelPreset, type ProviderModelPreset } from '../../config/providerPresets';
import type { Ref, ComputedRef } from 'vue';
import type { ProviderForm } from './types';
import type { CatalogModelSummary } from '../../services/kratos/model_catalog/v1/index';

export function useProviderInspect(deps: {
  providerForm: ProviderForm;
  providerAddMode: Ref<'catalog' | 'custom'>;
  providerCreateInspectFingerprint: Ref<string>;
  providerEditIdentityAtOpen: Ref<string>;
  editingId: Ref<string>;
  isProviderResource: ComputedRef<boolean>;
  currentProviderPreset: ComputedRef<{ key?: string; authType?: string } | undefined>;
  applyModelPresetValues: (preset: ProviderModelPreset, overwrite?: boolean) => void;
  findCatalogModel: (modelId: string) => CatalogModelSummary | undefined;
  applyCatalogModel: (modelId: string) => void;
  catalogProviderId: Ref<string>;
  catalogModels: Ref<CatalogModelSummary[]>;
}) {
  const platformStore = usePlatformStore();
  const $q = useQuasar();

  const checkingModel = ref(false);

  const isLocalProviderModel = computed(() => {
    if (deps.providerForm.provider_type === 'ollama') return true;
    const raw = deps.providerForm.api_base_url.trim();
    if (!raw) return false;
    return /^(https?|wss?):\/\/(localhost|127\.0\.0\.1|\[::1\])([/:?#]|$)/i.test(raw);
  });

  const hasInspectApiKey = computed(() => {
    if (deps.providerForm.api_key.trim()) return true;
    if (deps.providerForm.secret_id.trim() && deps.providerForm.secret_key.trim()) return true;
    if (deps.providerForm.aws_region.trim()) return true;
    if (deps.editingId.value && deps.providerForm.api_key_set) return true;
    return false;
  });

  const canInspectProviderModel = computed(() => {
    if (!deps.providerForm.provider_code.trim() || !deps.providerForm.model_api_id.trim()) return false;
    if (isLocalProviderModel.value) return true;
    return hasInspectApiKey.value;
  });

  const providerIdentityChanged = computed(() => {
    if (!deps.editingId.value || !deps.isProviderResource.value) return false;
    const cur = `${deps.providerForm.provider_code.trim()}\0${deps.providerForm.model_api_id.trim()}`;
    return cur !== deps.providerEditIdentityAtOpen.value;
  });

  function providerCreateInspectFingerprintValue(): string {
    return [
      deps.providerForm.provider_code.trim(),
      deps.providerForm.model_api_id.trim(),
      deps.providerForm.api_base_url.trim(),
      deps.providerForm.api_key.trim(),
      deps.providerForm.provider_type.trim(),
      deps.providerForm.variant.trim(),
      deps.providerForm.secret_id.trim(),
      deps.providerForm.secret_key.trim(),
      deps.providerForm.aws_region.trim(),
    ].join('\0');
  }

  const canSubmitNewProviderModel = computed(() => {
    if (!deps.isProviderResource.value) return true;
    if (deps.editingId.value && !providerIdentityChanged.value) return true;
    if (isLocalProviderModel.value) return true;
    if (
      deps.catalogProviderId.value &&
      deps.providerForm.model_api_id.trim() &&
      deps.catalogModels.value.some((m) => m.id === deps.providerForm.model_api_id.trim())
    ) {
      return true;
    }
    if (
      deps.providerAddMode.value === 'catalog' &&
      deps.catalogProviderId.value &&
      deps.providerForm.catalog_managed &&
      deps.providerForm.model_api_id.trim()
    ) {
      return true;
    }
    const saved = deps.providerCreateInspectFingerprint.value;
    if (!saved) return false;
    return saved === providerCreateInspectFingerprintValue();
  });

  async function inspectCurrentProviderModel() {
    const code = deps.providerForm.provider_code.trim();
    const model = deps.providerForm.model_api_id.trim();
    if (!code || !model) {
      $q.notify({ type: 'negative', message: '请先填写 Provider 名称和模型ID' });
      return;
    }
    if (!canInspectProviderModel.value) {
      $q.notify({ type: 'warning', message: '非本地模型需填写 API 密钥后才能检查' });
      return;
    }
    checkingModel.value = true;
    try {
      const result = await platformStore.inspectModel({
        resource_id: deps.editingId.value,
        provider_code: code,
        provider_type: deps.providerForm.provider_type,
        model_api_id: model,
        api_base_url: deps.providerForm.api_base_url.trim(),
        api_key: deps.providerForm.api_key.trim(),
        variant: deps.providerForm.variant,
        secret_id: deps.providerForm.secret_id.trim(),
        secret_key: deps.providerForm.secret_key.trim(),
        aws_region: deps.providerForm.aws_region.trim(),
      });
      if (!result.ok) {
        if (!deps.editingId.value) {
          deps.providerCreateInspectFingerprint.value = '';
        }
        const preset = findModelPreset(deps.currentProviderPreset.value?.key || code, model);
        const catalogModel = deps.findCatalogModel(model);
        if (catalogModel && deps.catalogProviderId.value) {
          deps.applyCatalogModel(catalogModel.id || model);
          deps.providerForm.catalog_managed = true;
          if (!deps.editingId.value) {
            deps.providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
          }
          $q.notify({
            type: 'warning',
            message: `${result.message || '未获取到模型参数'}；已使用 models.dev 目录参数回填`,
          });
          return;
        }
        if (preset) {
          deps.applyModelPresetValues(preset, true);
          deps.providerForm.metadata_source = `${deps.currentProviderPreset.value?.key || code}-preset`;
          deps.providerForm.raw_metadata_json = JSON.stringify({
            source: 'frontend-provider-preset',
            provider: deps.currentProviderPreset.value?.key || code,
            model,
          });
          $q.notify({ type: 'warning', message: `${result.message || '未获取到模型参数'}；已使用前端预设参数回填` });
          return;
        }
        $q.notify({ type: 'warning', message: result.message || '未获取到模型参数，也没有匹配的预设参数' });
        return;
      }
      if (!deps.editingId.value) {
        deps.providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
      }
      if (deps.catalogProviderId.value && deps.providerForm.model_api_id.trim()) {
        const cm = deps.findCatalogModel(deps.providerForm.model_api_id.trim());
        if (cm && !deps.providerForm.input_price_usd_per_1m && !deps.providerForm.output_price_usd_per_1m) {
          deps.applyCatalogModel(cm.id || deps.providerForm.model_api_id.trim());
        }
      }
      if (result.enable_token_tailoring) deps.providerForm.enable_token_tailoring = true;
      if (!deps.providerForm.model_display_name.trim()) {
        deps.providerForm.model_display_name = result.model_display_name || model;
      }
      if (result.model_size_label && !deps.providerForm.model_size_label.trim()) {
        deps.providerForm.model_size_label = result.model_size_label;
      }
      if (result.raw_metadata_json && !deps.providerForm.raw_metadata_json.trim()) {
        deps.providerForm.raw_metadata_json = result.raw_metadata_json;
      }
      $q.notify({ type: 'positive', message: result.message || '已验证 Provider 连通性' });
    } catch (error) {
      if (!deps.editingId.value) {
        deps.providerCreateInspectFingerprint.value = '';
      }
      $q.notify({ type: 'negative', message: errorMessage(error) });
    } finally {
      checkingModel.value = false;
    }
  }

  return {
    checkingModel,
    isLocalProviderModel,
    hasInspectApiKey,
    canInspectProviderModel,
    providerIdentityChanged,
    canSubmitNewProviderModel,
    providerCreateInspectFingerprintValue,
    inspectCurrentProviderModel,
  };
}
