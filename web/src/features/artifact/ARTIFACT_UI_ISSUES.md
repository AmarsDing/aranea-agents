# 制品管理页面（ArtifactsPage）问题清单

> 审查日期：2026-05-29
> 审查范围：前端 ArtifactsPage + 后端 ArtifactService 全链路
> 审查技能：aranea-review

---

## 一、BUG（功能缺陷）

### A1: 后端 `toProtoArtifactMeta` 缺失 `StorageUri` 字段 ✅ 已修复

- **文件**: `internal/service/artifact.go` L282-294
- **现象**: `toProtoArtifactMeta` 映射函数未设置 `StorageUri` 字段，导致所有 API 响应中 `storage_uri` 始终为空字符串
- **影响**: 前端详情弹窗 `ArtifactsDetailDialog.vue` 显示 "存储：fs — "（URI 为空）
- **修复**: 在映射函数中添加 `StorageUri: a.StorageURI,`

### A2: 前端未暴露 `DeleteArtifactVersion` API ✅ 已修复

- **文件**: `web/src/features/artifact/api.ts`
- **现象**: 后端 proto 已定义 `DeleteArtifactVersion` RPC（DELETE `/v1/artifacts/{id}/versions/{version}`），但前端 api.ts 未封装该接口，Store 也无对应方法
- **影响**: 用户无法在 UI 中删除单个版本，只能删除整个逻辑制品（所有版本）
- **修复**: 在 api.ts 添加 `deleteArtifactVersion` 函数，Store 添加 `removeVersion` 方法

---

## 二、架构违规（红线违反）

### B1: `ArtifactPreview.vue` 直接调用 API ✅ 已修复

- **文件**: `web/src/features/artifact/ArtifactPreview.vue` L45
- **现象**: 直接 import `previewArtifact` from `./api` 并调用，绕过 Store
- **违反红线**: 前端红线 #2（展示组件不得 import `features/*/api`）
- **修复**: 改用 `useArtifactStore().loadPreview()`

### B2: `useArtifactsPage.ts` 直接调用 API 获取版本列表 ✅ 已修复

- **文件**: `web/src/features/artifact/useArtifactsPage.ts` L5, L135
- **现象**: 直接 import `listArtifactVersions` from `./api` 并调用，绕过 Store
- **违反红线**: 数据流规范要求 composable 通过 Store 访问 API
- **修复**: 在 Store 添加 `listVersions` 方法，composable 改用 Store

### B3: `ArtifactList.vue` 直接调用 API ✅ 已修复

- **文件**: `web/src/features/artifact/ArtifactList.vue` L56
- **现象**: 直接 import `signDownloadUrl`、`artifactDownloadHref`、`deleteArtifact` from `./api` 并调用
- **违反红线**: 前端红线 #4（Dialog/浮层组件不得在组件内直接调 API）
- **修复**: 下载改用 Store 的 `signDownload` + `artifactDownloadHref`；删除改用 Store 的 `remove`

### B4: 展示组件放置位置错误 ✅ 已修复

- **文件**: `ArtifactsUploadDialog.vue`、`ArtifactsDetailDialog.vue`
- **现象**: 纯展示组件（仅 props/emits）放在 `features/artifact/components/`，违反红线 #5
- **违反红线**: 前端红线 #5（展示组件 .vue 放 `components/<域>/`，禁止放在 `features/<域>/`）
- **修复**: 迁移至 `components/artifact/`，更新 import 路径

---

## 三、UX 问题

### C1: Session 筛选输入框缺少防抖 ✅ 已修复

- **文件**: `web/src/pages/ArtifactsPage.vue` L14
- **现象**: `sessionFilter` 输入框没有 `debounce`，每次按键都触发 `watch` → `loadRows()` API 调用
- **修复**: 添加 `debounce="300"` + `@update:model-value="onSessionFilterChange"`

### C2: `created_at` 列显示原始时间戳字符串 ✅ 已修复

- **文件**: `web/src/pages/ArtifactsPage.vue`
- **现象**: 创建时间列直接显示 ISO 字符串，无本地化格式化
- **修复**: 添加 `body-cell-created_at` slot，使用共享 `formatDate()` 格式化

### C3: `ArtifactList.vue` 下载失败静默吞错 ✅ 已修复

- **文件**: `web/src/features/artifact/ArtifactList.vue` L109
- **现象**: `onDownload` 的 catch 块静默吞错，用户无反馈
- **修复**: 添加 `$q.notify` 错误提示

### C4: 表格 `version` 列显示裸数字 ✅ 已修复

- **文件**: `web/src/pages/ArtifactsPage.vue`
- **现象**: 表格版本列显示 `1`、`2`，详情弹窗显示 `v1`、`v2`，风格不统一
- **修复**: 添加 `body-cell-version` slot，显示 `v{{ version }}`

---

## 四、代码质量

### D1: `formatBytes` 函数在 6 处重复定义 ✅ 已修复

- **现象**: 同一功能重复实现，且逻辑不一致（有的支持 GB，有的不支持）
- **修复**: 提取到 `web/src/shared/format.ts` 共享模块，统一实现（支持 GB）

### D2: `formatDate` 函数在 10+ 处重复定义 ✅ 部分修复

- **现象**: 各组件各自定义 `formatDate`
- **修复**: 在 `shared/format.ts` 添加 `formatDate`，制品域已统一使用；其他域待后续迭代迁移

---

## 五、aranea-review 审查结果

### 审查通过项（7/9）

| 检查项 | 判定 | 说明 |
|--------|:----:|------|
| FD1: 展示组件是否 import Store/API | ✅ | 展示组件无 Store/API import；容器组件 import Store 合规 |
| FD2: Page 是否直接 import api | ✅ | ArtifactsPage.vue 仅 import composable |
| FD4: Dialog/浮层是否内部调 API | ✅ | Dialog 通过 emit 或 Store 间接操作 |
| FD6: 新 HTTP 调用是否在 api.ts | ✅ | `deleteArtifactVersion` 正确添加在 api.ts |
| FL5: composable 是否绕过 Store | ✅ | useArtifactsPage.ts 全部通过 Store 调用 |
| FU9: 是否使用 AppRegistryTable | ✅ | ArtifactsPage.vue 正确使用 |
| FU10: 表格列宽是否用 registryCol | ✅ | artifactTableUi.ts 全部使用标准列定义 |

### 已修复项（2/9，原不通过）

| 检查项 | 原判定 | 修复后 |
|--------|:------:|:------:|
| FL1: 展示组件位置 | ❌ | ✅ 迁移至 `components/artifact/` |
| FB4: $q.notify 位置 | ❌ | 🟡 建议级，记录备忘 |

### 待改进项（🟡 建议级，非阻断）

1. **FB4**: `ArtifactList.vue` 中 3 处 `$q.notify` 应上提到 composable 或改为 emit 事件，由父组件处理通知
2. **ArtifactPreview.vue 逻辑重复**: 该组件内联了与 `useArtifactPreview.ts` 完全相同的逻辑，应改为使用该 composable
3. **artifactDownloadHref 归属**: Store 暴露的 `artifactDownloadHref` 是纯工具函数，更适合放在 `shared/` 中

---

## 六、修复优先级

| 优先级 | 编号 | 说明 | 状态 |
|--------|------|------|:----:|
| P0 | A1 | 后端数据丢失，影响详情展示 | ✅ |
| P1 | B1, B2, B3, B4 | 架构红线违规 | ✅ |
| P1 | C1 | 性能问题，每次按键触发 API | ✅ |
| P2 | C2, C3, C4 | UX 体验问题 | ✅ |
| P2 | A2 | 缺失功能 | ✅ |
| P3 | D1, D2 | 代码质量 | ✅ |
