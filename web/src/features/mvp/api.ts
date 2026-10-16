export type ContentItem = {
  ID: string; Title: string; Body: string; SourceType: string; SourceURL: string; Status: string;
};
export type ReviewTask = {
  ID: string; ContentItemID: string; DraftTitle: string; DraftBody: string; Status: string;
  MachineFlags?: string[]; DecisionNote?: string;
};
export type PublishJob = {
  ID: string; Status: string; Platform: string; Title: string; PackageDir?: string; Mode: string;
  ArtifactID?: string;
};
export type Opportunity = {
  ID: string; Title: string; Summary: string; Audience: string; Status: string; Score: number;
};
export type Supplier = {
  ID: string; OpportunityID: string; Name: string; URL: string; PriceRange: string; Status: string; Notes: string;
};
export type IngestResult = {
  source: string; fetched: number; created: number; deduped: number; feed_url?: string; query?: string; errors?: string[];
};

async function j<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((data as any).error || res.statusText);
  return data as T;
}

export function packageFileURL(jobId: string, name: 'manifest.json' | 'body.md') {
  return `/api/v1/mvp/publish/jobs/${encodeURIComponent(jobId)}/files/${name}`;
}

export const mvpApi = {
  listContent: (q = '') => j<{ items: ContentItem[]; total: number }>(`/api/v1/mvp/content${q}`),
  createContent: (body: Partial<ContentItem>) => j<{ item: ContentItem }>('/api/v1/mvp/content', { method: 'POST', body: JSON.stringify(body) }),
  ingestRSS: (feed_url: string, workspace_id = '') =>
    j<IngestResult>('/api/v1/mvp/ingest/rss', { method: 'POST', body: JSON.stringify({ feed_url, workspace_id }) }),
  ingestSearch: (query: string, hits: { URL: string; Title: string; Snippet: string }[], workspace_id = '') =>
    j<IngestResult>('/api/v1/mvp/ingest/search', { method: 'POST', body: JSON.stringify({ query, hits, workspace_id }) }),
  listReview: () => j<{ items: ReviewTask[]; total: number }>('/api/v1/mvp/review/pending'),
  enqueueReview: (body: Partial<ReviewTask>) => j<ReviewTask>('/api/v1/mvp/review', { method: 'POST', body: JSON.stringify(body) }),
  approve: (id: string, note = 'ok') => j<{ review: ReviewTask; publish_job: PublishJob }>(`/api/v1/mvp/review/${id}/approve`, { method: 'POST', body: JSON.stringify({ reviewer_id: 'ops', note, platform: 'generic' }) }),
  reject: (id: string, note: string) => j<ReviewTask>(`/api/v1/mvp/review/${id}/reject`, { method: 'POST', body: JSON.stringify({ reviewer_id: 'ops', note }) }),
  listPublish: () => j<{ items: PublishJob[] }>('/api/v1/mvp/publish/jobs?status=packaged'),
  listOpp: (audience = '') => j<{ items: Opportunity[] }>(`/api/v1/mvp/opportunities${audience ? `?audience=${audience}` : ''}`),
  createOpp: (body: Partial<Opportunity>) => j<Opportunity>('/api/v1/mvp/opportunities', { method: 'POST', body: JSON.stringify(body) }),
  ackOpp: (id: string) => j<Opportunity>(`/api/v1/mvp/opportunities/${id}/ack`, { method: 'POST', body: '{}' }),
  listSourcing: (opportunityId = '') =>
    j<{ items: Supplier[] }>(`/api/v1/mvp/sourcing${opportunityId ? `?opportunity_id=${encodeURIComponent(opportunityId)}` : ''}`),
  createSupplier: (body: Partial<Supplier>) => j<Supplier>('/api/v1/mvp/sourcing', { method: 'POST', body: JSON.stringify(body) }),
};
