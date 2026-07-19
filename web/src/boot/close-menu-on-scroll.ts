import { defineBoot } from '#q-app/wrappers';

/**
 * 全局滚动时关闭已打开的 QMenu（含 QSelect 下拉）。
 * 背景：主滚动容器为 window，Quasar 菜单滚动时只会重定位/钳位而不关闭，
 * 菜单会脱离锚点漂浮在视口顶部（见 dogfood 报告 ISSUE-006）。
 * 通过菜单 DOM 元素上的组件实例调用 QMenu 暴露的 hide()。
 */
export default defineBoot(() => {
  window.addEventListener(
    'scroll',
    (evt) => {
      // 菜单/对话框内部的滚动不触发关闭（如下拉树内的滚动区域）
      if (evt.target instanceof Element && evt.target.closest('.q-menu, .q-dialog')) return;
      document.querySelectorAll('.q-menu').forEach((el) => {
        const exposed = (el as HTMLElement & { __vueParentComponent?: { exposed?: { hide?: () => void } } })
          .__vueParentComponent?.exposed;
        exposed?.hide?.();
      });
    },
    { capture: true, passive: true },
  );
});
