// 专业模式：同一个任务的两层深度里，默认停在哪一层。
//   关（默认）：任务详情先看「概况」（该你做什么、进展、结果），交办面板只留必要的选择；
//   开：任务详情先看「过程」（执行链、终端、每一步），列表和详情带上轮数 / Token / 花费，交办面板展开高级项。
// 只是个人的显示偏好，记在本机；不改权限（Harness 能不能进仍由权限决定）。
// 设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §5
import { ref, watch } from 'vue';

const STORE_KEY = 'aranea.pro';

function read(): boolean {
  try {
    return window.localStorage.getItem(STORE_KEY) === '1';
  } catch {
    return false;
  }
}

// 模块级：各处拿到的是同一个开关，切一下全站跟着变
const pro = ref(read());
watch(pro, (on) => {
  try {
    window.localStorage.setItem(STORE_KEY, on ? '1' : '0');
  } catch {
    /* 写不进去（隐私模式等）就只在本次会话里生效 */
  }
});

export function useProMode() {
  return {
    pro,
    toggle: () => {
      pro.value = !pro.value;
    },
  };
}
