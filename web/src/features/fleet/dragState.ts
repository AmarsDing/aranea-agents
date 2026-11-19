import { reactive } from 'vue';

/** 正在拖动的标签（所有窗格据此显示落点区域）。 */
export const dragging = reactive({ tab: '' });
