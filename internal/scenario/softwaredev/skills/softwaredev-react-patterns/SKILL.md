# Skill: React 设计模式与最佳实践

## 概述
React 前端开发的组件设计、Hooks 合规、状态管理、性能优化、服务端渲染与测试规范。目标是构建可组合、可维护、高性能的 React 应用。

## 核心规则

### 1. 组件设计
- 展示组件与容器组件分离：展示组件只接收 props + 发出 events，不直接访问全局状态
- Compound Components 模式：父子组件通过 Context 共享隐式状态（如 `<Select><Option/></Select>`）
- Render Props 用于横切逻辑复用：通过函数 prop 注入渲染逻辑，而非 HOC
- 组件职责单一：每个组件只做一件事，超过 200 行考虑拆分
- Props 接口显式定义 TypeScript 类型，禁止 `any` 或隐式 `props: any`
- 组件文件结构：一个文件一个组件，导出组件 + Props 类型
- 子组件组合优于 Props 配置：`<Card><Header/><Body/></Card>` 优于 `<Card showHeader/>`

### 2. Hooks 合规
- 依赖数组必须完整：`useEffect`/`useMemo`/`useCallback` 的 deps 包含所有闭包引用
- 自定义 Hook 命名 `use*` 前缀，返回值使用数组或具名对象
- 避免闭包陷阱：在 `useEffect` 中引用过期的 state，使用 `useRef` 或函数式更新
- `useEffect` 只用于同步外部系统（订阅、定时器、DOM 操作），不用于派生状态计算
- 派生状态直接计算：`const fullName = firstName + ' ' + lastName`，不需要 `useState` + `useEffect`
- `useRef` 用于可变值不触发重渲染，不用于存储应触发渲染的状态
- Hooks 调用顺序固定，禁止在条件/循环/嵌套函数中调用 Hooks

### 3. 状态管理
- 本地状态优先：状态尽可能靠近使用它的组件
- 共享状态提升到最小公共父组件，而非直接跳到全局 Store
- Context 慎用：仅用于主题、语言、认证等低频变更的全局状态
- 全局状态最小化：只有跨多页面/多组件高频访问的状态才进全局 Store
- 状态更新使用不可变方式：`setItems(prev => [...prev, newItem])`，禁止直接 mutate
- 服务端状态使用 TanStack Query（React Query），不手动 useState + useEffect 获取
- 表单状态使用 React Hook Form / Formik，不手动管理每个字段的 state

### 4. 性能优化
- `React.memo` 用于接收复杂 props 且频繁重渲染的组件，不盲目 memo 所有组件
- `useMemo` 缓存昂贵计算（过滤大列表、复杂转换），不缓存简单值
- `useCallback` 仅在传递给 memo 化子组件时使用，不包裹所有函数
- 代码分割：路由级使用 `React.lazy` + `Suspense`，大组件按需加载
- 虚拟列表：长列表使用 `react-window` / `react-virtuoso`，禁止渲染万级 DOM 节点
- 图片优化：使用 `loading="lazy"`、WebP 格式、响应式 `srcSet`
- 避免不必要的重渲染：稳定引用（useMemo/useCallback）、key 稳定、state 粒度细化

### 5. 服务端渲染
- Next.js App Router 为首选 SSR 框架，使用 `app/` 目录结构
- Server Components 默认，仅在需要交互时添加 `"use client"` 指令
- 数据获取在 Server Component 中直接 `async/await`，不使用 `useEffect`
- Streaming SSR：使用 `Suspense` 边界实现渐进式渲染，不阻塞整页
- 元数据使用 `generateMetadata` 而非客户端 `useEffect` 设置 `<title>`
- 客户端状态水合：Server → Client 传递序列化数据，禁止传递函数/类实例
- 路由分组使用 `(group)` 目录，布局嵌套使用 `layout.tsx`，不重复渲染

### 6. 测试
- 使用 Testing Library：测试用户行为而非实现细节
- `userEvent` 模拟真实用户交互，优于 `fireEvent`
- API Mock 使用 MSW（Mock Service Worker），拦截网络层而非 mock 模块
- 测试结构：Arrange → Act → Assert，每个测试一个断言焦点
- 组件测试关注可访问性：`getByRole` / `getByLabelText` 优于 `getByTestId`
- 自定义 Hook 测试使用 `renderHook`，不手动构建包裹组件
- E2E 测试使用 Playwright，覆盖关键用户流程

## 反模式（禁止）

- Prop drilling 超过 3 层：中间组件只透传 props 不使用，应使用 Context 或状态管理
- `useEffect` 做数据获取：竞态条件、加载状态管理复杂，应使用 TanStack Query
- 内联对象/函数 prop：每次渲染创建新引用导致子组件无效重渲染
- 在 `useEffect` 中计算派生状态：应直接在渲染时计算
- `useState` + `useEffect` 同步两个状态：应合并为单一状态源
- 条件渲染中使用 Hooks：违反 Hooks 规则，导致调用顺序不一致
- `index` 作为列表 key：列表增删导致错误复用，应使用稳定唯一 ID
- 在 Server Component 中使用 `useState`/`useEffect`：Server Component 无客户端交互能力
