// 经营驾驶舱数据编排（整合 P2-8）：15s 轮询总览 + 节点抽屉下钻。
// 行为口径与 ceo-dashboard 原 App.vue 一致：
//   - 刷新失败保留旧数据，仅以 banner 明示「看板后端不可达」；
//   - 下钻回源失败降级：用总览已物化的节点快照开抽屉，banner 明示
//     「明细不可用」而非空白（扫漏项）；
//   - 错误提示不经全局 toast（axiosHandler 对 CeoboardService skipErrorNotify），
//     全部由页面 banner 自呈现——轮询场景避免 15s 一次的 toast 风暴。

import { onMounted, onUnmounted, ref } from 'vue';
import { i18n } from '../../i18n';
import { createCeoboardService } from '../../services';
import { fetchCompanySnapshot } from './company';
import { normalizeNodeDetail, normalizeOverview } from './normalize';
import type { CompanySnapshot, NodeDetail, Overview } from './types';

export const CEOBOARD_POLL_INTERVAL_MS = 15_000;

function errMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

export function useCeoboard() {
  const svc = createCeoboardService();
  const ov = ref<Overview | null>(null);
  const snap = ref<CompanySnapshot | null>(null);
  const err = ref('');
  const snapErr = ref('');
  const drawer = ref<NodeDetail | null>(null);
  let timer: number | undefined;

  async function refresh() {
    // 两个数据源互不阻塞：流程总览失败不影响经营快照展示，反之亦然。
    const [ovRes, snapRes] = await Promise.allSettled([
      svc.GetOverview({}),
      fetchCompanySnapshot(),
    ]);
    if (ovRes.status === 'fulfilled') {
      ov.value = normalizeOverview(ovRes.value);
      err.value = '';
    } else {
      err.value = i18n.global.t('ceoboard.page.backendUnreachable', { msg: errMessage(ovRes.reason) });
    }
    if (snapRes.status === 'fulfilled') {
      snap.value = snapRes.value;
      snapErr.value = '';
    } else {
      // 保留上一次快照；仅提示经营数据暂不可用。
      snapErr.value = i18n.global.t('ceoboard.ops.unavailable', { msg: errMessage(snapRes.reason) });
    }
  }

  async function openNode(wfId: string, nodeId: string) {
    try {
      drawer.value = normalizeNodeDetail(await svc.GetNode({ workflowId: wfId, nodeId }));
    } catch (e: unknown) {
      // 下钻回源失败降级：用总览快照开抽屉，明示「明细不可用」。
      const wf = ov.value?.lanes.flatMap((l) => l.workflows).find((w) => w.id === wfId);
      const nv = wf?.nodes.find((nd) => nd.id === nodeId);
      if (wf && nv && ov.value) {
        drawer.value = {
          ...nv,
          workflow_id: wf.id,
          workflow_name: wf.name,
          enter: i18n.global.t('ceoboard.drawer.detailMissing'),
          exit: i18n.global.t('ceoboard.drawer.detailMissing'),
          as_of: ov.value.as_of,
          fetch_err: i18n.global.t('ceoboard.drawer.fetchErr', { msg: errMessage(e) }),
        };
      } else {
        err.value = i18n.global.t('ceoboard.drawer.openFailed', { msg: errMessage(e) });
      }
    }
  }

  function closeDrawer() {
    drawer.value = null;
  }

  onMounted(() => {
    void refresh();
    timer = window.setInterval(() => void refresh(), CEOBOARD_POLL_INTERVAL_MS);
  });
  onUnmounted(() => {
    if (timer !== undefined) window.clearInterval(timer);
  });

  return { ov, snap, err, snapErr, drawer, refresh, openNode, closeDrawer };
}
