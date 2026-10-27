import { useQuery } from '@tanstack/react-query';
import type { User } from '@/api';
import { http } from '@/lib/http';

export const meQueryKey = ['me'] as const;

export type CurrentUser = User;

export function fetchMe(): Promise<CurrentUser> {
  return http.get<CurrentUser>('/users/me');
}

export function useMe(enabled = true) {
  return useQuery({
    queryKey: meQueryKey,
    queryFn: fetchMe,
    enabled,
  });
}
