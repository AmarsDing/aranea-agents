import { reactive, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import type { A2AProxyConfig } from './types';
import { useAgentDetailStore } from '../../stores/agents';
import { buildA2AAuthJSON, A2A_AUTH_TYPE_OPTIONS } from '../a2a/authUtils';

export function useAgentA2AProxyTab(
  agentId: () => string,
  a2aProxy: () => A2AProxyConfig | undefined,
  onSaved?: () => void,
) {
  const $q = useQuasar();
  const detailStore = useAgentDetailStore();
  const { saving } = storeToRefs(detailStore);
  const showSecret = ref(false);
  const authSecret = ref('');
  const mtls = reactive({ cert_file: '', key_file: '', ca_file: '' });
  const authTypeOptions = A2A_AUTH_TYPE_OPTIONS;
  const proxyForm = reactive<A2AProxyConfig>({
    remote_url: '',
    enable_streaming: true,
    timeout_seconds: 30,
    auth_type: 'none',
  });

  function parseAuthConfig(raw?: string, authType?: string) {
    authSecret.value = '';
    mtls.cert_file = '';
    mtls.key_file = '';
    mtls.ca_file = '';
    if (!raw) return;
    try {
      const parsed = JSON.parse(raw) as {
        api_key?: string;
        token?: string;
        cert_file?: string;
        key_file?: string;
        ca_file?: string;
      };
      if (authType === 'mtls') {
        mtls.cert_file = parsed.cert_file ?? '';
        mtls.key_file = parsed.key_file ?? '';
        mtls.ca_file = parsed.ca_file ?? '';
        return;
      }
      authSecret.value = parsed.api_key ?? parsed.token ?? '';
    } catch {
      authSecret.value = '';
    }
  }

  function buildAuthConfigJson(): string | undefined {
    return buildA2AAuthJSON(proxyForm.auth_type ?? 'none', authSecret.value, mtls);
  }

  watch(
    a2aProxy,
    (cfg) => {
      if (!cfg) return;
      proxyForm.remote_url = cfg.remote_url ?? '';
      proxyForm.enable_streaming = cfg.enable_streaming ?? true;
      proxyForm.timeout_seconds = cfg.timeout_seconds ?? 30;
      proxyForm.auth_type = cfg.auth_type ?? 'none';
      parseAuthConfig(cfg.auth_config_json, proxyForm.auth_type);
    },
    { immediate: true },
  );

  async function saveProxy() {
    if (!proxyForm.remote_url.trim()) {
      $q.notify({ type: 'negative', message: '远程 URL 不能为空' });
      return;
    }
    const authType = proxyForm.auth_type?.trim() || 'none';
    if (authType === 'api_key' || authType === 'bearer') {
      if (!authSecret.value.trim()) {
        $q.notify({ type: 'negative', message: '请填写鉴权密钥' });
        return;
      }
    }
    if (authType === 'mtls' && (!mtls.cert_file.trim() || !mtls.key_file.trim())) {
      $q.notify({ type: 'negative', message: 'mTLS 需填写 cert_file 与 key_file' });
      return;
    }
    try {
      const payload: A2AProxyConfig = {
        ...proxyForm,
        auth_type: authType === 'none' ? undefined : authType,
        auth_config_json: buildAuthConfigJson(),
      };
      await detailStore.patch(agentId(), { a2a_proxy_config: payload });
      $q.notify({ type: 'positive', message: 'A2A 代理配置已保存' });
      onSaved?.();
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '保存失败' });
    }
  }

  return {
    saving,
    showSecret,
    authSecret,
    mtls,
    authTypeOptions,
    proxyForm,
    saveProxy,
  };
}
