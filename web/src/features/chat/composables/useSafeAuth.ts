// web/src/features/chat/composables/useSafeAuth.ts
//
// Layer-compliance wrapper for useAuthStore. Components under src/components/
// must not call useXxxStore() directly (enforced by scripts/check-frontend-layer.mjs).
// This composable returns the auth identity fields needed by v2 rendering
// components, with a safe fallback when Pinia isn't installed (e.g., unit tests).
import { useAuthStore } from '../../../stores/auth';

export interface SafeAuthIdentity {
  displayLabel: string;
  avatarLetter: string;
}

export function useSafeAuth(): SafeAuthIdentity {
  try {
    const auth = useAuthStore();
    return {
      displayLabel: auth.displayLabel || 'You',
      avatarLetter: auth.avatarLetter || 'U',
    };
  } catch {
    return { displayLabel: 'You', avatarLetter: 'U' };
  }
}
