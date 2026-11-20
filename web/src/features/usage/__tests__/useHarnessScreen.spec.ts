import { describe, expect, it } from 'vitest';
import {
  consumedIn,
  creditRows,
  localHourLabel,
  monthEndForecast,
  rangeStartUTC,
  sameTimeYesterday,
  shareRows,
  toolRows,
  usedTone,
} from '../useHarnessScreen';
import type { PlatformCredit } from '../../fleet/api';
import type { ModelUsageBreakdownRow } from '../types';
import type { Tool } from '../../tools/types';

const t = (key: string, named?: Record<string, unknown>) => (named ? `${key}${JSON.stringify(named)}` : key);

function row(name: string, cost: number, tokens: number, calls = 1): ModelUsageBreakdownRow {
  return {
    provider_code: 'p',
    model_api_id: name,
    model_display_name: name,
    agent_id: '',
    agent_key: '',
    call_count: calls,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: tokens,
    total_cost_micro_usd: cost,
    avg_latency_ms: 0,
    avg_tokens_per_second: 0,
    success_rate: 1,
  };
}

describe('大屏显示 · 模型消耗', () => {
  it('按费用排前 5，其余合成一行，占比加起来是 1', () => {
    const src = [1, 2, 3, 4, 5, 6, 7].map((i) => row(`m${i}`, i * 1_000_000, 10));
    const { rows, byCost } = shareRows(src, (r) => r.model_display_name, t);
    expect(byCost).toBe(true);
    expect(rows.map((r) => r.label).slice(0, 5)).toEqual(['m7', 'm6', 'm5', 'm4', 'm3']);
    expect(rows[5].label).toBe('harnessScreen.othersRest');
    expect(rows[5].cost).toBe('$3.00');
    expect(rows.reduce((a, r) => a + r.share, 0)).toBeCloseTo(1);
  });

  it('接口只返回前 8 个时，占比按整段合计算，「其余」包括没返回的部分', () => {
    // 返回的 6 行共 $21，整段实际花了 $100
    const src = [1, 2, 3, 4, 5, 6].map((i) => row(`m${i}`, i * 1_000_000, 10));
    const total = { total_cost_micro_usd: 100_000_000, total_tokens: 1000, call_count: 50 };
    const { rows } = shareRows(src, (r) => r.model_display_name, t, total);
    expect(rows[0].share).toBeCloseTo(0.06);
    expect(rows[5].cost).toBe('$80.00');
    expect(rows[5].calls).toBe('45');
    expect(rows.reduce((a, r) => a + r.share, 0)).toBeCloseTo(1);
  });

  it('全部没定价时按 Token 算占比，并标出未定价', () => {
    const { rows, byCost } = shareRows([row('a', 0, 300), row('b', 0, 100)], (r) => r.model_display_name, t);
    expect(byCost).toBe(false);
    expect(rows[0].share).toBeCloseTo(0.75);
    expect(rows[0].unpriced).toBe(true);
  });

  it('没有数据时是空的', () => {
    expect(shareRows([], (r) => r.model_display_name, t).rows).toEqual([]);
  });

  it('只有调用次数、没有 Token 和费用的行不画（否则空行会占满一圈）', () => {
    expect(shareRows([row('', 0, 0, 3)], (r) => r.model_display_name, t).rows).toEqual([]);
  });
});

describe('大屏显示 · 周期与预估', () => {
  const now = new Date(Date.UTC(2026, 10, 20, 15, 30));

  it('周期起点与后端一致：都从 UTC 零点算', () => {
    expect(rangeStartUTC('today', now).toISOString()).toBe('2026-11-20T00:00:00.000Z');
    expect(rangeStartUTC('7d', now).toISOString()).toBe('2026-11-14T00:00:00.000Z');
    expect(rangeStartUTC('30d', now).toISOString()).toBe('2026-10-22T00:00:00.000Z');
    expect(rangeStartUTC('month', now).toISOString()).toBe('2026-11-01T00:00:00.000Z');
  });

  it('月底预估按已过天数线性外推；月初不到 3 天不预估', () => {
    // 11 月 30 天，已过 19.6458 天，花了 $196.458 → 约 $300
    expect(monthEndForecast(196_458_333, now)! / 1e6).toBeCloseTo(300, 0);
    expect(monthEndForecast(5_000_000, new Date(Date.UTC(2026, 10, 2, 12)))).toBeNull();
  });
});

describe('大屏显示 · 时间口径', () => {
  const pt = (key: string, calls: number) => ({
    date_key: key,
    call_count: calls,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: calls * 10,
    total_cost_micro_usd: calls * 1_000_000,
    success_count: 0,
    failed_count: 0,
    cancelled_count: 0,
    avg_latency_ms: 0,
    avg_tokens_per_second: 0,
  });

  it('昨日同时段：已过的小时全算，当前小时按分钟折算，之后的不算', () => {
    const points = [pt('2026-11-18T00', 10), pt('2026-11-18T01', 10), pt('2026-11-18T02', 10), pt('2026-11-18T05', 99)];
    // 现在是 UTC 02:30
    const r = sameTimeYesterday(points, new Date(Date.UTC(2026, 10, 19, 2, 30)));
    expect(r.calls).toBeCloseTo(25);
    expect(r.cost).toBeCloseTo(25_000_000);
  });

  it('UTC 小时键换成本地小时', () => {
    const expected = `${String(new Date('2026-11-19T08:00:00Z').getHours()).padStart(2, '0')}:00`;
    expect(localHourLabel('2026-11-19T08')).toBe(expected);
    expect(localHourLabel('11-19')).toBe('11-19');
  });
});

describe('大屏显示 · 工具消耗', () => {
  it('条长按第一名算，各段按这个工具的调用数算', () => {
    const tool = (key: string, calls: number, ok: number, fail: number) =>
      ({
        key,
        display_name: key,
        invoke_count: calls,
        success_count: ok,
        failure_count: fail,
        blocked_count: 0,
      }) as Tool;
    const rows = toolRows([tool('a', 200, 190, 10), tool('b', 50, 50, 0)]);
    expect(rows[0].ratio).toBe(1);
    expect(rows[1].ratio).toBe(0.25);
    expect(rows[0].okPart).toBeCloseTo(0.95);
    expect(rows[0].failRate).toBe('5.0%');
  });
});

describe('大屏显示 · 积分与额度', () => {
  const credit = (p: Partial<PlatformCredit> & { platform: string }): PlatformCredit => ({
    source: 'auto',
    plan: '',
    total: -1,
    remaining: -1,
    unit: 'credits',
    unlimited: false,
    updatedAt: '2026-11-19T10:00:00Z',
    note: '',
    history: [],
    ...p,
  });

  it('近 7 天消耗：只算下降，充值（上涨）不算；不到 2 个点算不出', () => {
    const now = new Date(Date.UTC(2026, 10, 20, 12));
    const h = [
      { date: '2026-11-10', remaining: 999 }, // 窗口外
      { date: '2026-11-13', remaining: 500 }, // 窗口起点（7 天前）
      { date: '2026-11-15', remaining: 420 },
      { date: '2026-11-17', remaining: 600 }, // 充值
      { date: '2026-11-20', remaining: 550 },
    ];
    expect(consumedIn(h, 7, now)).toBe(130);
    expect(consumedIn([{ date: '2026-11-20', remaining: 5 }], 7, now)).toBeNull();
  });

  it('手填超过 7 天没更新标「可能过期」，并显示近 7 天用量', () => {
    const now = new Date(Date.UTC(2026, 10, 20, 12));
    const [old] = creditRows(
      null,
      [credit({ platform: 'cursor', source: 'manual', total: 500, remaining: 100, updatedAt: '2026-11-10T00:00:00Z' })],
      null,
      t,
      now,
    );
    expect(old.stale).toBe(true);
    expect(old.sub).toContain('harnessScreen.credits.stale');
    const [fresh] = creditRows(
      null,
      [
        credit({
          platform: 'deepseek',
          remaining: 80,
          unit: 'CNY',
          updatedAt: '2026-11-20T11:00:00Z',
          history: [
            { date: '2026-11-19', remaining: 100 },
            { date: '2026-11-20', remaining: 80 },
          ],
        }),
      ],
      null,
      t,
      now,
    );
    expect(fresh.stale).toBe(false);
    expect(fresh.sub).toContain('harnessScreen.credits.used7d{"value":"¥20"}');
    expect(fresh.spark).toEqual([100, 80]);
  });

  it('预算在第一行；有总额才画已用条；终端已知百分比挂到对应平台', () => {
    const rows = creditRows(
      {
        configured_count: 2,
        total_cap_micro_usd: 100_000_000,
        total_spent_micro_usd: 75_000_000,
        max_utilization_ratio: 0.9,
      },
      [
        credit({ platform: 'deepseek', remaining: 110.5, unit: 'CNY' }),
        credit({ platform: 'cursor', source: 'manual', total: 500, remaining: 50, plan: 'Pro' }),
      ],
      [
        {
          agent: 'cursor',
          source: 'log',
          sessions: 3,
          windows: [
            { label: '5h', tokens: 1200, percent: 92, resetsAt: '' },
            { label: '7d', tokens: 5000, percent: -1, resetsAt: '' },
          ],
        },
      ],
      t,
    );
    expect(rows.map((r) => r.key)).toEqual(['budget', 'deepseek', 'cursor']);
    expect(rows[0].used).toBeCloseTo(0.75);
    expect(rows[0].tone).toBe('warn');
    expect(rows[1].value).toBe('¥110.5');
    expect(rows[1].used).toBeNull();
    expect(rows[1].editable).toBe(false);
    expect(rows[2].used).toBeCloseTo(0.9);
    expect(rows[2].tone).toBe('danger');
    expect(rows[2].editable).toBe(true);
    // 百分比未知的窗口不显示
    expect(rows[2].windows).toHaveLength(1);
  });

  it('手填平台没登记过写「未登记」，不当成 0', () => {
    const [row] = creditRows(null, [credit({ platform: 'qoder', source: 'manual', updatedAt: '' })], null, t);
    expect(row.value).toBe('harnessScreen.credits.unset');
    expect(row.missing).toBe(true);
    expect(row.used).toBeNull();
  });

  it('自动平台取不到时写「—」并带上原因', () => {
    const [row] = creditRows(
      null,
      [credit({ platform: 'deepseek', note: '后端没有配置 DEEPSEEK_API_KEY', updatedAt: '' })],
      null,
      t,
    );
    expect(row.value).toBe('—');
    expect(row.sub).toContain('DEEPSEEK_API_KEY');
  });

  it('没配置预算、拿不到积分时没有任何一行', () => {
    expect(creditRows(null, null, null, t)).toEqual([]);
  });

  it('70% 琥珀、90% 红', () => {
    expect([10, 70, 89, 90].map(usedTone)).toEqual(['ok', 'warn', 'warn', 'danger']);
  });
});
