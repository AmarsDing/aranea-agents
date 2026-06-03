import { kratosApi } from '../../services';
import type { Industry, Department, Position, PositionPromptResult, VariantInfo } from './types';

const BASE = '/v1/industries';

export async function listIndustries(): Promise<{ items: Industry[]; total: number }> {
  const { data } = await kratosApi.get(`${BASE}`);
  return data;
}

export async function getIndustry(key: string): Promise<Industry> {
  const { data } = await kratosApi.get(`${BASE}/${key}`);
  return data;
}

export async function listDepartments(industryKey: string): Promise<{ items: Department[]; total: number }> {
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/departments`);
  return data;
}

export async function listPositions(
  industryKey: string,
  departmentKey?: string,
): Promise<{ items: Position[]; total: number }> {
  const params: Record<string, string> = {};
  if (departmentKey) params.department_key = departmentKey;
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/positions`, { params });
  return data;
}

export async function getPositionPrompt(
  industryKey: string,
  positionKey: string,
  variant?: string,
): Promise<PositionPromptResult> {
  const params: Record<string, string> = { variant: variant || 'general' };
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/positions/${positionKey}/prompt`, { params });
  return {
    promptContent: data.prompt_content ?? data.promptContent ?? '',
    variant: data.variant ?? 'general',
    positionName: data.position_name ?? data.positionName ?? '',
    departmentName: data.department_name ?? data.departmentName ?? '',
    industryName: data.industry_name ?? data.industryName ?? '',
    industryDescription: data.industry_description ?? data.industryDescription ?? '',
    departmentDescription: data.department_description ?? data.departmentDescription ?? '',
    positionDescription: data.position_description ?? data.positionDescription ?? '',
    responsibilitiesJson: data.responsibilities_json ?? data.responsibilitiesJson ?? '',
    variantDescription: data.variant_description ?? data.variantDescription ?? '',
  };
}

export async function listPositionVariants(industryKey: string, positionKey: string): Promise<VariantInfo[]> {
  const { data } = await kratosApi.get(`${BASE}/${industryKey}/positions/${positionKey}/variants`);
  return (data.variants ?? []).map((v: VariantInfo) => ({
    key: v.key ?? '',
    label: v.label ?? v.key ?? '',
  }));
}
