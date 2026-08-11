import { describe, expect, it } from 'vitest';
import { createLlmProviderModelServiceClient } from '../../../services/kratos/llm_provider_model/v1/index';

type RecordedCall = { path: string; method: string; body: string | null };

// Regression guard for the model registry search box: the proto used to declare
// ListProviderModels(google.protobuf.Empty), so the generated client dropped
// page/pageSize/search and the request went out as a bare GET with no query.
describe('LlmProviderModelService generated client', () => {
  it('ListProviderModels serializes page/pageSize/search into the query string', async () => {
    const calls: RecordedCall[] = [];
    const client = createLlmProviderModelServiceClient((req) => {
      calls.push({ path: req.path, method: req.method, body: req.body });
      return Promise.resolve({ items: [], total: 0, page: 1, pageSize: 20 });
    });

    await client.ListProviderModels({ page: 2, pageSize: 10, search: 'gpt 4o' });

    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].body).toBeNull();
    expect(calls[0].path).toContain('page=2');
    expect(calls[0].path).toContain('pageSize=10');
    expect(calls[0].path).toContain('search=gpt%204o');
  });
});
