export type AdminSession = {
  id: number;
  name: string;
  email: string;
  access: string;
  avatar: string;
  /** P2 (mobile): JWT returned by Login only; persisted via services/authToken for Bearer/WS auth. */
  token?: string;
  /** P1 补漏·强制改密门:true 时路由守卫把用户钉在 /force-reset-password,后端同步只放行改密/登出/查自身。 */
  forceReset?: boolean;
};
