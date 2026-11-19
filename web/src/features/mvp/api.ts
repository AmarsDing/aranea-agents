export type ContentItem = {
  ID: string;
  Title: string;
  Body: string;
  SourceType: string;
  SourceURL: string;
  Status: string;
};
export type ReviewTask = {
  ID: string;
  ContentItemID: string;
  DraftTitle: string;
  DraftBody: string;
  Status: string;
  MachineFlags?: string[];
  DecisionNote?: string;
};
export type PublishJob = {
  ID: string;
  Status: string;
  Platform: string;
  Title: string;
  PackageDir?: string;
  Mode: string;
  ArtifactID?: string;
};
export type Opportunity = {
  ID: string;
  Title: string;
  Summary: string;
  Audience: string;
  Status: string;
  Score: number;
};
export type Supplier = {
  ID: string;
  OpportunityID: string;
  Name: string;
  URL: string;
  PriceRange: string;
  Status: string;
  Notes: string;
};
export type IngestResult = {
  source: string;
  fetched: number;
  created: number;
  deduped: number;
  feed_url?: string;
  query?: string;
  errors?: string[];
};

import { createReviewService } from '../../services';

// ——— E 阶段 W1b（需求文档 §5.3 拍板 A-2）：review/opportunity 的 list/decide/ack
// 五端点已收编进 review.v1 proto 服务（/v1/review/*，RBAC review.view/review.decide，
// reviewer 服务端取 JWT、禁请求体自报、自批 400、重复决议 409）。以下适配层把新
// camelCase 视图映回旧 PascalCase 形状，stores/pages/useYouQueue 零改动；
// enqueue/createOpp 等保留的裸路由仍走 j()。———
function viewToReviewTask(v: Record<string, unknown>): ReviewTask {
  let flags: string[] = [];
  const raw = String(v.machineFlagsJson ?? '');
  if (raw) {
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) flags = parsed.map(String);
    } catch {
      flags = [];
    }
  }
  return {
    ID: String(v.id ?? ''),
    ContentItemID: String(v.contentItemId ?? ''),
    DraftTitle: String(v.draftTitle ?? ''),
    DraftBody: String(v.draftBody ?? ''),
    Status: String(v.status ?? ''),
    MachineFlags: flags,
    DecisionNote: String(v.decisionNote ?? ''),
  };
}

function viewToOpportunity(v: Record<string, unknown>): Opportunity {
  return {
    ID: String(v.id ?? ''),
    Title: String(v.title ?? ''),
    Summary: String(v.summary ?? ''),
    // review.v1 OpportunityView 以 source 承载 audience（proto 一期无 score 列，
    // 评分展示回归已登记 W4 评审）。
    Audience: String(v.source ?? ''),
    Status: String(v.status ?? ''),
    Score: 0,
  };
}

async function j<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init?.headers || {}) },
    ...init,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    // 保留 HTTP 状态与响应体，供 isForbiddenApiError 判定 403（message 文案保持不变）。
    const err = new Error((data as any).error || res.statusText) as Error & {
      status?: number;
      response?: { status: number; data: unknown };
    };
    err.status = res.status;
    err.response = { status: res.status, data };
    throw err;
  }
  return data as T;
}

export { packageFileURL } from './links';

export const mvpApi = {
  listContent: (q = '') => j<{ items: ContentItem[]; total: number }>(`/api/v1/mvp/content${q}`),
  createContent: (body: Partial<ContentItem>) =>
    j<{ item: ContentItem }>('/api/v1/mvp/content', { method: 'POST', body: JSON.stringify(body) }),
  ingestRSS: (feed_url: string, workspace_id = '') =>
    j<IngestResult>('/api/v1/mvp/ingest/rss', { method: 'POST', body: JSON.stringify({ feed_url, workspace_id }) }),
  ingestSearch: (query: string, hits: { URL: string; Title: string; Snippet: string }[], workspace_id = '') =>
    j<IngestResult>('/api/v1/mvp/ingest/search', {
      method: 'POST',
      body: JSON.stringify({ query, hits, workspace_id }),
    }),
  listReview: async () => {
    const res = await createReviewService().ListPendingReviews({ page: 1, pageSize: 200 });
    return {
      items: (res.items ?? []).map((r) => viewToReviewTask(r as Record<string, unknown>)),
      total: Number(res.total ?? 0),
    };
  },
  enqueueReview: (body: Partial<ReviewTask>) =>
    j<ReviewTask>('/api/v1/mvp/review', { method: 'POST', body: JSON.stringify(body) }),
  approve: async (id: string, note = 'ok') => {
    const view = await createReviewService().ApproveReview({ id, note });
    // 收编后发布单由服务端联动创建并立即处理（CreateFromApprovedReview +
    // ProcessNext），响应不再携带 publish_job——产物包请走发布作业列表。
    return { review: viewToReviewTask(view as Record<string, unknown>), publish_job: null as PublishJob | null };
  },
  reject: async (id: string, note: string) => {
    const view = await createReviewService().RejectReview({ id, note });
    return viewToReviewTask(view as Record<string, unknown>);
  },
  listPublish: () => j<{ items: PublishJob[] }>('/api/v1/mvp/publish/jobs?status=packaged'),
  listOpp: async (audience = '') => {
    const res = await createReviewService().ListOpportunities({ page: 1, pageSize: 200 });
    let items = (res.items ?? []).map((o) => viewToOpportunity(o as Record<string, unknown>));
    // review.v1 一期无 audience 过滤参数 → 客户端过滤（数据量小，200 条窗内）。
    if (audience) items = items.filter((o) => o.Audience === audience);
    return { items };
  },
  createOpp: (body: Partial<Opportunity>) =>
    j<Opportunity>('/api/v1/mvp/opportunities', { method: 'POST', body: JSON.stringify(body) }),
  ackOpp: async (id: string) =>
    viewToOpportunity((await createReviewService().AckOpportunity({ id })) as Record<string, unknown>),
  listSourcing: (opportunityId = '') =>
    j<{ items: Supplier[] }>(
      `/api/v1/mvp/sourcing${opportunityId ? `?opportunity_id=${encodeURIComponent(opportunityId)}` : ''}`,
    ),
  createSupplier: (body: Partial<Supplier>) =>
    j<Supplier>('/api/v1/mvp/sourcing', { method: 'POST', body: JSON.stringify(body) }),
};
