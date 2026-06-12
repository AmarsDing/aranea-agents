import { getBackendOrigin } from './runtime';

export type AuthHealthInfo = {
  status: string;
  auth_mode: 'jwt' | 'bypass' | string;
  cookie_name: string;
  ws_path: string;
  deploy_env: string;
  reason?: string;
};

/** GET /healthz — includes auth_mode for dev diagnostics (admin-auth design §7.3).
 *  Returns partial info even on 503 (status="starting"|"failed") so callers
 *  can distinguish "server starting" from "server unreachable". */
export async function fetchAuthHealth(): Promise<AuthHealthInfo | null> {
  const origin = getBackendOrigin();
  const url = origin ? `${origin}/healthz` : '/healthz';
  try {
    const resp = await fetch(url, { method: 'GET', cache: 'no-store', signal: AbortSignal.timeout(5000) });
    if (resp.ok) {
      return (await resp.json()) as AuthHealthInfo;
    }
    // 503 means server is starting or failed — parse body for status info.
    if (resp.status === 503) {
      try {
        const body = (await resp.json()) as AuthHealthInfo;
        return body;
      } catch {
        return null;
      }
    }
    return null;
  } catch {
    return null;
  }
}
