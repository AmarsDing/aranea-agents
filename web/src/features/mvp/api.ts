export type ContentItem = {
  ID: string; Title: string; Body: string; SourceType: string; SourceURL: string; Status: string;
};
export type ReviewTask = {
  ID: string; ContentItemID: string; DraftTitle: string; DraftBody: string; Status: string;
  MachineFlags?: string[]; DecisionNote?: string;
};
export type PublishJob = {
  ID: string; Status: string; Platform: string; Title: string; PackageDir?: string; Mode: string;
};
export type Opportunity = {
  ID: string; Title: string; Summary: string; Audience: string; Status: string; Score: number;
};
export type Supplier = {
  ID: string; OpportunityID: string; Name: string; URL: string; PriceRange: string; Status: string; Notes: string;
};

async function j<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data as T;
}

export const mvpApi = {
  listContent: (q = '') => j<{ items: ContentItem[]; total: number }>(`/api/v1/mvp/content${q}`),
  createContent: (body: Partial<ContentItem>) => j<{ item: ContentItem }>('/api/v1/mvp/content', { method: 'POST', body: JSON.stringify(body) }),
  listReview: () => j<{ items: ReviewTask[]; total: number }>('/api/v1/mvp/review/pending'),
  enqueueReview: (body: Partial<ReviewTask>) => j<ReviewTask>('/api/v1/mvp/review', { method: 'POST', body: JSON.stringify(body) }),
  approve: (id: string, note = 'ok') => j<{ review: ReviewTask; publish_job: PublishJob }>(`/api/v1/mvp/review/${id}/approve`, { method: 'POST', body: JSON.stringify({ reviewer_id: 'ops', note, platform: 'generic' }) }),
  reject: (id: string, note: string) => j<ReviewTask>(`/api/v1/mvp/review/${id}/reject`, { method: 'POST', body: JSON.stringify({ reviewer_id: 'ops', note }) }),
  listPublish: () => j<{ items: PublishJob[] }>('/api/v1/mvp/publish/jobs?status=packaged'),
  listOpp: (audience = '') => j<{ items: Opportunity[] }>(`/api/v1/mvp/opportunities${audience ? `?audience=${audience}` : ''}`),
  createOpp: (body: Partial<Opportunity>) => j<Opportunity>('/api/v1/mvp/opportunities', { method: 'POST', body: JSON.stringify(body) }),
  ackOpp: (id: string) => j<Opportunity>(`/api/v1/mvp/opportunities/${id}/ack`, { method: 'POST', body: '{}' }),
  listSourcing: () => j<{ items: Supplier[] }>('/api/v1/mvp/sourcing'),
  createSupplier: (body: Partial<Supplier>) => j<Supplier>('/api/v1/mvp/sourcing', { method: 'POST', body: JSON.stringify(body) }),
};
