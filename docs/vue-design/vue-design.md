# 前端编写代码规范（Vue 3 / Quasar / Pinia）

> **文档性质**：本文是 **`web/` 前端开发的绑定准则**。人类开发者与 **AI 编码助手** 在实施界面开发、重构与迁移时，**以本文为最高优先级**；与本文冲突的旧习惯默认废弃，除非 PR 中显式说明「例外 + 技术债」。

**目标**：单向数据流、职责分离、组合式逻辑。新人或 AI 在 **5 分钟内** 能回答：「这段逻辑应落在哪一层？」

**适用范围**：`web/` 目录（Quasar SPA，Vue 3 + TypeScript + Pinia）。

**AI 系统提示精简版**（英文 MUST/MUST NOT，便于粘贴）：[vue-design-agent-rules.md](./vue-design-agent-rules.md)

---

## 0. AI / 开发者速览（必须先读）

### 0.1 唯一合法数据流（禁止逆行）

```text
API / Service（纯函数，只谈 HTTP 与类型）
        ↓ 仅能被 Store actions（或 Store 调用的私有方法）调用
Pinia Store（状态 + 业务过程 + loading/error）
        ↓ 仅能被 Composable / Page 读取与触发 action
Composable（为页面打包响应式 API，可组合多 Store）
        ↓
Page（布局、路由、把数据交给子组件）
        ↓ props
Component（仅展示：props in / emits out）
```

**用词约定**：

- **展示组件**：`components/**` 下、以「渲染与交互表象」为主的 `.vue` 文件；**禁止**依赖 Pinia、**禁止**调用业务 API（见下表）。
- **页面**：`pages/**` 下的 `*Page.vue`；可调用 Composable，**禁止**长串散装 `await fetch` / 复制多页共用的业务过程。

### 0.2 禁止事项一览（AI 逐条自检）

在 **展示组件** 的 `<script>` 中，**不得**出现以下 import 或等价调用（容器组件若白名单批准，须在文件首注释 `// Container: approved because ...`）：

| 禁止 | 说明 |
|------|------|
| `useXxxStore` / `defineStore` | 状态与请求收敛在 Store |
| `features/*/api`、`services/`（`kratosApi` / `createFooService`）中发请求的门面 | 网络只能在 Service/API 层声明，**请求动作**在 Store |
| `axios` / `api.get` / `kratosApi` / `createFooService()` 等 | 同上 |
| 在组件内 `watch` + 拉列表并 `ref` 存「跨组件要共享」的业务数据 | 应进 Store |

**允许**在展示组件内：

- `vue`、`@vueuse/*`（无网络）、`quasar` 组件 API、`defineProps` / `defineEmits`、**纯展示**用的 `computed`（仅依赖 props）、本地 UI 状态（如 `expanded`、`tab`）且不承载业务真源。

### 0.3 放哪里的决策树（AI 按顺序判断）

1. **是否涉及与后端交换数据或全局共享的业务状态？**  
   - 是 → **不进**纯展示组件；先考虑 **Store + `features/<域>/api.ts` 或现有 `services/`**。
2. **是否只是「一个接口、无跨页面状态」？**  
   - 仍把调用放在 **Store action**（可设小型 domain store），Composable 只调 action；避免 Page 直接调 Service。
3. **是否多页面复用同一套加载/筛选逻辑？**  
   - 写 **`useXxx` Composable**，内部只组合 Store，不直接 `axios`。
4. **是否仅影响单组件外观、且数据已全部由父级传入？**  
   - **展示组件** + props/emits。

---

## 1. 架构关系图（与 0.1 一致）

```text
┌─────────────────────────────────────────────────────┐
│  Page                                                │
│  布局、路由、调用 useXxx()、props 下发、处理 emits    │
└──────────┬──────────────────────┬───────────────────┘
           │                        │
           ▼                        ▼
┌─────────────────────┐   ┌────────────────────────────┐
│ Composable (useXxx) │   │ Component（展示）            │
│ 读/写 Store         │   │ 仅 props / emits           │
│ 不直接 HTTP*        │   │ 无 Store、无业务 API        │
└──────────┬──────────┘   └────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────┐
│ Pinia Store                                         │
│ state + actions；actions 内 async + loading/error    │
└──────────┬──────────────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────────────────────────┐
│ API / Service：`features/<域>/api.ts`、`services/*` │
│ 纯函数、无 Pinia、无路由                             │
└─────────────────────────────────────────────────────┘
```

\* **首选**：Composable 只调用 **Store actions**，不直接 import `features/*/api`。若团队在某次迁移中为减轻改动暂留 `Composable → Service`，必须在 composable 文件顶部注释 `// TECH-DEBT: direct API call; move to store — issue #xxx`，且新代码禁止照搬。

---

## 2. 与本仓库目录的对应关系（`web/src/`）

| 层级 | 路径 | 职责 |
|------|------|------|
| HTTP 纯封装 | `features/<域>/api.ts`（及域内子模块）、`services/index.ts`（`create*Service`）、`axiosHandler`（**`kratosApi`**）；过渡 **`/api/v1/...`** 在同域 **`legacyRest.ts`**（如 **`features/chat/legacyRest.ts`**）或 **`features/*/api.ts`** 用 **`kratosApi`** / **`axios` + `getBackendBaseURL()`** —— **禁止**在其中追加 **已迁至 Kratos** 的新逻辑 | 只做请求与类型映射，**不**持有业务 loading |
| Store | `stores/<域>/index.ts`，经 `stores/index.ts` **具名导出**；**default export** 必须保留 Quasar 要求的 **Pinia 工厂** | 领域状态 + actions |
| Composable | `composables/useXxx.ts`（跨域）或 `features/<域>/useXxx.ts`（域内） | 暴露给 Page 的薄 API |
| 展示组件 | `components/<域>/**/*.vue` | props / emits |
| 页面 | `pages/**/*Page.vue` | 组合布局与 Composable |
| Feature | `features/<域>/` | **`api.ts`**、域内 **`useXxx.ts`**、与 Store 互补的模块；**不含** §0.2 展示用 `.vue`（见 §2 路径硬性） |

**路径硬性 + 浮层 UX（须遵守）**

- 凡 **§0.2** 所定义的 **展示组件**（纯 props / emits、禁止 Store / 业务 API 调用），其 **`.vue` 必须** 放在 **`components/<域>/`**，**禁止** 放在 `features/<域>/` 或 `pages/` 内再套一层「伪 feature」目录取代该路径。
- **`features/<域>/`** 只放：HTTP 门面 `api.ts`、域内 composable、**容器**组件（若经 §0.2 表的白名单批准，须在文件首注释 `// Container: approved because …`）。
- **Dialog / Drawer / 全屏表单** 等浮层：默认视为展示组件，**同路径规则**；`script` 内 **禁止** `import features/*/api`、`create*Service` 发请求 — 只 **`emit('submit', payload)`**，由 **Page / Store action** 调 API（与 §0.1 一致）。
- **浮层视觉**（非纯逻辑）：须遵守 **[UX.md](../UI/UX.md) §1～§2**：`background: var(--glass-elevated)`（或 `--glass-surface`）+ **`backdrop-filter` 与 `-webkit-backdrop-filter`** 成对；主按钮用 **`var(--color-accent)` / `--color-accent-hover`**，**禁止**在日间用夜间霓虹青紫作默认强调（UX §1）。
- 与展示子组件紧耦合、**无网络请求** 的纯函数 / 常量可与展示组件同域共址为 **`components/<域>/*.ts`**（示例：`components/teams/teamUtils.ts`）；其中 **仅允许** 对 `features/<域>/api` **type-only** import，禁止运行时依赖会触发 §0.2 禁止项的模块。
- **新代码与迁移 PR** 须按上表落盘；存量若暂在 `features/<域>/` 的展示 `.vue`，应在同一域的迁移中 **迁至** `components/<域>/` 并改 import，**不得**扩大「临时共址」范围。
- **Monitor 域（落地对照）**：展示 `.vue` 位于 [`web/src/components/monitor/`](../../web/src/components/monitor/)（如 `MonitorPage` 组合的 `MonitorHeroSection`、`MonitorGlassPanel`、`AuditTable`、`TraceList`、`RealtimeEvents`、`LogStream` 等）；[`web/src/features/monitor/`](../../web/src/features/monitor/) **仅**保留 **`api.ts`、`types.ts`、`utils.ts`**（生成客户端封装与类型、纯函数），由页面与子组件 `import`。若后续收紧 **B5c**，SSE 相关组件应改为 **emit**，由 Page / Store 调 `features/monitor/api`。

---

## 3. 各层细则（AI 实现时的约束）

### 3.1 Service / `features/*/api.ts`

- 一个函数对应 **一个** 后端能力（或同一资源的单一操作）。
- **不得**：读 `useRoute`、改 Pinia、`$q.notify`（通知在 Store 或 Composable 中统一策略）。
- **本仓库**：
  - **Kratos**：在 **`features/<域>/api.ts`**（或 **`features/<域>/<topic>.ts`**，例如会话用 **`features/session/api.ts`**）中 **`import { createXxxService } from "../../services/index"`**（路径按目录调整），经 **`requestHandler`** 访问 **`/v1/...`**。
  - **过渡路径（网关改写或旧前缀）**：写在 **`features/*/api.ts`** 或与 **`legacyRest.ts`** 同域收口；优先 **`kratosApi`** **`/v1/...`**。**禁止**把这些文件当成「万能兜底」。
  - 新路径集中写 **feature api**，勿在 `.vue` 写裸 URL。

### 3.2 Store

- 按域拆分（`agents`、`avatar` 等），避免单文件持续增长。
- **异步、错误、列表重置** 等放在 **actions**；对外暴露清晰的 `loadXxx` / `saveXxx`。
- Getter 可用 `computed`（Setup Store）；敏感写入不走「外部随意 patch」。

### 3.3 Composable

- 命名：`use` 前缀。
- 返回：`ref`/`computed`/方法；副作用（何时拉数）写清 JSDoc 或简短注释。
- **默认**只依赖 Store；若直接调 Service 须按 §1 标注技术债。

### 3.4 展示组件

- **磁盘路径**：必须符合 **§2「路径硬性」**——`.vue` 落在 **`components/<域>/`**，不得放在 `features/<域>/`（容器白名单例外见 §2）。
- 完整 **`defineProps` / `defineEmits`**；类型优先 TypeScript + 泛型 props（必要时）。
- **禁止**把「是否登录」「权限」「列表来源」等业务真源藏在组件内部；由父级传入或全局路由元信息在 Page 层处理后再传 props。

### 3.5 Page

- **理想行数**：`<script setup>` 以「import + 调用一个或少几个 composable + 路由绑定」为主；超出则下沉逻辑。
- 仅处理：布局、`useRoute`、把 emits 转给 composable 提供的方法。

---

## 4. AI 开发/迁移检查清单（交付前必跑）

在做完功能或重构后，对改动文件逐条勾选（PR 描述可粘贴本节勾选结果）：

- [ ] 是否存在 **展示组件** 直接调用 API / Store？若有 → 已上收或已备案例外。
- [ ] 新网络请求是否只出现在 **`features/*/api.ts` 或 `services/`**，且由 **Store action** 触发？
- [ ] 同一数据是否在多组件重复 `fetch`？若是 → 已合并到 Store 单一数据源。
- [ ] Page 是否仅组合 composable + 传参，无大段业务 `if/else`？
- [ ] 新增 Store 是否已在 `stores/index.ts` 具名导出？未破坏 **default export Pinia**？
- [ ] **Quasar** 专用：`boot/pinia` 与 `stores/index` 的 Pinia 安装方式是否与现有仓库一致（避免双 Pinia）？

---

## 5. 迁移剧本（legacy → 合规）

当 AI 或开发者接到「迁移旧代码」任务时，按顺序执行：

1. **画数据流**：标出当前「谁发起请求、谁保存列表、谁被多页面读取」。
2. **抽 Service**：若请求逻辑嵌在 `.vue` 或巨无霸 composable，先挪到 `features/<域>/api.ts`（或域内子模块如 **`features/session/api.ts`**）；**Kratos** 调用一律经 **`services/index.ts`** 的 **`create*Service()`** 或 **`kratosApi`**。确需旧前缀时用 **`axios`** + **`getBackendBaseURL()`**：实现在 **`features/<域>/api.ts`** 或 **`features/<域>/legacyRest.ts`**，**勿**再建 mega facade。
3. **建或扩展 Store**：新增 action `loadXxx`，把原 `ref` 列表、`loading` 移入 state。
4. **写或收窄 Composable**：`useXxx` 只暴露 `storeToRefs` / 调用 `store.loadXxx`。
5. **瘦 Page**：删除散装请求，换 composable。
6. **瘦组件**：删除 Store/API import，改为 props；原 `emit` 由 Page 接住再调 composable/store。
7. **回归**：相关路由点一遍；检查无循环依赖（Store 勿 import `.vue`）。

---

## 6. 正反例（AI 对照修正）

| 场景 | 反例 | 正例 |
|------|------|------|
| 列表卡片 | `AgentCard.vue` 内 `listMessages()` | Page/composable 拉数据，`AgentCard` 只收 `agent` prop |
| 头像 | 展示组件里 `createAvatarService()` | `stores/avatar` 或父级已算好 `src`，或 props 传 `imageSrc` |
| 全局会话 | 多个 Page 各自 `useAppStore` 散装 patch | 与现有 `stores/app.ts` 对齐，扩展 **action** |
| 一次性弹窗只读 | 子组件 `onMounted` 里 `api.get` | Store action `openDialogLoad` 或由 Page 打开前 `store.load` |

---

## 7. 端到端示例（规范形状）

以下路径为示例；实际 import 以项目 **路径别名** 与现网目录为准。

**`features/skill/api.ts`**

```typescript
import { createAgentService } from "../../services";

export async function fetchAgent(agentId: string) {
  const svc = createAgentService();
  return svc.GetAgent({ id: agentId });
}
```

**`stores/skill/index.ts`**

```typescript
import { defineStore } from "pinia";
import { ref } from "vue";
import { fetchAgentSkillStats } from "../../features/skill/api";

export const useSkillStore = defineStore("skill", () => {
  const stats = ref<unknown[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);

  async function loadStats(agentId: string) {
    loading.value = true;
    error.value = null;
    try {
      stats.value = await fetchAgentSkillStats(agentId);
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载失败";
    } finally {
      loading.value = false;
    }
  }

  return { stats, loading, error, loadStats };
});
```

**`composables/useSkillStats.ts`**

```typescript
import { computed, watch } from "vue";
import { useSkillStore } from "../stores/skill";

export function useSkillStats(getAgentId: () => string | undefined) {
  const store = useSkillStore();
  watch(
    getAgentId,
    (id) => {
      if (id) void store.loadStats(id);
    },
    { immediate: true }
  );
  return {
    stats: computed(() => store.stats),
    loading: computed(() => store.loading),
    error: computed(() => store.error),
    refresh: () => {
      const id = getAgentId();
      if (id) void store.loadStats(id);
    }
  };
}
```

**`components/skill/SkillStatsTable.vue`**

```vue
<template>
  <q-table flat :rows="stats" :loading="loading" :columns="columns" />
</template>

<script setup lang="ts">
defineProps<{ stats: unknown[]; loading: boolean }>();
const columns = [/* 仅 UI 列定义 */];
</script>
```

**`pages/agent/AgentSkillPage.vue`**

```vue
<template>
  <q-page padding>
    <SkillStatsTable :stats="stats" :loading="loading" />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import SkillStatsTable from "../../components/skill/SkillStatsTable.vue";
import { useSkillStats } from "../../composables/useSkillStats";

const route = useRoute();
const agentId = computed(() => route.params.agentId as string);
const { stats, loading } = useSkillStats(() => agentId.value);
</script>
```

并在 `stores/index.ts` 增加：`import { useSkillStore } from "./skill";`，`export { useSkillStore };`（**不得**删除 default export 的 Pinia 工厂）。

---

## 8. 本文能否指导 AI？——可检验标准

满足下列 **可观测条件**，即视为本文对 AI **可用、可验**：

1. **定位**：给定「用户故事」，AI 能输出 **目标文件路径列表**（改哪些 store/composable/component/page），且每条符合 §0.3 决策树。
2. **门禁**：AI 生成的展示组件 diff 中 **不出现** §0.2 禁止 import（容器除外且须注释）。
3. **迁移**：按 §5 步骤能描述对存量文件的 **拆分顺序**，不引入循环依赖。
4. **交付**：PR 附 §4 检查清单勾选说明。

若某需求违反本文（例如必须超低延迟在叶子组件打请求），须在 PR 写清 **例外原因、边界、回收计划**；否则评审可拒。

---

## 9. 历史代码与渐进式重构

- **新代码**：默认 **全文遵守** §0～§7。
- **存量**：允许短暂偏离，但必须 **标注技术债**（见 §1 Composable 脚注）并逐步按 §5 迁移。
- 典型可归档目标：**页面级巨型 composable**（如复杂列表页）拆为 **Store（数据真源） + 薄 composable（编排） + 纯展示子组件**。

---

*文档维护：与 `web/` 同步演进；变更分层原则时需更新 §0 与 §8，并递增「文档版本」说明。*

**版本**：2026-04-29 · 以 AI 可执行为导向的修订版
