import { describe, expect, it } from 'vitest';
import {
  buildCronTaskPayload,
  canSaveCronForm,
  cronTargetKind,
  describeCronExpression,
  describeCronSchedule,
  describeFleetSchedule,
  describeInterval,
  emptyCronTaskForm,
  formatCronShort,
} from '../../../components/cron/cronTaskUtils';
import type { CronTaskRow } from '../types';

describe('describeCronExpression', () => {
  it.each([
    ['0 3 * * *', '每天 03:00'],
    ['30 8 * * 1-5', '工作日 08:30'],
    ['0 18 * * 5', '每周五 18:00'],
    ['0 9 * * 1,3,5', '每周一、三、五 09:00'],
    ['0 4 1 * *', '每月 1 日 04:00'],
    ['0 * * * *', '每小时整点'],
    ['15 * * * *', '每小时第 15 分'],
    ['*/10 * * * *', '每 10 分钟'],
    ['0 */2 * * *', '每 2 小时整点'],
  ])('%s → %s', (expr, want) => {
    expect(describeCronExpression(expr)).toBe(want);
  });

  it('returns empty for patterns it cannot phrase', () => {
    expect(describeCronExpression('0 3 * 6 *')).toBe('');
    expect(describeCronExpression('bad')).toBe('');
  });
});

describe('describeCronSchedule / describeInterval', () => {
  it('phrases intervals', () => {
    expect(describeInterval(900)).toBe('每 15 分钟');
    expect(describeInterval(7200)).toBe('每 2 小时');
    expect(describeInterval(86400)).toBe('每天');
    expect(describeInterval(0)).toBe('每 15 分钟');
  });
  it('falls back to the raw expression', () => {
    expect(describeCronSchedule({ schedule_type: 'cron', cron_expression: '0 3 * 6 *' })).toBe('cron 0 3 * 6 *');
  });
});

describe('describeFleetSchedule', () => {
  it.each([
    ['every 6h', '每 6 小时'],
    ['every 30m', '每 30 分钟'],
    ['daily 02:00', '每天 02:00'],
    ['weekdays 09:30', '工作日 09:30'],
    ['weekly mon 09:00', '每周一 09:00'],
    ['hourly', '每小时'],
  ])('%s → %s', (raw, want) => {
    expect(describeFleetSchedule(raw)).toBe(want);
  });
});

describe('formatCronShort', () => {
  it('drops the year within the same year and handles empty / zero values', () => {
    const now = new Date(2026, 10, 19);
    expect(formatCronShort(new Date(2026, 10, 20, 3, 0).toISOString(), now)).toBe('11-20 03:00');
    expect(formatCronShort(new Date(2027, 0, 2, 9, 5).toISOString(), now)).toBe('2027-01-02 09:05');
    expect(formatCronShort('', now)).toBe('—');
    expect(formatCronShort('0001-01-01T00:00:00Z', now)).toBe('—');
  });
});

describe('system targets in the form', () => {
  const systemRow = {
    id: 'cron_model_registry_sync',
    key: 'model-registry-sync',
    name: 'Model Registry Sync',
    agent_id: '',
    enabled: true,
    sort_order: 0,
    config_json: '{"target_type":"model_registry_sync","message":"sync model registry","interval_seconds":3600}',
    metadata_json: '{"run_count":10,"next_run_at":"2026-11-20T00:00:00Z"}',
  } as unknown as CronTaskRow;

  it('keeps target kind and is savable', () => {
    expect(cronTargetKind({ target_type: 'model_registry_sync' })).toBe('model_registry_sync');
    expect(cronTargetKind({ team_id: 't1' })).toBe('team');
    const form = emptyCronTaskForm();
    form.target_type = 'model_registry_sync';
    form.message = 'sync model registry';
    expect(canSaveCronForm(form, true)).toBe(true);
  });

  it('does not resend metadata on edit (avoids overwriting runner counters)', () => {
    const form = emptyCronTaskForm();
    form.name = 'model-registry-sync';
    form.target_type = 'model_registry_sync';
    form.message = 'sync model registry';
    form.interval_minutes = 60;
    const payload = buildCronTaskPayload(form, systemRow);
    expect(payload.metadata_json).toBeUndefined();
    expect(JSON.parse(payload.config_json ?? '{}').target_type).toBe('model_registry_sync');
    expect(buildCronTaskPayload(form, null).metadata_json).toBe('{}');
  });
});

describe('applyCronRowToForm timezone', () => {
  it('keeps UTC for stored tasks without a timezone (what the runner uses)', async () => {
    const { applyCronRowToForm } = await import('../../../components/cron/cronTaskUtils');
    const form = emptyCronTaskForm();
    applyCronRowToForm(
      {
        key: 'k',
        name: 'n',
        config_json: '{"schedule_type":"cron","cron_expression":"0 3 * * *"}',
      } as unknown as CronTaskRow,
      form,
    );
    expect(form.timezone).toBe('UTC');
  });
});
