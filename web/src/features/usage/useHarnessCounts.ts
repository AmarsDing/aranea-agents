import { onMounted, ref } from 'vue';
import { listChannelsPaged } from '../channels/api';
import { listCredentials } from '../credentials/api';
import { listCronTasks } from '../cron/api';
import { listGraphs } from '../graph/api';
import { listMcpServersPaged } from '../mcp/api';
import { listSkills } from '../skills/api';
import { listTools } from '../tools/api';

export type HarnessCountKey = 'tools' | 'skills' | 'mcp' | 'channels' | 'credentials' | 'graphs' | 'cron';
export type HarnessCounts = Partial<Record<HarnessCountKey, number>>;

/**
 * 基座总览各领域卡上的数。每个数独立读取：没有权限的不发请求，读取失败的不出现在结果里，
 * 界面对缺的数写「—」，不显示成 0。
 */
export function useHarnessCounts(canSee: (path: string) => boolean) {
  const counts = ref<HarnessCounts>({});

  const loaders: Array<[HarnessCountKey, string, () => Promise<number>]> = [
    ['tools', '/tools', async () => (await listTools({ page: 1, page_size: 1 })).total],
    ['skills', '/skills', async () => (await listSkills({ page: 1, page_size: 1 })).total],
    ['mcp', '/mcp-servers', async () => (await listMcpServersPaged({ page_size: 1 })).total],
    ['channels', '/channels', async () => (await listChannelsPaged({ page_size: 1 })).total],
    ['credentials', '/credentials', async () => (await listCredentials({ pageSize: 1 })).total],
    // shortcut: 工作流接口不返回总数，这里最多数到 200 个；超过时接口加 total 再改
    ['graphs', '/graphs', async () => (await listGraphs(200)).items.length],
    ['cron', '/cron', async () => (await listCronTasks()).length],
  ];

  async function refresh() {
    const next: HarnessCounts = {};
    await Promise.all(
      loaders
        .filter(([, path]) => canSee(path))
        .map(async ([key, , load]) => {
          try {
            next[key] = await load();
          } catch (err) {
            console.warn(`[overview] ${key} count failed`, err);
          }
        }),
    );
    counts.value = next;
  }

  onMounted(refresh);
  return { counts, refresh };
}
