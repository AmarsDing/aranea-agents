import type { RunStatusValue } from "./types";
import type { ConversationSource, ConversationTurnStatus, DeliveryStatus } from "./conversation";
import { runStatusToTurnStatus } from "./conversation";

export type StatusPresentation = {
  label: string;
  tone: "neutral" | "info" | "warning" | "success" | "danger";
};

export function presentRunStatus(status: RunStatusValue | string): StatusPresentation {
  const turnStatus = runStatusToTurnStatus(status) ?? normalizeTurnStatus(status);
  return presentTurnStatus(turnStatus);
}

export function presentTurnStatus(status: ConversationTurnStatus | undefined): StatusPresentation {
  switch (status) {
    case "queued":
      return { label: "排队中", tone: "neutral" };
    case "running":
      return { label: "正在生成", tone: "info" };
    case "awaiting_user":
      return { label: "等你确认", tone: "warning" };
    case "background":
      return { label: "后台运行", tone: "info" };
    case "completed":
      return { label: "已完成", tone: "success" };
    case "failed":
      return { label: "失败可重试", tone: "danger" };
    case "cancelled":
      return { label: "已取消", tone: "neutral" };
    default:
      return { label: "待开始", tone: "neutral" };
  }
}

export function presentDeliveryStatus(status: DeliveryStatus | undefined): StatusPresentation {
  switch (status) {
    case "pending":
      return { label: "待发送", tone: "neutral" };
    case "sending":
      return { label: "发送中", tone: "info" };
    case "delivered":
      return { label: "已送达", tone: "success" };
    case "failed":
      return { label: "发送失败", tone: "danger" };
    case "skipped":
      return { label: "已跳过", tone: "neutral" };
    default:
      return { label: "无需外发", tone: "neutral" };
  }
}

export function presentConversationSource(source: ConversationSource | string | undefined): string {
  switch ((source ?? "").toLowerCase()) {
    case "channel":
      return "外部 Channel";
    case "cron":
      return "定时任务";
    case "a2a":
      return "A2A";
    case "durable":
      return "后台任务";
    case "ws":
    case "web":
      return "网页聊天";
    default:
      return "聊天";
  }
}

export function toneToQuasarColor(tone: StatusPresentation["tone"]): string {
  switch (tone) {
    case "info":
      return "primary";
    case "warning":
      return "warning";
    case "success":
      return "positive";
    case "danger":
      return "negative";
    default:
      return "grey-7";
  }
}

function normalizeTurnStatus(status: string): ConversationTurnStatus | undefined {
  switch (status) {
    case "queued":
    case "running":
    case "awaiting_user":
    case "background":
    case "completed":
    case "failed":
    case "cancelled":
      return status;
    default:
      return undefined;
  }
}
