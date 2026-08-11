import { describe, expect, it } from 'vitest';
import { createMCPServerServiceClient } from '../../../services/kratos/mcp_server/v1/index';

type RecordedCall = { path: string; method: string; body: string | null };

// Regression guard for the MCP server registry page: the rpc used to declare
// ListMCPServers(google.protobuf.Empty), so the generated client dropped
// page/pageSize/search and the request went out as a bare GET with no query,
// silently breaking server-side pagination and search.
describe('MCPServerService generated client', () => {
  it('ListMCPServers serializes page/pageSize/search into the query string', async () => {
    const calls: RecordedCall[] = [];
    const client = createMCPServerServiceClient((req) => {
      calls.push({ path: req.path, method: req.method, body: req.body });
      return Promise.resolve({ items: [], total: 0, page: 1, pageSize: 20 });
    });

    await client.ListMCPServers({ page: 2, pageSize: 10, search: 'play wright' });

    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].body).toBeNull();
    expect(calls[0].path).toContain('page=2');
    expect(calls[0].path).toContain('pageSize=10');
    expect(calls[0].path).toContain('search=play%20wright');
  });

  it('ListMCPServers with no params sends a bare GET (legacy unpaginated path)', async () => {
    const calls: RecordedCall[] = [];
    const client = createMCPServerServiceClient((req) => {
      calls.push({ path: req.path, method: req.method, body: req.body });
      return Promise.resolve({ items: [], total: 0, page: 1, pageSize: 20 });
    });

    await client.ListMCPServers({});

    expect(calls).toHaveLength(1);
    expect(calls[0].path).toBe('v1/mcp-servers');
  });
});
