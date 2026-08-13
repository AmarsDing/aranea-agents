import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listMcpServers,
  listMcpServersPaged,
  createMcpServer,
  updateMcpServer,
  deleteMcpServer,
  testMcpServer,
  validateMcpServer,
  listMcpUserCredentials,
  upsertMcpUserCredential,
  deleteMcpUserCredential,
  type PlatformResourceInput,
  type McpServerListQuery,
} from '../../features/mcp/api';
import type {
  McpServerRow,
  McpServerTestResult,
  McpServerValidateResult,
  McpUserCredential,
  McpUserCredentialInput,
} from '../../features/mcp/types';

export const useMcpStore = defineStore('mcp', () => {
  const servers = ref<McpServerRow[]>([]);
  const total = ref(0);
  const loading = ref(false);

  async function loadServers(query?: McpServerListQuery) {
    loading.value = true;
    try {
      if (query) {
        const result = await listMcpServersPaged(query);
        servers.value = result.items;
        total.value = result.total;
        return result;
      }
      servers.value = await listMcpServers();
      total.value = servers.value.length;
      return { items: servers.value, total: total.value, page: 1, page_size: total.value };
    } finally {
      loading.value = false;
    }
  }

  async function addServer(payload: PlatformResourceInput) {
    const created = await createMcpServer(payload);
    servers.value.push(created);
    total.value += 1;
    return created;
  }

  async function editServer(id: string, payload: Partial<PlatformResourceInput>) {
    const updated = await updateMcpServer(id, payload);
    servers.value = servers.value.map((s) => (s.id === id ? updated : s));
    return updated;
  }

  async function removeServer(id: string) {
    await deleteMcpServer(id);
    servers.value = servers.value.filter((s) => s.id !== id);
    total.value = Math.max(0, total.value - 1);
  }

  async function test(id: string): Promise<McpServerTestResult> {
    return testMcpServer(id);
  }

  async function validate(enabled: boolean, configJson: string): Promise<McpServerValidateResult> {
    return validateMcpServer(enabled, configJson);
  }

  async function fetchUserCredentials(mcpServerId: string, userId: string): Promise<McpUserCredential[]> {
    return listMcpUserCredentials(mcpServerId, userId);
  }

  async function saveUserCredential(mcpServerId: string, userId: string, input: McpUserCredentialInput) {
    return upsertMcpUserCredential(mcpServerId, userId, input);
  }

  async function removeUserCredential(mcpServerId: string, userId: string, credentialKey: string) {
    return deleteMcpUserCredential(mcpServerId, userId, credentialKey);
  }

  return {
    servers,
    total,
    loading,
    loadServers,
    addServer,
    editServer,
    removeServer,
    test,
    validate,
    fetchUserCredentials,
    saveUserCredential,
    removeUserCredential,
  };
});
