/**
 * P2 (mobile): persisted session token for cross-origin clients.
 *
 * The login response carries the same JWT that the server sets as an
 * HttpOnly cookie. Same-origin web deployments never need this module —
 * the cookie is sent automatically. Tauri Android / frp deployments are
 * cross-origin (page served from the loopback proxy, backend on a public
 * origin), so the WebView cookie may be unavailable; the stored token is
 * then attached as `Authorization: Bearer` (axios interceptor) and as the
 * WS `?token=` query (buildWsUrl default).
 *
 * Storage: localStorage. Acceptable here because the token is exactly what
 * a cross-origin deployment must expose to JS anyway; same-origin security
 * still relies on the HttpOnly cookie.
 */

const STORAGE_KEY = 'aranea.auth.token';

export function getAuthToken(): string {
  if (typeof localStorage === 'undefined') return '';
  try {
    return localStorage.getItem(STORAGE_KEY)?.trim() ?? '';
  } catch {
    return '';
  }
}

export function setAuthToken(token: string): void {
  const v = token.trim();
  if (!v) return;
  try {
    localStorage.setItem(STORAGE_KEY, v);
  } catch {
    // storage unavailable (private mode) — session falls back to cookie
  }
}

export function clearAuthToken(): void {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

/** Authorization header value for axios interceptor; empty when no token. */
export function bearerAuthHeader(): Record<string, string> {
  const token = getAuthToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}
