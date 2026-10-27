// JWT 登录态管理。App 启动时如果 localStorage 有 token，自动 GET /users/me 回填 user
// （否则 Layout 等组件读 ctx.user 永远是 null，刷新后看不到登出菜单）。

import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { resetUnauthorizedState, setUnauthorizedHandler } from '@/features/auth/lib/auth-events';
import { AUTH_TOKEN_STORAGE_KEY, tokenStorage } from '@/features/auth/lib/auth-storage';
import { http } from '@/lib/http';
import { notifySessionEnded } from '@/features/auth/lib/session-events';
import { meQueryKey, useMe, type CurrentUser } from '@/features/auth/query/auth-queries';
import { appRoutes } from '@/app/routes';
import type { LoginRequest, RegisterRequest, Session } from '@/api';
import { AuthContext } from './model/auth-context';
import { safeRedirectPath } from './lib/redirect';

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(tokenStorage.get());
  const navigate = useNavigate();
  const qc = useQueryClient();
  const me = useMe(!!token);
  const user = token ? (me.data ?? null) : null;

  const handleAuthSuccess = useCallback(
    (data: { token: string; user: CurrentUser }, redirectTo: string) => {
      tokenStorage.set(data.token);
      resetUnauthorizedState();
      setToken(data.token);
      qc.setQueryData(meQueryKey, data.user);
      navigate(safeRedirectPath(redirectTo), { replace: true });
    },
    [navigate, qc]
  );

  const clearLocalSession = useCallback(() => {
    setToken(null);
    qc.clear();
    notifySessionEnded();
  }, [qc]);

  useEffect(() => {
    return setUnauthorizedHandler(() => {
      tokenStorage.clear();
      clearLocalSession();
      if (window.location.pathname !== appRoutes.login)
        navigate(appRoutes.login, { replace: true });
    });
  }, [clearLocalSession, navigate]);

  useEffect(() => {
    function onStorage(event: StorageEvent) {
      if (event.key !== AUTH_TOKEN_STORAGE_KEY) return;
      if (!event.newValue) {
        clearLocalSession();
        if (window.location.pathname !== appRoutes.login)
          navigate(appRoutes.login, { replace: true });
        return;
      }
      setToken(event.newValue);
      resetUnauthorizedState();
      void qc.invalidateQueries({ queryKey: meQueryKey });
    }
    window.addEventListener('storage', onStorage);
    return () => window.removeEventListener('storage', onStorage);
  }, [clearLocalSession, navigate, qc]);

  const login = useCallback(
    async (creds: LoginRequest, redirectTo: string = appRoutes.home) => {
      const data = await http.post<Session>(`/sessions`, creds);
      handleAuthSuccess(data, redirectTo);
    },
    [handleAuthSuccess]
  );

  const register = useCallback(
    async (creds: RegisterRequest, redirectTo: string = appRoutes.home) => {
      const data = await http.post<Session>(`/users`, creds);
      handleAuthSuccess(data, redirectTo);
    },
    [handleAuthSuccess]
  );

  const logout = useCallback(() => {
    // JWT 没有服务端状态，登出只清本地。
    resetUnauthorizedState();
    tokenStorage.clear();
    clearLocalSession();
    navigate(appRoutes.login, { replace: true });
  }, [clearLocalSession, navigate]);

  return (
    <AuthContext.Provider value={{ token, user, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
}
