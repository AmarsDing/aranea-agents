import { reactive } from 'vue';

export type AppPromptType = 'text' | 'textarea' | 'radio';

export interface AppPromptItem {
  label: string;
  value: string;
  /** 置灰但保留可读文案（如「存为草稿」在草稿不可用时）。 */
  disabled?: boolean;
  /** 选项补充说明，渲染在 label 下方。 */
  hint?: string;
}

export interface AppPromptOptions {
  /** 标题栏文案（必填，同时用于 aria-labelledby）。 */
  title: string;
  /** 标题下的说明区，可多行。 */
  message?: string;
  /** 输入框标签（radio 时不用）。 */
  label?: string;
  placeholder?: string;
  type?: AppPromptType;
  /** 打开时的初值；text 默认空串，radio 默认第一项可用值。 */
  defaultValue?: string;
  /** 空值不可提交；radio 恒为必选（不受此开关影响）。 */
  required?: boolean;
  /** type=radio 的选项列表。 */
  items?: AppPromptItem[];
  confirmLabel?: string;
  cancelLabel?: string;
  /** danger = 破坏性操作（红色确认键）。 */
  tone?: 'primary' | 'danger';
  /** 默认 true：误触遮罩/Esc 不丢输入；取消仍可通过取消键或 Esc 完成。 */
  persistent?: boolean;
}

export interface AppPromptState {
  visible: boolean;
  title: string;
  message: string;
  label: string;
  placeholder: string;
  type: AppPromptType;
  items: AppPromptItem[];
  defaultValue: string;
  required: boolean;
  confirmLabel: string;
  cancelLabel: string;
  tone: 'primary' | 'danger';
  persistent: boolean;
  loading: boolean;
}

/** 全站唯一的输入类弹框状态，由 `AppPromptHost` 渲染。 */
export const promptState = reactive<AppPromptState>({
  visible: false,
  title: '',
  message: '',
  label: '',
  placeholder: '',
  type: 'text',
  items: [],
  defaultValue: '',
  required: false,
  confirmLabel: '',
  cancelLabel: '',
  tone: 'primary',
  persistent: true,
  loading: false,
});

let pendingResolve: ((value: string | null) => void) | null = null;
// 串行闸门：上一次输入未结束时后一次排队，避免两个 Promise 抢同一个 resolve。
let queue: Promise<void> = Promise.resolve();

function open(options: AppPromptOptions): Promise<string | null> {
  return new Promise<string | null>((resolve) => {
    pendingResolve = resolve;
    const type = options.type ?? 'text';
    const items = options.items ?? [];
    const firstEnabled = items.find((item) => !item.disabled)?.value ?? '';
    promptState.title = options.title;
    promptState.message = options.message ?? '';
    promptState.label = options.label ?? '';
    promptState.placeholder = options.placeholder ?? '';
    promptState.type = type;
    promptState.items = items;
    promptState.defaultValue = options.defaultValue ?? (type === 'radio' ? firstEnabled : '');
    promptState.required = type === 'radio' ? true : (options.required ?? false);
    promptState.confirmLabel = options.confirmLabel ?? '';
    promptState.cancelLabel = options.cancelLabel ?? '';
    promptState.tone = options.tone ?? 'primary';
    promptState.persistent = options.persistent ?? true;
    promptState.loading = false;
    promptState.visible = true;
  });
}

function settle(value: string | null): void {
  const resolve = pendingResolve;
  if (!resolve) return;
  pendingResolve = null;
  promptState.visible = false;
  promptState.loading = false;
  resolve(value);
}

/**
 * 请求一次输入：确认返回输入值（text/textarea 已 trim），取消 / Esc / 关闭返回 null。
 * 调用方用 `const value = await prompt({...}); if (!value) return;` 即可保留原分支语义。
 */
export function prompt(options: AppPromptOptions): Promise<string | null> {
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

/** `AppPromptHost` 专用：用户点了确认键。 */
export function promptAccept(value: string): void {
  const normalized = promptState.type === 'radio' ? value : value.trim();
  settle(normalized);
}

/** `AppPromptHost` 专用：用户取消 / Esc / 关闭对话框。 */
export function promptCancel(): void {
  settle(null);
}

/** `AppPromptHost` 专用：异步提交期间锁定两个按钮与 Esc。 */
export function setPromptLoading(value: boolean): void {
  if (pendingResolve) promptState.loading = value;
}
