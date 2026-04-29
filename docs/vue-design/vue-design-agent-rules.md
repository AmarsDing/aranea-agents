# Agent rules: Vue frontend (`web/`) — MUST / MUST NOT

**Scope**: Quasar SPA under `web/` (Vue 3, TypeScript, Pinia).  
**Full spec (human + AI, diagrams, examples)**: [vue-design.md](./vue-design.md) — use it for nuance; **this file is for system prompts and quick compliance checks**.

---

## Data flow (single direction; no skipping layers)

`features/<domain>/api.ts` or `services/*` (HTTP only, pure functions)  
→ **Pinia store actions** (state, loading, error, business flow)  
→ **Composable `useXxx`** (thin API for pages; combine stores)  
→ **Page** (`pages/**/*Page.vue`, route + layout + wire composable)  
→ **Presentational component** (`components/**`, **props in / emits out only**)

---

## MUST

- Put **new HTTP calls** in `web/src/features/<domain>/api.ts` or existing `web/src/services/*` / `api/http` patterns; **never** invent ad-hoc URLs inside `.vue`.
- Trigger network requests from **Pinia `actions`** (store calls the API/service layer).
- Prefer **Composable → Store actions** (not Composable → API directly). *If* you must call API from a composable during migration, add at file top: `// TECH-DEBT: direct API call; move to store — issue #...`
- Pages: **route + layout + call `useXxx()` + pass props + handle emits**; keep `<script setup>` short.
- Presentational components: **`defineProps` + `defineEmits`**, `computed` only from props, **local UI state** (tabs, expanded) that is **not** the business source of truth.
- New stores: `web/src/stores/<domain>/index.ts`, **re-export** from `web/src/stores/index.ts` **without** removing Quasar’s **`export default store(() => createPinia())`** (or equivalent Pinia factory).
- After changes, verify **no duplicate Pinia** (`boot/pinia` vs `stores/index`) matches current repo pattern.

---

## MUST NOT (in presentational `components/**/*.vue` `<script>`)

- `useXxxStore`, `defineStore`, `storeToRefs` used **to drive business data** (unless file is an approved **container** with first-line comment: `// Container: approved because ...`).
- Imports from **`api/client`**, **`features/*/api`**, **`services/`** request entrypoints, **`axios`**, **`api.get`**, **`legacyRestApi`**, **`create*Service()`** used to **mutate remote state or load lists**.
- **`watch` + fetch + `ref`** for data **shared across components** — that belongs in a **store**.

---

## Placement cheat sheet (`web/src/`)

| Need | Put it in |
|------|-----------|
| Raw HTTP + types | `features/<domain>/api.ts` or `services/*` |
| Cached lists, loading, errors, workflows | `stores/<domain>/` |
| Reusable “page glue” | `composables/useXxx.ts` or `features/<domain>/useXxx.ts` |
| Pure UI | `components/<domain>/` |
| Shell + router | `pages/*Page.vue` |

---

## Migration mini-steps (when refactoring legacy)

1. Move fetches to API/service module.  
2. Add/update **store** + **actions**.  
3. Point **composable** at store only (or mark TECH-DEBT).  
4. Slim **page**; strip **component** of store/API.

---

## PR self-check (agent outputs)

- [ ] No forbidden imports in presentational components.  
- [ ] No new network logic only inside `.vue` (except approved container).  
- [ ] Store actions own async; `stores/index.ts` still default-exports Pinia.  
- [ ] Exceptions documented with reason + follow-up.

---

*Version: aligned with vue-design.md (2026-04-29).*
