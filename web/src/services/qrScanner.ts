/**
 * P3.1 (mobile pairing): QR scanner wrapper around
 * tauri-plugin-barcode-scanner.
 *
 * Only functional inside the Tauri shell (Android); in plain browser or
 * dev-server contexts every call degrades so the same code can be wired
 * unconditionally. The plugin module is imported lazily so non-Tauri bundles
 * never evaluate the Tauri bridge.
 *
 * Runtime verification depends on the Android build environment (same
 * situation as the notification plugin in P2).
 */

export type QrScanOutcome =
  | { kind: 'scanned'; content: string }
  | { kind: 'cancelled' }
  | { kind: 'unavailable' }
  | { kind: 'denied' }
  | { kind: 'error' };

type BarcodeScannerPlugin = typeof import('@tauri-apps/plugin-barcode-scanner');

let pluginPromise: Promise<BarcodeScannerPlugin | null> | null = null;

function isTauriShell(): boolean {
  return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

/** True when the camera scanner can be offered (Tauri shell only). */
export function isQrScannerAvailable(): boolean {
  return isTauriShell();
}

async function loadPlugin(): Promise<BarcodeScannerPlugin | null> {
  if (!isTauriShell()) return null;
  pluginPromise ??= import('@tauri-apps/plugin-barcode-scanner').catch((err: unknown) => {
    console.warn('qrScanner: plugin load failed', err);
    return null;
  });
  return pluginPromise;
}

/** Opens the camera scanner once and resolves with the scanned text. */
export async function scanQrOnce(): Promise<QrScanOutcome> {
  const plugin = await loadPlugin();
  if (!plugin) return { kind: 'unavailable' };
  try {
    let perm = await plugin.checkPermissions();
    if (perm !== 'granted') {
      perm = await plugin.requestPermissions();
    }
    if (perm !== 'granted') return { kind: 'denied' };
    const result = await plugin.scan({ windowed: false, cameraDirection: 'back' });
    const content = result.content?.trim();
    return content ? { kind: 'scanned', content } : { kind: 'cancelled' };
  } catch (err) {
    // User closing the scanner surfaces as a rejected promise on some builds.
    console.warn('qrScanner: scan failed', err);
    return { kind: 'error' };
  }
}
