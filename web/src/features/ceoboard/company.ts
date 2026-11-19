// 电商经营快照拉取（GET /v1/ceoboard/company）。
// 该接口为只读 JSON BFF，不经过 proto 生成类型；失败时由页面 banner 呈现，
// 因此显式 skipErrorNotify，避免 15s 轮询触发全局 toast。
import { kratosApi } from '../../services/axiosHandler';
import type { CompanySnapshot } from './types';

export async function fetchCompanySnapshot(): Promise<CompanySnapshot> {
  const res = await kratosApi.get<CompanySnapshot>('/v1/ceoboard/company', {
    skipErrorNotify: true,
  });
  return res.data;
}
