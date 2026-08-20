import type { EvalCaseResult, EvalRun } from './types';

function escapeCsvCell(value: string): string {
  if (/[",\n\r]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

function downloadBlob(filename: string, mime: string, body: string): void {
  const blob = new Blob([body], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/** Export run summary + case results as CSV (EVAL-02 report export, client-side). */
export function exportEvalRunCsv(run: EvalRun, rows: EvalCaseResult[]): void {
  const header = [
    'run_id',
    'case_id',
    'input',
    'actual_output',
    'exact_match',
    'contains_match',
    'llm_judge_score',
    'tool_call_accuracy',
    'human_pass',
    'human_score',
    'human_comment',
    'error_message',
  ];
  const lines = [
    header.join(','),
    ...rows.map((r) =>
      [
        run.id,
        r.case_id,
        r.input ?? '',
        r.actual_output ?? '',
        String(r.exact_match),
        String(r.contains_match),
        String(r.llm_judge_score ?? ''),
        String(r.tool_call_accuracy ?? ''),
        r.human_pass == null ? '' : String(r.human_pass),
        r.human_score == null ? '' : String(r.human_score),
        r.human_comment ?? '',
        r.error_message ?? '',
      ]
        .map((c) => escapeCsvCell(String(c)))
        .join(','),
    ),
  ];
  downloadBlob(`eval-run-${run.id}.csv`, 'text/csv;charset=utf-8', lines.join('\n'));
}

/** Export full run report as JSON for tooling / AI review. */
export function exportEvalRunJson(run: EvalRun, rows: EvalCaseResult[]): void {
  const payload = {
    run,
    results: rows,
    exported_at: new Date().toISOString(),
  };
  downloadBlob(`eval-run-${run.id}.json`, 'application/json;charset=utf-8', JSON.stringify(payload, null, 2));
}
