/**
 * P2 (mobile): local notification wrapper around tauri-plugin-notification.
 *
 * Only functional inside the Tauri shell (Android/desktop); in plain browser
 * or dev-server contexts every call degrades to a no-op so the same code can
 * be wired unconditionally. The plugin module is imported lazily so non-Tauri
 * bundles never evaluate the Tauri bridge.
 *
 * Deep-link: notifications carry `extra.route`; the click handler registered
 * via {@link initLocalNotifications} forwards it to the caller (router.push).
 */

export type LocalNotificationPayload = {
  title: string;
  body: string;
  /** App-internal route to open on tap, e.g. `/mobile/chat?session=...`. */
  route?: string;
};

type NotificationPlugin = typeof import('@tauri-apps/plugin-notification');

let pluginPromise: Promise<NotificationPlugin | null> | null = null;

function isTauriShell(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

async function loadPlugin(): Promise<NotificationPlugin | null> {
  if (!isTauriShell()) return null;
  pluginPromise ??= import('@tauri-apps/plugin-notification').catch((err: unknown) => {
    console.warn('localNotification: plugin load failed', err);
    return null;
  });
  return pluginPromise;
}

async function ensurePermission(plugin: NotificationPlugin): Promise<boolean> {
  if (await plugin.isPermissionGranted()) return true;
  const perm = await plugin.requestPermission();
  return perm === 'granted';
}

/**
 * Registers the notification-tap listener and requests permission.
 * Returns false outside the Tauri shell or when permission is denied.
 */
export async function initLocalNotifications(onOpenRoute: (route: string) => void): Promise<boolean> {
  const plugin = await loadPlugin();
  if (!plugin) return false;
  await plugin.onAction((notification) => {
    const route = (notification.extra as { route?: unknown } | undefined)?.route;
    if (typeof route === 'string' && route.startsWith('/')) {
      onOpenRoute(route);
    }
  });
  return ensurePermission(plugin);
}

/** Fires a local notification; no-op outside Tauri or without permission. */
export async function notifyLocal(payload: LocalNotificationPayload): Promise<void> {
  const plugin = await loadPlugin();
  if (!plugin) return;
  if (!(await ensurePermission(plugin))) return;
  plugin.sendNotification({
    title: payload.title,
    body: payload.body,
    extra: payload.route ? { route: payload.route } : undefined,
  });
}
