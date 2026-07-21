import { computed, defineComponent, ref } from 'vue';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useProviderWizard } from '../useProviderWizard';
import { usePlatformStore } from '../../../stores/platform';
import ProviderWizardStep1Connect from '../../../components/platform/ProviderWizardStep1Connect.vue';
import type { PlatformResource, PlatformResourceName, ProviderForm } from '../types';
import type { CatalogModelSummary } from '../../../services/kratos/model_catalog/v1/index';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn() }),
}));

vi.mock('../../model-catalog/api', () => ({
  listCatalogProviders: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listCatalogModels: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}));

import { listCatalogModels } from '../../model-catalog/api';

const QSelectStub = defineComponent({
  name: 'QSelect',
  props: ['label', 'useInput', 'newValueMode', 'modelValue', 'options'],
  template: '<div class="q-select-stub" />',
});

const quasarStubs = {
  QSelect: QSelectStub,
  QInput: true,
  QBtn: true,
  QBtnToggle: true,
  QToggle: true,
  QBanner: true,
  QItem: true,
  QItemSection: true,
  QItemLabel: true,
};

function withSetup<T>(composable: () => T): T {
  let result!: T;
  mount(
    defineComponent({
      setup() {
        result = composable();
        return () => null;
      },
    }),
  );
  return result;
}

function createWizard(editingId = '') {
  return withSetup(() =>
    useProviderWizard({
      editingId: ref(editingId),
      dialogOpen: ref(false),
      saving: ref(false),
      resource: computed(() => 'provider' as PlatformResourceName),
      isProviderResource: computed(() => true),
      rows: ref<PlatformResource[]>([]),
    }),
  );
}

describe('useProviderWizard · 自定义模式预设切换', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.mocked(listCatalogModels).mockReset().mockResolvedValue({ items: [], total: 0 });
  });

  it('切换到无 catalog 模型的预设时清空上一供应商的残留模型与定价', async () => {
    const wizard = createWizard();
    // 模拟目录模式选择 Anthropic 后的残留
    wizard.providerForm.model_api_id = 'claude-sonnet-4-5';
    wizard.providerForm.model_display_name = 'Claude Sonnet 4.5 (latest)';
    wizard.providerForm.context_window_k = 1000;
    wizard.providerForm.max_output_tokens = 64000;
    wizard.providerForm.input_price_usd_per_1m = 3;
    wizard.providerForm.output_price_usd_per_1m = 15;
    wizard.providerForm.cache_read_usd_per_1m = 0.3;

    await wizard.applyProviderPreset('ollama');

    expect(wizard.providerForm.provider_code).toBe('ollama');
    expect(wizard.providerForm.api_base_url).toBe('http://localhost:11434');
    // 残留模型数据必须清空，避免出现「ollama / claude-sonnet-4-5」组合
    expect(wizard.providerForm.model_api_id).toBe('');
    expect(wizard.providerForm.model_display_name).toBe('');
    expect(wizard.providerForm.input_price_usd_per_1m).toBe(0);
    expect(wizard.providerForm.output_price_usd_per_1m).toBe(0);
    expect(wizard.providerForm.cache_read_usd_per_1m).toBe(0);
    expect(wizard.providerForm.context_window_k).toBeNull();
  });

  it('预设存在 catalog 模型时自动选择首个模型', async () => {
    vi.mocked(listCatalogModels).mockResolvedValueOnce({
      items: [
        {
          id: 'llama3.1:8b',
          name: 'Llama 3.1 8B',
          contextTokens: 128000,
          outputTokens: 8192,
        } as CatalogModelSummary,
      ],
      total: 1,
    });
    const wizard = createWizard();
    await wizard.applyProviderPreset('ollama');
    expect(wizard.providerForm.model_api_id).toBe('llama3.1:8b');
    expect(wizard.providerForm.model_display_name).toBe('Llama 3.1 8B');
  });
});

describe('useProviderWizard · 编辑模式身份变更', () => {
  const deepseekRow = {
    id: 'res-deepseek',
    key: 'deepseek:deepseek-v4-flash',
    name: 'DeepSeek V4 Flash',
    provider: 'deepseek',
    model: 'deepseek-v4-flash',
    enabled: true,
    sort_order: 0,
    description: '',
    config_json: JSON.stringify({
      provider_type: 'openai',
      variant: 'deepseek',
      api_base_url: 'https://api.deepseek.com',
      api_key_set: true,
      catalog_managed: true,
      catalog_source: 'models.dev',
    }),
    metadata_json: '{}',
  } as unknown as PlatformResource;

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.mocked(listCatalogModels)
      .mockReset()
      .mockResolvedValue({
        items: [{ id: 'deepseek-v4-flash', name: 'DeepSeek V4 Flash' } as CatalogModelSummary],
        total: 1,
      });
  });

  it('修改 Provider ID 后即使模型仍命中 catalog 也必须重新检查', async () => {
    const wizard = createWizard('res-deepseek');
    await wizard.populateProviderForm(deepseekRow);
    expect(wizard.providerIdentityChanged.value).toBe(false);
    expect(wizard.canSubmitNewProviderModel.value).toBe(true);

    wizard.providerForm.provider_code = 'deepseek2';
    expect(wizard.providerIdentityChanged.value).toBe(true);
    // 模型仍命中 catalog，但 provider 已被手动篡改，catalog 命中不得放行
    expect(wizard.canSubmitNewProviderModel.value).toBe(false);
  });

  it('身份变更后检查通过则允许保存（编辑模式指纹闭环）', async () => {
    const wizard = createWizard('res-deepseek');
    await wizard.populateProviderForm(deepseekRow);
    wizard.providerForm.provider_code = 'deepseek2';

    const store = usePlatformStore();
    vi.spyOn(store, 'inspectModel').mockResolvedValue({
      ok: true,
      message: 'ok',
      model_display_name: '',
      model_size_label: '',
      raw_metadata_json: '',
      enable_token_tailoring: false,
    });

    await wizard.inspectCurrentProviderModel();

    // 编辑模式检查成功后必须记录指纹，否则「变更→检查→保存」流程不闭环
    expect(wizard.providerCreateInspectFingerprint.value).not.toBe('');
    expect(wizard.canSubmitNewProviderModel.value).toBe(true);
  });
});

function stubProviderForm(): ProviderForm {
  return {
    provider_type: 'ollama',
    variant: 'openai',
    model_api_id: '',
    provider_code: 'ollama',
    provider_display_name: 'Ollama（本地）',
    model_display_name: '',
    api_base_url: 'http://localhost:11434',
    api_key: '',
    secret_id: '',
    secret_key: '',
    aws_region: '',
    enabled: true,
  } as unknown as ProviderForm;
}

function mountStep1(overrides: Record<string, unknown> = {}) {
  return mount(ProviderWizardStep1Connect, {
    props: {
      providerForm: stubProviderForm(),
      editingId: null,
      providerAddMode: 'custom',
      catalogProviderSearch: '',
      catalogProviderId: '',
      catalogProviderHint: '',
      catalogProviderDocUrl: '',
      catalogProviderOptions: [],
      catalogLoading: false,
      catalogModelsHint: '',
      catalogModelsLoading: false,
      providerPresetKey: 'ollama',
      providerPresetOptions: [],
      providerRuntimeLocked: true,
      providerRuntimeSummary: 'Ollama',
      providerTypeOptions: [],
      showApiKey: false,
      apiKeyFieldHint: '',
      apiKeyMaskedPlaceholder: '',
      revealingCredentials: false,
      useCatalogModelPicker: false,
      providerModelOptions: [],
      providerCodeRule: () => true,
      providerRuntimeBindingPreview: '',
      providerIdentityChanged: false,
      variantOptions: [],
      currentAuthType: 'none',
      showSecretKey: false,
      secretKeyMaskedPlaceholder: '',
      canInspectProviderModel: false,
      checkingModel: false,
      filterCatalogModelsLocal: () => {},
      ...overrides,
    },
    global: { stubs: quasarStubs },
  });
}

describe('ProviderWizardStep1Connect · 模型输入', () => {
  function findModelSelect(wrapper: ReturnType<typeof mountStep1>) {
    const select = wrapper.findAllComponents(QSelectStub).find((s) => s.props('label') === '模型');
    expect(select).toBeTruthy();
    return select!;
  }

  it('自定义模式且无 catalog 模型时仍允许输入自定义模型 ID', () => {
    const wrapper = mountStep1();
    const modelSelect = findModelSelect(wrapper);
    expect(modelSelect.props('useInput')).toBe(true);
    expect(modelSelect.props('newValueMode')).toBe('add-unique');
  });

  it('目录模式不允许新增目录外模型', () => {
    const wrapper = mountStep1({ providerAddMode: 'catalog', useCatalogModelPicker: true });
    const modelSelect = findModelSelect(wrapper);
    expect(modelSelect.props('useInput')).toBe(true);
    expect(modelSelect.props('newValueMode')).toBeFalsy();
  });
});
