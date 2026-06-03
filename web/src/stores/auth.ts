import { defineStore } from 'pinia';
import {
  getCurrentAdmin,
  loginAdminByEmail,
  loginAdminByUsername,
  logoutAdmin,
} from '../features/admin/api';
import type { AdminSession } from '../features/admin/types';
import { clearServerDownNotify } from '../features/heartbeat/useServerHeartbeat';

export type AuthIdentityMode = 'username' | 'email';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as AdminSession | null,
    sessionChecked: false,
    loginLoading: false,
  }),
  getters: {
    /** Platform admin (access=admin) — required to opt into Native runtime_engine. */
    isPlatformAdmin(state): boolean {
      return (
        String(state.user?.access ?? '')
          .trim()
          .toLowerCase() === 'admin'
      );
    },
    displayLabel(state): string {
      if (!state.user) return '';
      const n = state.user.name?.trim();
      if (n) return n;
      const e = state.user.email?.trim();
      return e || `id:${state.user.id}`;
    },
    avatarLetter(state): string {
      const s = (state.user?.name || state.user?.email || '?').trim();
      return s ? s.slice(0, 1).toUpperCase() : '?';
    },
  },
  actions: {
    async ensureSession() {
      if (this.sessionChecked) return;
      try {
        this.user = await getCurrentAdmin();
      } catch {
        this.user = null;
      }
      this.sessionChecked = true;
      if (this.user) clearServerDownNotify();
    },

    forceRecheckSession() {
      this.sessionChecked = false;
    },

    async login(mode: AuthIdentityMode, identity: string, password: string) {
      const id = identity.trim();
      const pw = password;
      if (!id || !pw) {
        throw new Error('identity and password required');
      }
      this.loginLoading = true;
      try {
        this.user = mode === 'email' ? await loginAdminByEmail(id, pw) : await loginAdminByUsername(id, pw);
        this.sessionChecked = true;
        clearServerDownNotify();
      } finally {
        this.loginLoading = false;
      }
    },

    async logout() {
      try {
        await logoutAdmin();
      } catch {
        // still clear client state even if revoke fails (e.g. expired cookie)
      }
      this.user = null;
      this.sessionChecked = true;
    },
  },
});
