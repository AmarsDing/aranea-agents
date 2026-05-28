import { computed, type ComputedRef } from "vue";
import { useI18n } from "vue-i18n";
import { isAvatarAssetRef } from "../avatar/iconModel";
import { reasoningMarkdown } from "./streamContentPatch";
import { MESSAGE_STATUS, isToolStatus } from "../../domain/types";
import type { ToolUseEvent } from "./types";
import type { Message } from "./types";

const AVATAR_PALETTE = [
  { name: "indigo", hex: "#5c6bc0" },
  { name: "cyan", hex: "#26c6da" },
  { name: "purple", hex: "#ab47bc" },
  { name: "teal", hex: "#26a69a" },
  { name: "deep-purple", hex: "#7e57c2" },
  { name: "deep-orange", hex: "#ff7043" },
  { name: "blue", hex: "#42a5f5" },
  { name: "pink", hex: "#ec407a" },
  { name: "green", hex: "#66bb6a" },
  { name: "amber", hex: "#ffa726" },
] as const;

type TeamMemberMessageMeta = {
  agent_id?: string;
  agent_key?: string;
  name?: string;
  role?: string;
  icon?: string;
};

type AgentMessageMeta = {
  agent_id?: string;
  agent_key?: string;
  name?: string;
  icon?: string;
};

export function useChatMessageRow(messages: ComputedRef<Message[]>) {
  const { t } = useI18n();

  function teamMemberMeta(message: Message): TeamMemberMessageMeta | null {
    if (message.team_member) {
      const tm = message.team_member;
      return { agent_id: tm.agent_id, name: tm.name, role: tm.role, icon: tm.icon };
    }
    try {
      const raw = JSON.parse(message.options_json || "{}") as {
        schema?: string;
        team_member?: TeamMemberMessageMeta;
        member_agent_key?: string;
        author?: string;
        display_name?: string;
      };
      if (raw.team_member) return raw.team_member;
      if (raw.member_agent_key) {
        return { agent_key: raw.member_agent_key, name: raw.display_name || raw.member_agent_key };
      }
      if (raw.schema === "chat.team_member/v1" && raw.author) {
        return { agent_key: raw.author, name: raw.author };
      }
      if (message.role === "member" && raw.author) {
        return { agent_key: raw.author, name: raw.author };
      }
      return null;
    } catch {
      return null;
    }
  }

  function agentMeta(message: Message): AgentMessageMeta | null {
    if (message.agent_ref) {
      const a = message.agent_ref;
      return { agent_id: a.id, agent_key: a.agent_key, name: a.name, icon: a.icon };
    }
    try {
      const raw = JSON.parse(message.options_json || "{}") as { agent?: AgentMessageMeta };
      return raw.agent ?? null;
    } catch {
      return null;
    }
  }

  function toolEventMeta(message: Message): ToolUseEvent | null {
    if (message.tool_event && typeof message.tool_event === "object") {
      return message.tool_event as ToolUseEvent;
    }
    try {
      const raw = JSON.parse(message.options_json || "{}") as { tool_event?: ToolUseEvent };
      return raw.tool_event ?? null;
    } catch {
      return null;
    }
  }

  function messageIdentityKey(message: Message): string {
    if (message.role === "user") return "user";
    const meta = teamMemberMeta(message) ?? agentMeta(message);
    if (meta?.agent_id) return meta.agent_id;
    if (meta?.agent_key) return meta.agent_key;
    if (message.model_name?.trim()) return message.model_name;
    return message.id || "assistant";
  }

  function displayMessageName(message: Message): string {
    if (message.role === "user") return t("chat.me");
    const member = teamMemberMeta(message);
    if (member?.name) {
      return member.role ? `${member.name} (${member.role})` : member.name;
    }
    const agent = agentMeta(message);
    if (agent?.name) return agent.name;
    if (message.model_name?.startsWith("team/")) {
      const [, role, name] = message.model_name.split("/");
      if (name && role) return `${name} (${role})`;
      if (name) return name;
    }
    return t("chat.assistant");
  }

  function isTeamMember(message: Message): boolean {
    return (
      message.role === "member" ||
      (message.role !== "user" && (Boolean(teamMemberMeta(message)) || Boolean(message.model_name?.startsWith("team/"))))
    );
  }

  function isStreaming(message: Message): boolean {
    return message.status === MESSAGE_STATUS.STREAMING || message.status === MESSAGE_STATUS.TOOL_RUNNING;
  }

  function isContinued(idx: number): boolean {
    if (idx <= 0) return false;
    const list = messages.value;
    const cur = list[idx];
    const prev = list[idx - 1];
    if (!cur || !prev) return false;
    if (prev.role !== cur.role) return false;
    return messageIdentityKey(prev) === messageIdentityKey(cur);
  }

  function paletteIndex(message: Message): number {
    const key = messageIdentityKey(message);
    let h = 0;
    for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
    return h % AVATAR_PALETTE.length;
  }

  function messageAvatarColor(message: Message): string {
    return AVATAR_PALETTE[paletteIndex(message)]?.name || "indigo";
  }

  function memberAccentHex(message: Message): string {
    return AVATAR_PALETTE[paletteIndex(message)]?.hex || "#5c6bc0";
  }

  function bubbleAccentStyle(message: Message): Record<string, string> | undefined {
    if (!isTeamMember(message)) return undefined;
    return { "--bubble-accent": memberAccentHex(message) };
  }

  function structuredToolEvent(message: Message): ToolUseEvent | null {
    const ev = toolEventMeta(message);
    if (!ev?.tool_name && !ev?.id) return null;
    return ev;
  }

  function isToolEventMessage(message: Message): boolean {
    return Boolean(toolEventMeta(message)) || message.status.startsWith("tool_");
  }

  function toolCollapseParts(message: Message): { summary: string; detail: string } {
    const md = message.content_markdown?.trim() ?? "";
    const nl = md.indexOf("\n");
    if (nl === -1) {
      return { summary: toolCollapsePlainLine(md), detail: "" };
    }
    const first = md.slice(0, nl).trim();
    const rest = md.slice(nl + 1).trim();
    return { summary: toolCollapsePlainLine(first), detail: rest };
  }

  function toolCollapsePlainLine(s: string): string {
    let value = s.replace(/\*\*/g, "").replace(/`/g, "").trim();
    if (value.length > 220) value = `${value.slice(0, 220)}…`;
    return value || "工具";
  }

  function toolCollapseSummary(message: Message): string {
    return toolCollapseParts(message).summary;
  }

  function toolCollapseDetail(message: Message): string {
    return toolCollapseParts(message).detail;
  }

  function isCollapsibleToolDetail(message: Message): boolean {
    if (!isToolEventMessage(message)) return false;
    if (message.status === MESSAGE_STATUS.TOOL_RUNNING) return false;
    return toolCollapseParts(message).detail.length > 0;
  }

  function messageAvatarRawIcon(message: Message): string {
    return teamMemberMeta(message)?.icon || agentMeta(message)?.icon || "";
  }

  function messageAvatarIcon(message: Message): string {
    if (message.role === "user") return "person";
    const icon = messageAvatarRawIcon(message);
    if (icon && !isAvatarAssetRef(icon)) return icon;
    if (isAvatarAssetRef(icon)) return "";
    return isTeamMember(message) ? "" : "smart_toy";
  }

  function messageAvatarInitials(message: Message): string {
    const raw = displayMessageName(message);
    const compact = raw.replace(/[()（）]/g, " ").replace(/\s+/g, " ").trim();
    if (!compact) return "…";
    const parts = compact.split(" ").filter(Boolean);
    if (parts.length >= 2) {
      const a = parts[0]!.slice(0, 1);
      const b = parts[1]!.slice(0, 1);
      return (a + b).toUpperCase();
    }
    const w = parts[0] || compact;
    if (/[\u4e00-\u9fff]/.test(w) && w.length >= 2) return w.slice(0, 2);
    if (/[\u4e00-\u9fff]/.test(w)) return w.slice(0, 1);
    return w.length <= 2 ? w.toUpperCase() : w.slice(0, 2).toUpperCase();
  }

  function assistantErrorDetail(message: Message): string {
    const raw = message.error_message?.trim();
    if (raw) return raw;
    const body = message.content_markdown?.trim() || "";
    if (body === "对话生成失败。") {
      return "未返回详细错误，请查看用量事件或后端日志。";
    }
    return "";
  }

  function userSendTagLine(message: Message): string {
    let intentKind = "";
    try {
      const raw = JSON.parse(message.options_json || "{}") as {
        intent_artifact?: { intent_kind?: string };
      };
      if (raw.intent_artifact?.intent_kind) {
        intentKind = raw.intent_artifact.intent_kind;
      }
    } catch {
      /* ignore */
    }
    const parts: string[] = [];
    if (intentKind) parts.push(intentKind);
    const st = message.status?.trim();
    if (st && st !== MESSAGE_STATUS.OK) parts.push(st);
    const err = message.error_message?.trim();
    if (err) parts.push(err);
    return parts.join(" · ");
  }

  return {
    t,
    reasoningMarkdown,
    displayMessageName,
    teamMemberMeta,
    messageIdentityKey,
    structuredToolEvent,
    isCollapsibleToolDetail,
    toolCollapseSummary,
    toolCollapseDetail,
    isTeamMember,
    isStreaming,
    isContinued,
    messageAvatarColor,
    bubbleAccentStyle,
    messageAvatarRawIcon,
    messageAvatarIcon,
    messageAvatarInitials,
    assistantErrorDetail,
    userSendTagLine,
    isToolEventMessage,
  };
}

export const CHAT_MESSAGE_AVATAR_SIZE = "44px";
export const CHAT_MESSAGE_AVATAR_ICON_SIZE = "24px";
