---
name: "vue-frontend-guide"
description: "Vue 3 前端通用编程规范指导。当编写 Vue 3 / Composition API / Pinia / TypeScript 代码时自动触发，提供组件设计、Composable 模式、状态管理、TypeScript 类型、响应式最佳实践等指导。"
---

# Vue 3 通用前端编程规范

> **项目特定约束**（数据流铁律、分层规范、UX 主题、聊天消息分组等）见 `aranea-frontend-guide` SKILL。本文只提供**通用 Vue 3 编程最佳实践**，不含项目特定约束。

---

## 目录

- [第一章：核心哲学](#第一章核心哲学)
- [第二章：组件设计](#第二章组件设计)
- [第三章：Composable 模式](#第三章composable-模式)
- [第四章：Pinia 状态管理](#第四章pinia-状态管理)
- [第五章：TypeScript 类型设计](#第五章typescript-类型设计)
- [第六章：响应式最佳实践](#第六章响应式最佳实践)
- [第七章：事件与通信](#第七章事件与通信)
- [第八章：性能优化](#第八章性能优化)
- [第九章：错误处理](#第九章错误处理)
- [第十章：测试规范](#第十章测试规范)
- [第十一章：命名约定](#第十一章命名约定)
- [第十二章：决策速查](#第十二章决策速查)

---

## 第一章：核心哲学

| 原则 | 说明 |
|------|------|
| 组合 > 配置 | Composition API 优于 Options API；Composable 优于 Mixin |
| 单一职责 | 组件只做一件事；Composable 只封装一个关注点 |
| 声明式 > 命令式 | 模板声明 UI 状态映射，而非命令式操作 DOM |
| 显式 > 隐式 | Props/Emits 显式声明；依赖注入显式 provide/inject |
| 不可变数据流 | Props 单向流动；事件向上冒泡；Store 集中管理共享状态 |

---

## 第二章：组件设计

### 2.1 组件分类

| 类型 | 特征 | 数据来源 |
|------|------|----------|
| **展示组件** | 纯 UI 渲染 + 交互 | Props in / Emits out |
| **容器组件** | 编排子组件 + 状态管理 | Store + Composable |
| **页面组件** | 路由入口 + 布局 | Composable + Store |
| **布局组件** | 骨架 + 插槽 | Slots |

### 2.2 Props 设计

```typescript
// ✅ 好：具体类型 + 默认值 + 校验
interface Props {
  title: string
  items: Item[]
  loading?: boolean
  modelValue?: string
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  modelValue: '',
})

// ❌ 差：any / 过多可选 / 无校验
defineProps<{
  data: any
  options?: Record<string, any>
  config?: any
}>()
```

**规则**：

1. Props 用 TypeScript interface 定义，不用运行时声明
2. 必填 props 不加 `?`；可选 props 必须有默认值
3. 禁止 `any` 类型；用具体类型或泛型
4. Props 数量 ≤ 8；超过则拆分组件或用对象 props
5. 禁止在子组件中修改 props（用 `emit('update:modelValue')` 替代）

### 2.3 Emits 设计

```typescript
// ✅ 好：显式声明 + 类型安全
const emit = defineEmits<{
  (e: 'submit', payload: SavePayload): void
  (e: 'update:modelValue', value: string): void
  (e: 'delete', id: string): void
}>()

// ❌ 差：无类型 / 隐式 emit
defineEmits(['submit', 'update:modelValue'])
```

**规则**：

1. 所有事件必须 `defineEmits` 显式声明
2. 事件名用 kebab-case（模板）或 camelCase（脚本）
3. 携带数据的事件必须有 payload 类型
4. 双向绑定用 `update:modelValue` / `update:xxx` 模式

### 2.4 组件结构约定

```vue
<script setup lang="ts">
// 1. 类型 import
import type { Item } from './types'
// 2. 组件 import
import ChildComponent from './ChildComponent.vue'
// 3. Composable import
import { useXxx } from './useXxx'
// 4. Props + Emits
const props = withDefaults(defineProps<Props>(), { ... })
const emit = defineEmits<Emits>()
// 5. Composable 调用
const { data, loading, save } = useXxx()
// 6. 响应式状态
const localState = ref(false)
// 7. 计算属性
const filteredItems = computed(() => ...)
// 8. 方法
function handleSubmit() { ... }
// 9. 生命周期 / watch
onMounted(() => { ... })
watch(() => props.items, ...)
</script>

<template>
  <!-- 模板 -->
</template>

<style lang="sass" scoped>
/* scoped 样式 */
</style>
```

### 2.5 插槽设计

```vue
<!-- ✅ 好：具名插槽 + 作用域插槽 -->
<slot name="header" :title="title" :subtitle="subtitle" />
<slot name="default" :item="item" :index="index" />
<slot name="footer" />

<!-- 使用方 -->
<template #header="{ title }">
  <h3>{{ title }}</h3>
</template>
<template #default="{ item }">
  <ItemCard :item="item" />
</template>
```

**规则**：

1. 插槽提供作用域数据时用显式类型
2. 默认插槽内容要有合理 fallback
3. 插槽数量 ≤ 5；超过考虑拆分组件

---

## 第三章：Composable 模式

### 3.1 基本结构

```typescript
export function useXxx(options?: UseXxxOptions) {
  const state = ref<State>(initialState)
  const loading = ref(false)
  const error = ref<Error | null>(null)

  const derived = computed(() => transform(state.value))

  async function loadData() {
    loading.value = true
    error.value = null
    try {
      state.value = await fetchData()
    } catch (e) {
      error.value = e as Error
    } finally {
      loading.value = false
    }
  }

  return {
    state: readonly(state),
    loading: readonly(loading),
    error: readonly(error),
    derived,
    loadData,
  }
}
```

### 3.2 Composable 设计规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 命名 | `use` 前缀 | 无前缀 |
| 返回值 | `ref`/`computed`/方法 | 返回 Store 实例 |
| 状态封装 | 内部 `ref`，外部 `readonly` | 直接暴露可变 ref |
| 副作用 | `onMounted`/`watchEffect` 中执行 | 组件外顶层副作用 |
| 清理 | `onScopeDispose` 清理定时器/监听 | 不清理导致泄漏 |
| 参数 | 响应式参数用 `MaybeRef<T>` | 强制 `.value` |
| 组合 | 调用其他 Composable | 复制逻辑 |

### 3.3 Composable 分类

| 类型 | 示例 | 特征 |
|------|------|------|
| 状态封装 | `useCounter`、`useToggle` | 纯本地状态 |
| 异步数据 | `useFetch`、`useList` | loading/error/data 三态 |
| 事件封装 | `useKeyboard`、`useResize` | 生命周期绑定 |
| 业务编排 | `useXxxPage`、`useXxxPanel` | 组合多 Store/Composable |

### 3.4 异步 Composable 模式

```typescript
export function useAsyncData<T>(fetcher: () => Promise<T>) {
  const data = ref<T | null>(null) as Ref<T | null>
  const loading = ref(false)
  const error = ref<Error | null>(null)

  async function execute() {
    loading.value = true
    error.value = null
    try {
      data.value = await fetcher()
    } catch (e) {
      error.value = e as Error
    } finally {
      loading.value = false
    }
  }

  return { data, loading, error, execute }
}
```

---

## 第四章：Pinia 状态管理

### 4.1 Store 结构

```typescript
export const useXxxStore = defineStore('xxx', () => {
  // State
  const items = ref<Item[]>([])
  const loading = ref(false)
  const error = ref<Error | null>(null)

  // Getters
  const activeItems = computed(() => items.value.filter(i => i.active))
  const itemById = computed(() => (id: string) => items.value.find(i => i.id === id))

  // Actions
  async function loadItems() {
    loading.value = true
    try {
      items.value = await fetchItems()
    } catch (e) {
      error.value = e as Error
    } finally {
      loading.value = false
    }
  }

  async function saveItem(item: Item) {
    const saved = await saveItemApi(item)
    const idx = items.value.findIndex(i => i.id === saved.id)
    if (idx >= 0) items.value[idx] = saved
    else items.value.push(saved)
  }

  function $reset() {
    items.value = []
    loading.value = false
    error.value = null
  }

  return { items, loading, error, activeItems, itemById, loadItems, saveItem, $reset }
})
```

### 4.2 Store 设计规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 按域拆分 | `useAgentsStore`、`useToolsStore` | 单文件持续增长 |
| 异步操作 | 放在 actions | 在组件中散装处理 |
| 状态重置 | 提供 `$reset` 方法 | 手动逐个清空 |
| 对外暴露 | 清晰的 `loadXxx` / `saveXxx` | 外部随意 patch |
| 跨 Store | `useXxxStore()` 调用 | Store 间 import 循环 |
| 响应式 | `storeToRefs` 解构 | 直接解构 Store 失去响应式 |

### 4.3 Store 使用规则

```typescript
// ✅ 好：storeToRefs 保持响应式
const { items, loading } = storeToRefs(useXxxStore())
const { loadItems, saveItem } = useXxxStore()

// ❌ 差：直接解构失去响应式
const { items, loading } = useXxxStore()
```

---

## 第五章：TypeScript 类型设计

### 5.1 类型组织

```typescript
// features/<域>/types.ts — 领域类型
export interface Agent {
  id: string
  name: string
  provider: string
  config: AgentConfig
}

export interface AgentConfig {
  model: string
  temperature: number
  tools: string[]
}

export type AgentStatus = 'active' | 'inactive' | 'error'

// features/<域>/api.ts — API 类型（可与领域类型不同）
export interface AgentApiResponse {
  agent_id: string
  agent_name: string
  // ...
}

// 转换函数
export function toAgent(api: AgentApiResponse): Agent {
  return { id: api.agent_id, name: api.agent_name, ... }
}
```

### 5.2 类型设计规则

| 规则 | ✅ 正确 | ❌ 错误 |
|------|---------|---------|
| 接口 > 类型别名 | `interface Agent { ... }` | `type Agent = { ... }`（除非需要联合/交叉） |
| 严格空检查 | `string \| null` | `string` 但可能 undefined |
| 禁止 any | 用 `unknown` + 类型守卫 | `any` 逃逸 |
| 枚举 | `as const` 对象或 union type | `enum`（运行时开销） |
| 泛型约束 | `<T extends BaseEntity>` | 无约束 `<T>` |
| API 类型分离 | API 响应类型 vs 领域类型 | 混用后端字段名 |

### 5.3 通用工具类型

```typescript
type MaybeRef<T> = T | Ref<T>
type AsyncState<T> = { data: T | null; loading: boolean; error: Error | null }
type PaginatedResult<T> = { items: T[]; total: number; page: number }
```

---

## 第六章：响应式最佳实践

### 6.1 ref vs reactive

| 场景 | 使用 | 原因 |
|------|------|------|
| 基本类型 | `ref` | reactive 不支持基本类型 |
| 对象（整体替换） | `ref` | `ref.value = newObj` 替换整个对象 |
| 对象（属性级修改） | `reactive` | 直接修改属性，无需 `.value` |
| 数组 | `ref` | 避免 reactive 数组陷阱 |
| Composable 返回值 | `ref` | 一致性 + 可解构 |

### 6.2 常见陷阱

```typescript
// ❌ 解构 reactive 失去响应式
const state = reactive({ count: 0, name: '' })
const { count } = state  // 失去响应式！

// ✅ 用 toRefs
const { count, name } = toRefs(state)

// ❌ 直接赋值 reactive 数组
const list = reactive<Item[]>([])
list = newData  // 错误！丢失响应式

// ✅ 用 ref 或 splice
const list = ref<Item[]>([])
list.value = newData

// ❌ watch 不触发深层
watch(source, callback)  // 只监听引用变化

// ✅ 深层监听
watch(source, callback, { deep: true })
// 或 watchEffect 自动追踪
```

### 6.3 computed 规则

1. computed 必须是纯函数，无副作用
2. 禁止在 computed 中修改状态
3. 昂贵计算用 `computed` 缓存，不要用 `watch` + `ref` 模拟
4. 需要"可写 computed"时用 getter + setter

---

## 第七章：事件与通信

### 7.1 组件通信方式选择

| 场景 | 方式 | 说明 |
|------|------|------|
| 父→子 | Props | 单向数据流 |
| 子→父 | Emits | 事件冒泡 |
| 跨层级 | Provide/Inject | 深层组件传值 |
| 全局状态 | Pinia Store | 跨组件共享 |
| 跨组件 | 事件总线（谨慎） | 仅限无关系组件 |
| DOM 事件 | `useEventListener` | Composable 封装 |

### 7.2 Provide/Inject 模式

```typescript
// Provider
const key: InjectionKey<Ref<string>> = Symbol('key')
provide(key, ref('value'))

// Injector
const value = inject(key, ref('default'))
```

**规则**：

1. 必须用 `InjectionKey<T>` 保证类型安全
2. 必须提供默认值或 null 断言
3. Inject 的值用 `readonly()` 包装防止子组件修改

---

## 第八章：性能优化

### 8.1 懒加载

```typescript
const LazyComponent = defineAsyncComponent(() => import('./HeavyComponent.vue'))
```

### 8.2 列表优化

1. 大列表用虚拟滚动（`q-virtual-scroll`）
2. `v-for` 必须用稳定 `key`（用 `id`，不用 `index`）
3. 列表项组件用 `defineComponent` + `emits` 声明避免不必要的更新

### 8.3 计算缓存

1. 模板中的复杂表达式提取为 `computed`
2. 多处复用的计算逻辑提取为 Composable
3. 避免在 `v-for` 中创建内联函数（用事件委托或提取方法）

### 8.4 其他

1. `v-once` 用于静态内容
2. `v-memo` 用于条件性更新
3. `shallowRef` / `shallowReactive` 用于大对象（避免深层代理开销）

---

## 第九章：错误处理

### 9.1 异步错误

```typescript
async function loadData() {
  loading.value = true
  error.value = null
  try {
    data.value = await fetchData()
  } catch (e) {
    error.value = e as Error
    console.error('Failed to load:', e)
  } finally {
    loading.value = false
  }
}
```

### 9.2 全局错误处理

```typescript
// app boot
app.config.errorHandler = (err, instance, info) => {
  console.error('Global error:', err, info)
}

// 组件内
onErrorCaptured((err, instance, info) => {
  console.error('Captured:', err)
  return false  // 阻止继续传播
})
```

### 9.3 规则

1. 所有异步操作必须有 try/catch
2. Store action 中捕获错误后设置 `error` 状态
3. 组件中用 `$q.notify` 展示用户友好错误（在 Composable/Store 中调用，不在展示组件中）
4. 禁止空 catch 块

---

## 第十章：测试规范

### 10.1 测试分类

| 类型 | 工具 | 范围 |
|------|------|------|
| 单元测试 | Vitest | Composable、Store、工具函数 |
| 组件测试 | Vitest + Vue Test Utils | 组件渲染 + 交互 |
| E2E | Cypress / Playwright | 关键用户流程 |

### 10.2 Composable 测试

```typescript
import { withSetup } from './test-utils'

describe('useXxx', () => {
  it('should load data', async () => {
    const { result } = withSetup(() => useXxx())
    await result.loadData()
    expect(result.data.value).toBeDefined()
    expect(result.loading.value).toBe(false)
  })
})
```

### 10.3 组件测试

```typescript
it('emits submit on button click', async () => {
  const wrapper = mount(FormComponent, {
    props: { modelValue: 'test' }
  })
  await wrapper.find('button').trigger('click')
  expect(wrapper.emitted('submit')).toBeTruthy()
})
```

---

## 第十一章：命名约定

| 场景 | 规范 | ✅ 示例 | ❌ 示例 |
|------|------|---------|---------|
| 组件文件 | PascalCase | `AgentCard.vue` | `agent-card.vue` |
| Composable 文件 | camelCase + use 前缀 | `useAgentPage.ts` | `agentPage.ts` |
| Store 文件 | 目录 + index.ts | `stores/agents/index.ts` | `stores/agents.ts` |
| 类型文件 | camelCase | `types.ts` | `Types.ts` |
| API 文件 | camelCase | `api.ts` | `Api.ts` |
| CSS 文件 | kebab-case + 下划线前缀 | `_css-vars-light.sass` | `cssVarsLight.sass` |
| Props | camelCase | `:modelValue` | `:model-value`（模板中可用 kebab） |
| Emits | camelCase | `@update:modelValue` | `@updateModelValue` |
| 事件名 | kebab-case | `@item-click` | `@itemClick` |
| CSS class | kebab-case | `.app-dialog-card` | `.appDialogCard` |
| CSS 变量 | kebab-case + 双横线 | `--color-accent` | `-colorAccent` |
| 常量 | SCREAMING_SNAKE | `MAX_RETRIES` | `maxRetries` |
| 布尔 ref | is/has/should 前缀 | `isLoading`、`hasError` | `loading`、`error`（易与类型混淆） |

---

## 第十二章：决策速查

```
需要什么？
│
├─ 组件间共享状态     → Pinia Store
├─ 组件间单向传值     → Props / Emits
├─ 跨层级传值         → Provide / Inject
├─ 复用有状态逻辑     → Composable
├─ 复用无状态逻辑     → 工具函数
├─ 表单双向绑定       → v-model + defineModel
├─ 异步数据加载       → Composable 封装（loading/error/data）
├─ 全局配置/主题      → CSS 变量 + Provide/Inject
├─ 大列表渲染         → 虚拟滚动
├─ 条件渲染重组件     → defineAsyncComponent
└─ 复杂组件状态机     → useReducer 模式或 XState
```

---

## 项目特定约束引用

以下内容不在本文范围，见 `aranea-frontend-guide` SKILL：

| 内容 | 位置 |
|------|------|
| 数据流铁律（API→Store→Composable→Page→Component） | `aranea-frontend-guide` §3 |
| 各层编码规范（API/Store/Composable/Component/Page） | `aranea-frontend-guide` §4 |
| 聊天消息分组（堆栈模型、turn_index 禁用） | `aranea-frontend-guide` §5 |
| UX 主题规范（日夜双模、玻璃材质、CSS 变量） | `aranea-frontend-guide` §6 |
| Dialog 毛玻璃规范 | `aranea-frontend-guide` §7 |
| Registry 列表表格规范 | `aranea-frontend-guide` §8 |
| 15 条红线（含 4 条已降级） | `aranea-frontend-guide` §1 |
| 编程规范 CS-F1~F9 | `aranea-frontend-guide` §13 |
