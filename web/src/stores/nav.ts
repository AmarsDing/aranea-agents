import { defineStore } from 'pinia';
import { getMenus } from '../features/admin/api';

/**
 * P1-7 菜单可见性（展示层过滤）。
 *
 * 契约（P0 详设 01 §6）：后端 `GET /v1/auth/menus` 按主体权限下发菜单，
 * **后端硬判**每个 API；前端只做"仅展示"过滤，因此本 store 的失败语义是
 * **放行展示**（visiblePaths === null → 全部可见），而不是隐藏——接口不可用时
 * 整站侧栏清空会让可用性事故放大，而未授权的请求仍会被后端 403 拒绝。
 */
export const useNavStore = defineStore('nav', {
  state: () => ({
    /** 可见路由白名单；null = 未获取/获取失败（放行展示）。 */
    visiblePaths: null as string[] | null,
    /** 后端下发的生效权限码（供更细粒度控件使用；superadmin 为 ['*']）。 */
    permissions: [] as string[],
    /** 后端下发的生效角色码。 */
    roles: [] as string[],
    /** 是否已完成一次成功下发。 */
    loaded: false,
    loading: false,
  }),
  getters: {
    /**
     * 展示层判定：未获取到白名单时放行（后端硬判）；已获取时按 path 精确匹配。
     * 返回函数以支持模板内逐项调用。
     */
    isPathVisible(state): (path: string) => boolean {
      return (path: string): boolean => state.visiblePaths === null || state.visiblePaths.includes(path);
    },
  },
  actions: {
    /** 拉取并按 path 展平白名单；失败保持 null（放行展示）并允许下次重试。 */
    async ensureMenus() {
      if (this.loading || this.loaded) return;
      this.loading = true;
      try {
        const reply = await getMenus();
        if (!reply) return;
        const paths: string[] = [];
        for (const group of reply.groups ?? []) {
          for (const item of group.items ?? []) {
            if (item.path) paths.push(item.path);
          }
        }
        this.visiblePaths = paths;
        this.permissions = reply.permissions ?? [];
        this.roles = reply.roles ?? [];
        this.loaded = true;
      } finally {
        this.loading = false;
      }
    },

    /** 登录态失效/切换账号时重置，避免把上一个用户的菜单白名单带到下一个会话。 */
    reset() {
      this.visiblePaths = null;
      this.permissions = [];
      this.roles = [];
      this.loaded = false;
      this.loading = false;
    },
  },
});
