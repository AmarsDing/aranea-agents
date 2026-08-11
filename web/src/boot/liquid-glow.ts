import { defineBoot } from '#q-app/wrappers';
import type { Directive } from 'vue';

/**
 * v-liquid-glow：液态玻璃卡片的鼠标跟随高光坐标源。
 * 仅在 pointermove 时把指针相对元素的百分比写入 --liquid-mx / --liquid-my，
 * 渲染完全由 CSS（_liquid-card.sass 夜间光学层）承担；指令本身不感知主题。
 */

const handlers = new WeakMap<HTMLElement, (e: PointerEvent) => void>();

const liquidGlow: Directive<HTMLElement> = {
  mounted(el) {
    const onMove = (e: PointerEvent) => {
      const rect = el.getBoundingClientRect();
      if (rect.width === 0 || rect.height === 0) return;
      el.style.setProperty('--liquid-mx', `${(((e.clientX - rect.left) / rect.width) * 100).toFixed(1)}%`);
      el.style.setProperty('--liquid-my', `${(((e.clientY - rect.top) / rect.height) * 100).toFixed(1)}%`);
    };
    handlers.set(el, onMove);
    el.addEventListener('pointermove', onMove, { passive: true });
  },
  unmounted(el) {
    const onMove = handlers.get(el);
    if (onMove) {
      el.removeEventListener('pointermove', onMove);
      handlers.delete(el);
    }
  },
};

export default defineBoot(({ app }) => {
  app.directive('liquid-glow', liquidGlow);
});
