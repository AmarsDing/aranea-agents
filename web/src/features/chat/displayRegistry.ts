/**
 * 展示型通知注册表（LBG-8，2026-09-02）。
 *
 * NoticeBlock 对机器载荷通知（如 deliverables）不再硬编码分支，改为查本注册表：
 * noticeType → { parse, component }。命中且 parse 非 null 时由对应组件渲染；
 * 未注册或解析失败返回 null，调用方退化回普通 markdown 通知（兜底语义不变）。
 *
 * 新增展示类型只需一个注册动作（noticeType + parser + 组件），不碰分发主流程。
 */
import type { Component } from 'vue';
import DeliverablesNoticeBody from '../../components/chat/notices/DeliverablesNoticeBody.vue';
import { DELIVERABLES_NOTICE_TYPE, parseDeliverableRefs } from './deliverablesNotice';

export interface DisplayTypeRegistration {
  /** 命中的 NoticeType（注册与查找均大小写不敏感、去首尾空白）。 */
  noticeType: string;
  /** 解析机器载荷；返回 null 表示载荷不合法，调用方回退普通通知。 */
  parse: (content: string) => unknown | null;
  /** 渲染组件；接收 parse 产物作为 `payload` prop。 */
  component: Component;
}

export interface ResolvedDisplay {
  component: Component;
  payload: unknown;
}

const registry = new Map<string, DisplayTypeRegistration>();

/** 注册展示类型；同 noticeType 后注册覆盖先注册。 */
export function registerDisplayType(reg: DisplayTypeRegistration): void {
  const key = reg.noticeType.trim().toLowerCase();
  if (key) registry.set(key, reg);
}

/** 按 noticeType 查注册表；未注册返回 undefined。 */
export function resolveDisplayType(noticeType: string | undefined): DisplayTypeRegistration | undefined {
  const key = (noticeType ?? '').trim().toLowerCase();
  return key ? registry.get(key) : undefined;
}

/** 命中注册表且载荷解析成功 → 组件+载荷；否则 null（调用方回退普通通知）。 */
export function resolveDisplayPayload(noticeType: string | undefined, content: string): ResolvedDisplay | null {
  const reg = resolveDisplayType(noticeType);
  if (!reg) return null;
  const payload = reg.parse(content);
  return payload == null ? null : { component: reg.component, payload };
}

// deliverables：首个注册项（parser 复用 parseDeliverableRefs，组件抽自原
// NoticeBlock 硬编码分支，渲染行为零变化）。
registerDisplayType({
  noticeType: DELIVERABLES_NOTICE_TYPE,
  parse: (content) => parseDeliverableRefs(DELIVERABLES_NOTICE_TYPE, content),
  component: DeliverablesNoticeBody,
});
