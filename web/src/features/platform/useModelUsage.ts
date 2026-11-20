import { computed, ref, type Ref } from 'vue';
import { listAgentsPaged } from '../agents/api';
import type { Agent } from '../agents/types';
import type { PlatformResource } from './types';

export type MissingModel = { provider: string; model: string; agents: Agent[] };

const keyOf = (provider: string, model: string) => `${provider.trim().toLowerCase()}/${model.trim().toLowerCase()}`;

/**
 * 模型 ↔ Agent 的关系（Agent 按 provider + model 两个字符串引用模型，没有外键）：
 *  - 每个登记的模型被几个 Agent 用；
 *  - Agent 引用了、却没有登记的模型——运行时只认 llm_provider_models 表，这些 Agent 建不起来。
 * 只数 agents.provider/model；运行时设置、系统设置里的模型引用不在这里（见 docs 连接领域审查）。
 */
export function useModelUsage(models: Ref<PlatformResource[]>) {
  const agents = ref<Agent[]>([]);
  const loaded = ref(false);
  const failed = ref(false);

  async function load() {
    failed.value = false;
    try {
      // shortcut: 后端单次最多给 500 个 Agent，超过 500 时计数会偏少；到那时改成后端按 provider/model 分组计数的接口
      agents.value = (await listAgentsPaged({ limit: 500, offset: 0 })).items;
      loaded.value = true;
    } catch {
      failed.value = true; // 没有 Agent 读权限或失败：关系一栏显示「—」，不影响模型列表
    }
  }

  const byModel = computed(() => {
    const map = new Map<string, Agent[]>();
    for (const agent of agents.value) {
      if (!agent.provider || !agent.model) continue;
      const key = keyOf(agent.provider, agent.model);
      map.set(key, [...(map.get(key) ?? []), agent]);
    }
    return map;
  });

  function agentsUsing(row: PlatformResource): Agent[] {
    return byModel.value.get(keyOf(row.provider ?? '', row.model ?? '')) ?? [];
  }

  /** 被 Agent 引用、但没有任何登记行（启用或停用都算登记）的模型，按引用数从多到少 */
  const missing = computed<MissingModel[]>(() => {
    const registered = new Set(models.value.map((row) => keyOf(row.provider ?? '', row.model ?? '')));
    const out: MissingModel[] = [];
    for (const list of byModel.value.values()) {
      const first = list[0];
      if (!first || registered.has(keyOf(first.provider, first.model))) continue;
      out.push({ provider: first.provider, model: first.model, agents: list });
    }
    return out.sort((a, b) => b.agents.length - a.agents.length);
  });

  /** 指向停用模型的 Agent 数（模型登记了但停用，Agent 同样建不起来） */
  const onDisabled = computed(() =>
    models.value.filter((row) => !row.enabled).reduce((sum, row) => sum + agentsUsing(row).length, 0),
  );

  return { agents, loaded, failed, load, agentsUsing, missing, onDisabled };
}
