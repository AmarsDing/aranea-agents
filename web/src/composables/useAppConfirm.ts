import { reactive } from 'vue';

export type AppConfirmTone = 'danger' | 'primary';

export interface AppConfirmOptions {
  /** 标题栏文案（必填，同时用于 aria-labelledby）。 */
  title: string;
  message?: string;
  /** danger = 破坏性操作（红色确认键），primary = 普通确认。 */
  tone?: AppConfirmTone;
  confirmLabel?: string;
  cancelLabel?: string;
  icon?: string;
}

export interface AppConfirmState {
  visible: boolean;
  title: string;
  message: string;
  tone: AppConfirmTone;
  confirmLabel: string;
  cancelLabel: string;
  icon: string;
  loading: boolean;
}

/** 全站唯一的确认框状态，由 `AppConfirmHost` 渲染。 */
export const state = reactive<AppConfirmState>({
  visible: false,
  title: '',
  message: '',
  tone: 'danger',
  confirmLabel: '',
  cancelLabel: '',
  icon: '',
  loading: false,
});

let pendingResolve: ((value: boolean) => void) | null = null;
// 串行闸门：上一次确认未结束时，后一次排队，避免两个 Promise 抢同一个 resolve。
let queue: Promise<void> = Promise.resolve();

function open(options: AppConfirmOptions): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    pendingResolve = resolve;
    state.title = options.title;
    state.message = options.message ?? '';
    state.tone = options.tone ?? 'danger';
    state.confirmLabel = options.confirmLabel ?? '';
    state.cancelLabel = options.cancelLabel ?? '';
    state.icon = options.icon ?? '';
    state.loading = false;
    state.visible = true;
  });
}

function settle(value: boolean): void {
  const resolve = pendingResolve;
  if (!resolve) return;
  pendingResolve = null;
  state.visible = false;
  state.loading = false;
  resolve(value);
}

/**
 * 请求一次确认：确认返回 true，取消 / Esc / 任意关闭返回 false。
 * 调用方只需 `if (!(await confirm({...}))) return;` 即可保留原 onOk 分支语义。
 */
export function confirm(options: AppConfirmOptions): Promise<boolean> {
  const result = queue.then(
    () => open(options),
    () => open(options),
  );
  queue = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}

/** `AppConfirmHost` 专用：用户点了确认键。 */
export function confirmAccept(): void {
  settle(true);
}

/** `AppConfirmHost` 专用：用户取消 / Esc / 关闭对话框。 */
export function confirmCancel(): void {
  settle(false);
}

/** `AppConfirmHost` 专用：异步提交期间锁定两个按钮与 Esc。 */
export function setConfirmLoading(value: boolean): void {
  if (pendingResolve) state.loading = value;
}
