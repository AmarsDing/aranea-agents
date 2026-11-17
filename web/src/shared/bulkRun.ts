/**
 * 批量操作条的通用执行器：后端没有「按 id 列表」的批量 RPC 时，
 * 前端逐项调用单条接口并聚合失败，保证批量条能拿到可展示的失败原因。
 */

export interface BulkRunFailure {
  id: string;
  message: string;
}

export interface BulkRunReport {
  succeeded: number;
  failures: BulkRunFailure[];
}

export async function runBulkSequential(
  ids: string[],
  run: (id: string) => Promise<unknown>,
): Promise<BulkRunReport> {
  let succeeded = 0;
  const failures: BulkRunFailure[] = [];
  for (const id of ids) {
    try {
      await run(id);
      succeeded += 1;
    } catch (error) {
      failures.push({ id, message: error instanceof Error ? error.message : String(error) });
    }
  }
  return { succeeded, failures };
}
