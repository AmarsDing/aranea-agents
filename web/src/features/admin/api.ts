/**
 * Admin：`AdminService.Login` POST **`/v1/admins/login`**（body：`password` + **`username`** 或 **`email`** 择一，camelCase）。
 * **`SetCookie`** 由服务端经 **`access_token`**（可 `KRATOS_AUTH_COOKIE`）下发；前端 **`kratosApi.withCredentials`** 携带。
 */
import { createAdminService } from '../../services';
import type { Admin, LoginRequest } from '../../services/kratos/admin/v1/index';
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
