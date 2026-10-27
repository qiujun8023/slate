import axios, { type AxiosError, type AxiosRequestConfig } from 'axios';
import type { Envelope } from '@/api';
import { notifyUnauthorized } from '@/features/auth/lib/auth-events';
import { tokenStorage } from '@/features/auth/lib/auth-storage';

export const api = axios.create({
  baseURL: '/api',
  timeout: 30_000,
});

api.interceptors.request.use((config) => {
  const token = tokenStorage.get();
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (r) => r,
  (err: AxiosError) => {
    if (err.response?.status === 401) {
      notifyUnauthorized();
    }
    return Promise.reject(err);
  }
);

async function call<T>(config: AxiosRequestConfig): Promise<T> {
  const { data } = await api.request<Envelope>(config);
  return data.data as T;
}

/** JSON API calls; each resolves to the envelope's data. */
export const http = {
  get: <T>(url: string, config?: AxiosRequestConfig) => call<T>({ ...config, method: 'GET', url }),
  post: <T>(url: string, body?: unknown, config?: AxiosRequestConfig) =>
    call<T>({ ...config, method: 'POST', url, data: body }),
  put: <T>(url: string, body?: unknown) => call<T>({ method: 'PUT', url, data: body }),
  patch: <T>(url: string, body?: unknown, config?: AxiosRequestConfig) =>
    call<T>({ ...config, method: 'PATCH', url, data: body }),
  delete: <T>(url: string) => call<T>({ method: 'DELETE', url }),
};

/** Binary responses: frames, previews and audio blobs. */
export async function fetchBinary(url: string, config?: AxiosRequestConfig): Promise<ArrayBuffer> {
  const { data } = await api.request<ArrayBuffer>({
    method: 'GET',
    ...config,
    url,
    responseType: 'arraybuffer',
  });
  return data;
}
