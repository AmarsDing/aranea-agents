import { createUsageService } from '../../services/index';
import { kratosApi } from '../../services/axiosHandler';
import type { BudgetAlert } from './types';

const usage = createUsageService();

export type UsageQuota = {
  id?: string;
  scope_type: string;
  scope_id: string;
  monthly_micro_usd: number;
  period_start?: string;
  period_end?: string;
  created_at?: string;
  updated_at?: string;
};

export type UsageQuotaCheck = {
  allowed: boolean;
  quota?: UsageQuota;
  spent_micro_usd: number;
  remaining_micro_usd: number;
  reason: string;
};

function quotaFromUnknown(raw: unknown): UsageQuota {
  const o = raw !== null && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  return {
    id: String(o.id ?? ''),
    scope_type: String(o.scope_type ?? o.scopeType ?? ''),
    scope_id: String(o.scope_id ?? o.scopeId ?? ''),
    monthly_micro_usd: Number(o.monthly_micro_usd ?? o.monthlyMicroUsd ?? 0),
    period_start: String(o.period_start ?? o.periodStart ?? ''),
    period_end: String(o.period_end ?? o.periodEnd ?? ''),
    created_at: String(o.created_at ?? o.createdAt ?? ''),
    updated_at: String(o.updated_at ?? o.updatedAt ?? ''),
  };
}

export async function setUsageQuota(
  scopeType: string,
  scopeId: string,
  body: { monthly_micro_usd: number; period_start?: string; period_end?: string },
): Promise<UsageQuota> {
  const { data } = await kratosApi.put<unknown>(
    `/v1/usage/quotas/${encodeURIComponent(scopeType)}/${encodeURIComponent(scopeId)}`,
    {
      scope_type: scopeType,
      scope_id: scopeId,
      monthly_micro_usd: body.monthly_micro_usd,
      period_start: body.period_start ?? '',
      period_end: body.period_end ?? '',
    },
  );
  return quotaFromUnknown(data);
}

export async function checkUsageQuota(scopeType: string, scopeId: string): Promise<UsageQuotaCheck> {
  const { data } = await kratosApi.get<unknown>(
    `/v1/usage/quotas/${encodeURIComponent(scopeType)}/${encodeURIComponent(scopeId)}/check`,
  );
  const o = data !== null && typeof data === 'object' ? (data as Record<string, unknown>) : {};
  return {
    allowed: Boolean(o.allowed),
    quota: o.quota ? quotaFromUnknown(o.quota) : undefined,
    spent_micro_usd: Number(o.spent_micro_usd ?? o.spentMicroUsd ?? 0),
    remaining_micro_usd: Number(o.remaining_micro_usd ?? o.remainingMicroUsd ?? 0),
    reason: String(o.reason ?? ''),
  };
}

/** micro-USD → USD display */
export function microUsdToUsd(micro: number): string {
  return (micro / 1_000_000).toFixed(2);
}

function budgetAlertFromUnknown(raw: unknown): BudgetAlert {
  const o = raw !== null && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  return {
    id: String(o.id ?? ''),
    scope_type: String(o.scope_type ?? o.scopeType ?? ''),
    scope_id: String(o.scope_id ?? o.scopeId ?? ''),
    alert_ratio: Number(o.alert_ratio ?? o.alertRatio ?? 0),
    enabled: Boolean(o.enabled ?? true),
    last_fired_at: String(o.last_fired_at ?? o.lastFiredAt ?? ''),
    created_at: String(o.created_at ?? o.createdAt ?? ''),
    updated_at: String(o.updated_at ?? o.updatedAt ?? ''),
  };
}

export async function listBudgetAlerts(scopeType: string, scopeId: string): Promise<BudgetAlert[]> {
  const raw = await usage.ListBudgetAlerts({ scopeType, scopeId });
  const items = (raw as { items?: unknown[] }).items ?? [];
  return items.map(budgetAlertFromUnknown);
}

export async function setBudgetAlert(
  scopeType: string,
  scopeId: string,
  body: { alert_ratio: number; enabled: boolean },
): Promise<BudgetAlert> {
  const raw = await usage.SetBudgetAlert({
    scopeType,
    scopeId,
    alertRatio: body.alert_ratio,
    enabled: body.enabled,
  });
  return budgetAlertFromUnknown(raw);
}
