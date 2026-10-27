import { createContext } from 'react';
import type { LoginRequest, RegisterRequest } from '@/api';
import type { CurrentUser } from '@/features/auth/query/auth-queries';

export interface AuthState {
  token: string | null;
  user: CurrentUser | null;
  login: (creds: LoginRequest, redirectTo?: string) => Promise<void>;
  register: (creds: RegisterRequest, redirectTo?: string) => Promise<void>;
  logout: () => void;
}

export const AuthContext = createContext<AuthState | null>(null);
