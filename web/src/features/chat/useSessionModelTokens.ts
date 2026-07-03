import { ref, watch, type Ref } from 'vue';
import { listSessionTurns } from '../session/api';
import type { SessionTurn } from '../session/types';

export type ModelTokenPoint = {
  turn: number;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
};

export type ModelTokenSeries = {
  /** Display label, e.g. "gpt-4o" or "openai/gpt-4o". */
  label: string;
  /** Provider + model key for series identity. */
  key: string;
  points: ModelTokenPoint[];
  totalIn: number;
  totalOut: number;
  totalAll: number;
};

export type SessionModelTokens = {
  series: ModelTokenSeries[];
  totalTurns: number;
  /** All turn numbers on the x-axis. */
  turns: number[];
};

/**
 * Fetch all SessionTurn records for a session and aggregate token usage per model.
 *
 * Note: SessionTurn only records the `final_provider`/`final_model` per turn — if a
 * single turn used multiple models, all tokens are attributed to the final model.
 * So this is an approximation, not a true per-model breakdown.
 */
export function useSessionModelTokens(sessionId: Ref<string | undefined | null>) {
  const data = ref<SessionModelTokens>({ series: [], totalTurns: 0, turns: [] });
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function load() {
    const id = sessionId.value;
    if (!id) {
      data.value = { series: [], totalTurns: 0, turns: [] };
      return;
    }
    loading.value = true;
    error.value = null;
    try {
      // Fetch up to 200 turns — enough for typical sessions while keeping payload small.
      const { items, total } = await listSessionTurns(id, 200, 0);
      data.value = aggregateByModel(items, total);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      data.value = { series: [], totalTurns: 0, turns: [] };
    } finally {
      loading.value = false;
    }
  }

  watch(sessionId, load, { immediate: true });

  return { data, loading, error, reload: load };
}

function aggregateByModel(turns: SessionTurn[], total: number): SessionModelTokens {
  if (!turns.length) return { series: [], totalTurns: total, turns: [] };

  // Sort by turn_number ascending so the x-axis is chronological.
  const sorted = [...turns].sort((a, b) => a.turn_number - b.turn_number);
  const turnsAxis = sorted.map((t) => t.turn_number);

  const byModel = new Map<string, ModelTokenSeries>();
  for (const t of sorted) {
    const provider = (t.final_provider || '').trim();
    const model = (t.final_model || '').trim();
    // Skip turns without a model (e.g., failed before model call).
    if (!model && !provider) continue;
    const key = provider && model ? `${provider}/${model}` : model || provider || 'unknown';
    const label = model || provider || 'unknown';

    let series = byModel.get(key);
    if (!series) {
      series = {
        key,
        label,
        points: [],
        totalIn: 0,
        totalOut: 0,
        totalAll: 0,
      };
      byModel.set(key, series);
    }
    series.points.push({
      turn: t.turn_number,
      inputTokens: t.input_tokens ?? 0,
      outputTokens: t.output_tokens ?? 0,
      totalTokens: t.total_tokens ?? 0,
    });
    series.totalIn += t.input_tokens ?? 0;
    series.totalOut += t.output_tokens ?? 0;
    series.totalAll += t.total_tokens ?? 0;
  }

  // Sort series by total tokens desc so the biggest consumer draws first.
  const series = Array.from(byModel.values()).sort((a, b) => b.totalAll - a.totalAll);

  return { series, totalTurns: total, turns: turnsAxis };
}
