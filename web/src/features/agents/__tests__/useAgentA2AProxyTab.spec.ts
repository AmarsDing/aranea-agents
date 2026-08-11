import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { reactive, ref } from 'vue';
import { useAgentA2AProxyTab } from '../useAgentA2AProxyTab';
import { discoverRemoteAgent } from '../../a2a/api';

const notify = vi.fn();
const patch = vi.fn();

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));

vi.mock('../../../stores/agents', () => ({
  useAgentDetailStore: () => reactive({ saving: ref(false), patch }),
}));

vi.mock('../../a2a/api', () => ({
  discoverRemoteAgent: vi.fn(),
}));

const discoverMock = vi.mocked(discoverRemoteAgent);

function makeTab(a2aProxy?: unknown) {
  return useAgentA2AProxyTab(
    () => 'agent-1',
    () => a2aProxy as never,
  );
}

describe('useAgentA2AProxyTab', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    notify.mockClear();
    patch.mockClear();
    discoverMock.mockReset();
  });

  describe('testConnection', () => {
    it('calls discoverRemoteAgent with built auth json and notifies success', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'bearer';
      tab.authSecret.value = 'secret-token';
      discoverMock.mockResolvedValue({
        agent_id: 'remote-1',
        display_name: 'Remote Agent',
        workspace: 'ws',
        enabled: true,
        capabilities: [{ name: 'chat' }],
        updated_at: '',
      } as never);

      await tab.testConnection();

      expect(discoverMock).toHaveBeenCalledWith({
        remote_url: 'https://remote.example.com',
        auth_type: 'bearer',
        auth_config_json: JSON.stringify({ token: 'secret-token' }),
      });
      const success = notify.mock.calls.find((c) => c[0]?.type === 'positive');
      expect(success, 'expected a positive notify').toBeTruthy();
      expect(success?.[0]?.message).toContain('Remote Agent');
    });

    it('rejects empty remote_url without calling the API', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = '  ';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects api_key auth without secret', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'api_key';
      tab.authSecret.value = '';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects mtls without key_file', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'mtls';
      tab.mtls.cert_file = '/certs/client.crt';
      tab.mtls.key_file = '';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects non-http(s) remote_url without calling the API', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'ftp://remote.example.com';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('notifies negative when the API call fails', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      discoverMock.mockRejectedValue(new Error('connection refused'));

      await tab.testConnection();

      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative', message: 'connection refused' }));
    });
  });

  describe('saveProxy validation', () => {
    it('rejects empty remote_url without patching', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = '';

      await tab.saveProxy();

      expect(patch).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });
  });
});
