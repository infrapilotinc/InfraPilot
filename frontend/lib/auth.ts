import { create } from "zustand";
import { persist } from "zustand/middleware";
import { api, User } from "./api";

// Helper to set cookie
function setCookie(name: string, value: string, days: number = 7) {
  const expires = new Date(Date.now() + days * 864e5).toUTCString();
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax`;
}

// Helper to delete cookie
function deleteCookie(name: string) {
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
}

interface AuthState {
  user: User | null;
  accessToken: string | null;
  refreshToken: string | null;
  mfaToken: string | null;
  mfaRequired: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<{ mfaRequired: boolean }>;
  verifyMFA: (code: string) => Promise<void>;
  cancelMFA: () => void;
  logout: () => void;
  setUser: (user: User | null) => void;
  setTokens: (accessToken: string, refreshToken: string) => Promise<void>;
  refreshAccessToken: () => Promise<void>;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      user: null,
      accessToken: null,
      refreshToken: null,
      mfaToken: null,
      mfaRequired: false,
      isLoading: false,

      login: async (email: string, password: string) => {
        set({ isLoading: true });
        try {
          const response = await api.login(email, password);

          if (response.mfa_required && response.mfa_token) {
            // Store MFA token and signal MFA is required
            set({
              mfaToken: response.mfa_token,
              mfaRequired: true,
            });
            return { mfaRequired: true };
          }

          if (response.access_token) {
            // Set in localStorage for API calls
            localStorage.setItem("access_token", response.access_token);
            // Set in cookie for middleware auth check
            setCookie("access_token", response.access_token);

            set({
              accessToken: response.access_token,
              refreshToken: response.refresh_token || null,
              mfaToken: null,
              mfaRequired: false,
            });

            // Fetch user info
            const user = await api.getCurrentUser();
            set({ user });
          }
          return { mfaRequired: false };
        } finally {
          set({ isLoading: false });
        }
      },

      verifyMFA: async (code: string) => {
        const { mfaToken } = get();
        if (!mfaToken) {
          throw new Error("No MFA token");
        }

        set({ isLoading: true });
        try {
          const response = await api.verifyMFA(mfaToken, code);

          if (response.access_token) {
            localStorage.setItem("access_token", response.access_token);
            setCookie("access_token", response.access_token);

            set({
              accessToken: response.access_token,
              refreshToken: response.refresh_token || null,
              mfaToken: null,
              mfaRequired: false,
            });

            // Fetch user info
            const user = await api.getCurrentUser();
            set({ user });
          }
        } finally {
          set({ isLoading: false });
        }
      },

      cancelMFA: () => {
        set({ mfaToken: null, mfaRequired: false });
      },

      logout: () => {
        localStorage.removeItem("access_token");
        deleteCookie("access_token");
        set({ user: null, accessToken: null, refreshToken: null, mfaToken: null, mfaRequired: false });
        window.location.href = "/login";
      },

      setUser: (user) => set({ user }),

      setTokens: async (accessToken: string, refreshToken: string) => {
        localStorage.setItem("access_token", accessToken);
        setCookie("access_token", accessToken);
        set({
          accessToken,
          refreshToken,
          mfaToken: null,
          mfaRequired: false,
        });

        // Fetch user info
        const user = await api.getCurrentUser();
        set({ user });
      },

      refreshAccessToken: async () => {
        const { refreshToken } = get();
        if (!refreshToken) {
          throw new Error("No refresh token");
        }

        try {
          const response = await api.refreshToken(refreshToken);
          localStorage.setItem("access_token", response.access_token);
          set({ accessToken: response.access_token });
        } catch {
          get().logout();
        }
      },
    }),
    {
      name: "auth-storage",
      partialize: (state) => ({
        user: state.user,
        refreshToken: state.refreshToken,
      }),
    }
  )
);
