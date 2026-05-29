export function formatElapsed(startTime?: string, endTime?: string): string {
  if (!startTime) return "";
  const start = new Date(startTime).getTime();
  const end = endTime ? new Date(endTime).getTime() : Date.now();
  const diffMs = Math.max(0, end - start);
  return formatDurationMs(diffMs);
}

export function formatDurationMs(ms: number): string {
  if (ms < 1000) return "<1s";
  const seconds = Math.floor(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainSeconds = seconds % 60;
  if (minutes < 60) return `${minutes}m ${remainSeconds}s`;
  const hours = Math.floor(minutes / 60);
  const remainMinutes = minutes % 60;
  return `${hours}h ${remainMinutes}m`;
}

export function phaseLabel(phase?: string): string {
  switch (phase) {
    case "interactive": return "Interactive";
    case "escalating": return "Escalating";
    case "durable": return "Durable";
    case "completed": return "Completed";
    case "failed": return "Failed";
    default: return phase ?? "";
  }
}
