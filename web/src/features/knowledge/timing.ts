/**
 * timing：知识库前端通用节流/防抖工具（纯函数，可单测）。
 *
 * 用于收敛高频触发源（WS 进度事件、文档列表 watch、编辑器 keystroke 写回），
 * 避免「N 个事件 → N 倍全量请求/重渲染」的请求放大。
 */

export interface DebouncedFn {
  /** 触发一次调用预约；窗口期内重复触发会重置计时。 */
  call: () => void;
  /** 立即执行挂起的调用（若有），并清除计时器。 */
  flush: () => void;
  /** 丢弃挂起的调用并清除计时器。 */
  cancel: () => void;
  /** 是否有挂起的调用。 */
  pending: () => boolean;
}

/**
 * 尾触发防抖：最后一次 call 后 waitMs 毫秒执行 fn。
 * 挂载在组件树上的调用方应在卸载时 cancel()，避免泄漏定时器。
 */
export function debounceTrailing(fn: () => void, waitMs: number): DebouncedFn {
  let timer: ReturnType<typeof setTimeout> | null = null;
  const cancel = (): void => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  };
  return {
    call: () => {
      cancel();
      timer = setTimeout(() => {
        timer = null;
        fn();
      }, waitMs);
    },
    flush: () => {
      if (timer !== null) {
        cancel();
        fn();
      }
    },
    cancel,
    pending: () => timer !== null,
  };
}
