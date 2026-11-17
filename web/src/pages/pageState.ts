/**
 * 页面五态判定（加载中 / 空数据 / 部分失败 / 全量失败 / 无权限）。
 *
 * 模板按 `v-if / v-else-if` 链消费本函数的返回值并渲染 `AppStatePanel`，
 * 因此「错误优先于空态」由判定顺序保证，而不是靠各页面各自拼条件：
 * 失败且无数据只可能得到 `error`，不会同时出现 `empty`。
 */

export type PageStateVariant = 'loading' | 'empty' | 'error' | 'partial' | 'forbidden';

export type PageStateInput = {
  /** 首屏/刷新中的加载标记 */
  loading: boolean;
  /** 数据来源失败信息（空串表示无错误） */
  error: string;
  /** 当前已持有的列表行数（不是 total） */
  count: number;
  /** 403 / PERMISSION_DENIED；由 composable 用 utils/kratosError.isForbiddenApiError 判定后传入 */
  forbidden?: boolean;
};

/**
 * 五态判定：无权限 > 全量失败 > 加载中 > 空数据；有数据时失败降级为 `partial`，
 * 列表照常渲染，只在上方加紧凑横幅。返回 `null` 表示正常渲染列表。
 */
export function resolvePageState({ loading, error, count, forbidden = false }: PageStateInput): PageStateVariant | null {
  const hasRows = count > 0;
  if (forbidden) return hasRows ? 'partial' : 'forbidden';
  if (error) return hasRows ? 'partial' : 'error';
  if (loading && !hasRows) return 'loading';
  if (!hasRows) return 'empty';
  return null;
}
