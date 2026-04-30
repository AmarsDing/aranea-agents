# Agent rules: Vue frontend (`web/`) — MUST / MUST NOT

**Scope**: Quasar SPA under `web/` (Vue 3, TypeScript, Pinia).  
**Full spec (human + AI, diagrams, examples)**: [vue-design.md](./vue-design.md) — use it for nuance; **this file is for system prompts and quick compliance checks**.

---

## Data flow (single direction; no skipping layers)

`features/<domain>/api.ts`（含域内子模块如 **`features/session/api.ts`**）
→ **Pinia store actions** (state, loading, error, business flow)  
→ **Composable `useXxx`** (thin API for pages; combine stores)  
→ **Page** (`pages/**/*Page.vue`, route + layout + wire composable)  
→ **Presentational component** (`components/**`, **props in / emits out only**)

---

## MUST

- Put **new HTTP calls** in `web/src/features/<domain>/api.ts`（或域内子模块，例如 **`features/session/api.ts`**），并通过 **`web/src/services/index.ts`** 的 **`createFooService()`** + `requestHandler` 调 Kratos；**不得**把新实现塞进 **`legacyRest.ts`** / **`axiosHandler`** 以外的自创巨型胶水入口（例外：仍为 **`legacyRestApi`** 的旧路径须写在 **`features/<域>/`** 内）。
- **遗留 REST**：仍走 **`legacyRestApi`**（`/api/v1/…`，实例来自 **`services/axiosHandler`**）。请在 **`features/<域>/api.ts`** 或 **`features/<域>/legacyRest.ts`** 收口。**禁止**在薄封装层追加 **已迁至 Kratos** 的领域逻辑（会话见 **`features/session/api.ts`**）。
- Trigger network requests from **Pinia `actions`** (store calls the API/service layer).
- Prefer **Composable → Store actions** (not Composable → API directly). *If* you must call API from a composable during migration, add at file top: `// TECH-DEBT: direct API call; move to store — issue #...`
- Pages: **route + layout + call `useXxx()` + pass props + handle emits**; keep `<script setup>` short.
- Presentational components: **`defineProps` + `defineEmits`**, `computed` only from props, **local UI state** (tabs, expanded) that is **not** the business source of truth.
- New stores: `web/src/stores/<domain>/index.ts`, **re-export** from `web/src/stores/index.ts` **without** removing Quasar’s **`export default store(() => createPinia())`** (or equivalent Pinia factory).
- After changes, verify **no duplicate Pinia** (`boot/pinia` vs `stores/index`) matches current repo pattern.

---

## MUST NOT (in presentational `components/**/*.vue` `<script>`)

- `useXxxStore`, `defineStore`, `storeToRefs` used **to drive business data** (unless file is an approved **container** with first-line comment: `// Container: approved because ...`).
- Imports from **`features/*/api`**, **`services/`** request entrypoints, **`axios`**, **`api.get`**, **`legacyRestApi`**, **`create*Service()`** used to **mutate remote state or load lists** — **including `dialogs/**` / form dialogs**: use **`emit('submit', payload)`** and handle HTTP in **Page or store action** unless the file is an approved **container** (`// Container: approved because …` at line 1).
- **`watch` + fetch + `ref`** for data **shared across components** — that belongs in a **store**.

---

## Placement cheat sheet (`web/src/`)

| Need | Put it in |
|------|-----------|
| Raw HTTP + types（**含 Kratos**） | **`features/<domain>/api.ts`** 或域内子模块（如 **`features/session/api.ts`**）；经 **`services/index.ts`** → **`create*Service()`** |
| Raw HTTP（仍 **legacy `/api/v1`**） | **`features/<domain>/api.ts`** 使用 **`legacyRestApi`**（来自 **`axiosHandler`**），或 **`features/<domain>/legacyRest.ts`**；**勿**把 **已迁 Kratos** 的新逻辑混进遗留路径 |
| Cached lists, loading, errors, workflows | `stores/<domain>/` |
| Reusable “page glue” | `composables/useXxx.ts` or `features/<domain>/useXxx.ts` |
| Pure UI（含 Dialog / Drawer；仅 props+emits） | `components/<domain>/`；**禁止**落 `features/<domain>/`；浮层材质 [UX.md](../UI/UX.md) §1～§2（玻璃 + 双前缀 blur；主操作 `--color-accent`）；**不得**在组件内直接调 `features/*/api`，应 **`emit('submit', …)`** |
| Shell + router | `pages/*Page.vue` |

---

## Migration mini-steps (when refactoring legacy)

1. Move fetches to API/service module.  
2. Add/update **store** + **actions**.  
3. Point **composable** at store only (or mark TECH-DEBT).  
4. Slim **page**; strip **component** of store/API.

---

## PR self-check (agent outputs)

- [ ] **浮层**（若有）：路径在 `components/<域>/`；无 `features/*/api` import；UX 玻璃 + accent 符合 UX.md §1～§2。
- [ ] No new network logic only inside `.vue` (except approved container).  
- [ ] Store actions own async; `stores/index.ts` still default-exports Pinia.  
- [ ] Exceptions documented with reason + follow-up.

---

*Version: aligned with vue-design.md (2026-04-29).*
