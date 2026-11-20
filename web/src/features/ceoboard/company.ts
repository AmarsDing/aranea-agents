// 电商经营快照拉取（GET /v1/ceoboard/company）。
// 该接口为只读 JSON BFF，不经过 proto 生成类型；失败时由页面 banner 呈现，
// 因此显式 skipErrorNotify，避免 15s 轮询触发全局 toast。
import { kratosApi } from '../../services/axiosHandler';
import type { CompanySnapshot } from './types';
import type { Board, PartialSource } from '../company/tree';

/** 看板总览「一张图」的后端聚合（GET /v1/ceoboard/org-graph）；字段同 company/tree 的 Board。 */
export async function fetchOrgGraph(): Promise<Board & { generatedAt: string; partial: PartialSource[] }> {
  const res = await kratosApi.get<Board & { generatedAt: string; partial: PartialSource[] }>('/v1/ceoboard/org-graph', {
    skipErrorNotify: true,
  });
  return res.data;
}

export async function fetchCompanySnapshot(): Promise<CompanySnapshot> {
  const res = await kratosApi.get<CompanySnapshot>('/v1/ceoboard/company', {
    skipErrorNotify: true,
  });
  return res.data;
}
