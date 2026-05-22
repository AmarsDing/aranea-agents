import type { AgentNodeStatus, DisplayStatus } from "./types";

/** Fine-grained status labels for Kanban subtitle and Graph node hint. */
export const AGENT_NODE_STATUS_STYLES: Record<
  AgentNodeStatus,
  { label: string; icon: string }
> = {
  idle: { label: "等待", icon: "radio_button_unchecked" },
  queued: { label: "排队中", icon: "hourglass_empty" },
  scheduled: { label: "已调度", icon: "schedule" },
  running: { label: "运行中", icon: "sync" },
  thinking: { label: "推理中", icon: "psychology" },
  tool_running: { label: "工具执行", icon: "build" },
  transferring: { label: "切换中", icon: "swap_horiz" },
  retrying: { label: "重试中", icon: "replay" },
  waiting_input: { label: "等待输入", icon: "pause_circle" },
  waiting_review: { label: "等待审核", icon: "rate_review" },
  waiting_assign: { label: "等待认领", icon: "assignment_ind" },
  blocked: { label: "已阻塞", icon: "block" },
  success: { label: "完成", icon: "check_circle" },
  failed: { label: "失败", icon: "error" },
  skipped: { label: "已跳过", icon: "skip_next" },
  cancelled: { label: "已取消", icon: "cancel" },
  timed_out: { label: "超时", icon: "timer_off" },
};

/** Aggregated display status for chips and Graph node border. */
export const DISPLAY_STATUS_STYLES: Record<
  DisplayStatus,
  { label: string; icon: string; color: string }
> = {
  waiting: { label: "等待", icon: "schedule", color: "grey" },
  active: { label: "运行中", icon: "sync", color: "primary" },
  suspended: { label: "挂起", icon: "pause_circle", color: "warning" },
  success: { label: "完成", icon: "check_circle", color: "positive" },
  failed: { label: "失败", icon: "error", color: "negative" },
  skipped: { label: "跳过", icon: "skip_next", color: "grey-6" },
  cancelled: { label: "取消", icon: "cancel", color: "grey" },
};

export const ORCHESTRATION_STATUS_ENVELOPE = "orchestration_agent_status" as const;

export const WORK_PHASE_LABELS: Record<string, string> = {
  received: "收到",
  doing: "进行中",
  delivered: "已交付",
};
