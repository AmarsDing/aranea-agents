export function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

export type McpTestResult = { ok: boolean; message?: string; status?: string };

/**
 * stdio 测试仅校验命令可执行、未真实启动子进程握手。
 * 此类结果用 info（而非 positive 绿色对勾）展示，避免视觉上传达「已连通」的假阳性。
 */
export function mcpTestNotify(result: McpTestResult): { type: string; message: string } {
  const base = result.message || result.status || '';
  const isStdioProbeOnly = result.ok && /stdio/i.test(base);
  return {
    type: result.ok ? (isStdioProbeOnly ? 'info' : 'positive') : 'warning',
    message: base,
  };
}
