import { describe, expect, it } from 'vitest';
import { defaultAgentAdvancedSettings, defaultAgentRuntimeConfig } from '../agentRuntimeConfig';
import { buildAgentConfigJson } from '../agentRuntimeConfigSerialize';
import { hydrateAgentRuntime, hydrateRuntimeFromConfigJson } from '../agentRuntimeConfigHydrate';

describe('agent eval auto config (config_json.evaluation)', () => {
  it('defaults to disabled with sane values', () => {
    const config = defaultAgentRuntimeConfig();
    expect(config.evaluation.auto_after_turn).toBe(false);
    expect(config.evaluation.dataset_id).toBe('');
    expect(config.evaluation.metrics).toBe('');
    expect(config.evaluation.num_runs).toBe(1);
    expect(config.evaluation.min_interval_sec).toBe(300);
  });

  it('serializes the evaluation section into config_json', () => {
    const config = defaultAgentRuntimeConfig();
    config.evaluation.auto_after_turn = true;
    config.evaluation.dataset_id = 'ds-1';
    config.evaluation.metrics = 'exact_match';
    config.evaluation.num_runs = 3;
    config.evaluation.min_interval_sec = 60;
    const raw = JSON.parse(buildAgentConfigJson(config, [])) as Record<string, unknown>;
    expect(raw.evaluation).toEqual({
      auto_after_turn: true,
      dataset_id: 'ds-1',
      metrics: 'exact_match',
      num_runs: 3,
      min_interval_sec: 60,
    });
  });

  it('hydrates from config_json (legacy path)', () => {
    const config = defaultAgentRuntimeConfig();
    hydrateRuntimeFromConfigJson(
      config,
      JSON.stringify({
        evaluation: {
          auto_after_turn: true,
          dataset_id: 'ds-9',
          metrics: 'exact_match',
          num_runs: 2,
          min_interval_sec: 30,
        },
      }),
    );
    expect(config.evaluation.auto_after_turn).toBe(true);
    expect(config.evaluation.dataset_id).toBe('ds-9');
    expect(config.evaluation.metrics).toBe('exact_match');
    expect(config.evaluation.num_runs).toBe(2);
    expect(config.evaluation.min_interval_sec).toBe(30);
  });

  it('hydrates evaluation from config_json when settings is the primary source', () => {
    const config = defaultAgentRuntimeConfig();
    const advanced = defaultAgentAdvancedSettings();
    hydrateAgentRuntime(config, advanced, {
      id: 'a1',
      settings: {},
      config_json: JSON.stringify({
        evaluation: { auto_after_turn: true, dataset_id: 'ds-2', min_interval_sec: 120 },
      }),
    } as never);
    expect(config.evaluation.auto_after_turn).toBe(true);
    expect(config.evaluation.dataset_id).toBe('ds-2');
    expect(config.evaluation.min_interval_sec).toBe(120);
    // Unset keys keep defaults.
    expect(config.evaluation.num_runs).toBe(1);
  });

  it('keeps defaults when config_json has no evaluation section', () => {
    const config = defaultAgentRuntimeConfig();
    hydrateRuntimeFromConfigJson(config, JSON.stringify({ memory: { enabled: false } }));
    expect(config.evaluation.auto_after_turn).toBe(false);
    expect(config.evaluation.dataset_id).toBe('');
  });
});
