import { getBackendOrigin } from "./runtime";

export type AuthHealthInfo = {
  status: string;
  auth_mode: "jwt" | "bypass" | string;
  cookie_name: string;
  ws_path: string;
  deploy_env: string;
};

/** GET /healthz — includes auth_mode for dev diagnostics (admin-auth design §7.3). */
export async function fetchAuthHealth(): Promise<AuthHealthInfo | null> {
  const origin = getBackendOrigin();
  const url = origin ? `${origin}/healthz` : "/healthz";
  try {
    const resp = await fetch(url, { method: "GET", cache: "no-store", signal: AbortSignal.timeout(5000) });
    if (!resp.ok) return null;
    return (await resp.json()) as AuthHealthInfo;
  } catch {
    return null;
  }
}
