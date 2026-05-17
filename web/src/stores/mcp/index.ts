import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listMcpServers,
  createMcpServer,
  updateMcpServer,
  deleteMcpServer,
  testMcpServer,
  type PlatformResource,
  type PlatformResourceInput
} from "../../features/mcp/api";
import type { McpServerTestResult } from "../../features/mcp/types";

export const useMcpStore = defineStore("mcp", () => {
  const servers = ref<PlatformResource[]>([]);
  const loading = ref(false);

  async function loadServers() {
    loading.value = true;
    try {
      servers.value = await listMcpServers();
    } finally {
      loading.value = false;
    }
  }

  async function addServer(payload: PlatformResourceInput) {
    const created = await createMcpServer(payload);
    servers.value.push(created);
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
  }

  async function test(id: string): Promise<McpServerTestResult> {
    return testMcpServer(id);
  }

  return { servers, loading, loadServers, addServer, editServer, removeServer, test };
});
