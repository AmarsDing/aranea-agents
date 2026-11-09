/**
 * Admin：`AdminService.Login` POST **`/v1/admins/login`**（body：`password` + **`username`** 或 **`email`** 择一，camelCase）。
 * **`SetCookie`** 由服务端经 **`access_token`**（可 `KRATOS_AUTH_COOKIE`）下发；前端 **`kratosApi.withCredentials`** 携带。
 */
import { createAdminService } from '../../services';
import type { Admin, LoginRequest, MenusReply } from '../../services/kratos/admin/v1/index';
import type { AdminSession } from './types';

const adminSvc = createAdminService();

function mapAdmin(raw: unknown): AdminSession {
  const r = raw as Record<string, unknown>;
  const id = Number(r.id ?? 0);
  const token = typeof r.token === 'string' && r.token ? r.token : undefined;
  return {
    id,
    name: String(r.name ?? ''),
    email: String(r.email ?? ''),
    access: String(r.access ?? ''),
    avatar: String(r.avatar ?? ''),
    ...(token ? { token } : {}),
    forceReset: r.forceReset === true,
  };
}

export async function loginAdminByUsername(username: string, password: string): Promise<AdminSession> {
  const req: LoginRequest = { password, username: username.trim() };
  const data = await adminSvc.Login(req);
  return mapAdmin(data);
}

export async function loginAdminByEmail(email: string, password: string): Promise<AdminSession> {
  const req: LoginRequest = { password, email: email.trim() };
  const data = await adminSvc.Login(req);
  return mapAdmin(data);
}

export async function logoutAdmin(): Promise<void> {
  await adminSvc.Logout({});
}

export async function getCurrentAdmin(): Promise<AdminSession> {
  const data = await adminSvc.Current({});
  return mapAdmin(data as Admin);
}

/**
 * P1 补漏·强制改密门:自助改密。成功即吊销全部会话(含当前),调用方须清理
 * 本地会话并回登录页。这是 force_reset 账号唯一可用的写操作。
 */
export async function changeAdminPassword(oldPassword: string, newPassword: string): Promise<void> {
  await adminSvc.ChangePassword({ oldPassword, newPassword });
}

/**
 * P1-7 菜单下发：后端按角色裁剪后的侧栏目录 + 生效权限码。
 * 返回 null 表示接口不可用（调用方放行展示——真正的判定在后端）。
 */
export async function getMenus(): Promise<MenusReply | null> {
  try {
    return await adminSvc.GetMenus({});
  } catch {
    return null;
  }
}
