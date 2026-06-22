import { computed, ref } from 'vue';
import {
  PROVIDER_PRESETS,
  findModelPreset,
  findProviderPreset,
  type ProviderModelPreset,
} from '../../config/providerPresets';
import type { Ref } from 'vue';

export function useProviderPresets(deps: {
  providerForm: Record<string, any>;
  applyModelPresetValues: (preset: ProviderModelPreset, overwrite?: boolean) => void;
}) {
  const providerPresetKey = ref('');

  const currentProviderPreset = computed(() =>
    findProviderPreset(providerPresetKey.value || deps.providerForm.provider_code),
  );

  const providerPresetOptions = computed(() =>
    PROVIDER_PRESETS.map((preset) => ({
      label: preset.label,
      value: preset.key,
      caption: `${preset.apiBaseUrl || '手动配置'} · ${metadataLabel(preset.metadataApi)}`,
    })),
  );

  function applyModelPreset(modelId: string) {
    const preset = findModelPreset(providerPresetKey.value || deps.providerForm.provider_code, modelId);
    if (!preset) return;
    deps.applyModelPresetValues(preset, false);
  }

  function setCustomModelValue(value: string, done?: (value: string, mode?: 'add' | 'add-unique' | 'toggle') => void) {
    done?.(value, 'add-unique');
    deps.providerForm.model_api_id = value;
  }

  function metadataLabel(value: string) {
    if (value === 'full') return '可查询参数';
    if (value === 'partial') return '可验证模型';
    if (value === 'limited') return '有限查询';
    return '手动维护';
  }

  return {
    providerPresetKey,
    currentProviderPreset,
    providerPresetOptions,
    applyModelPreset,
    setCustomModelValue,
    metadataLabel,
  };
}
