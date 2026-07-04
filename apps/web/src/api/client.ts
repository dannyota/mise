import type { ApiProblem } from '@mise/contract';

const BASE_URL =
  typeof import.meta !== 'undefined'
    ? (import.meta.env?.VITE_API_URL ?? '/api/v1')
    : '/api/v1';

let bearerToken: string | null = null;

export function setBearerToken(t: string | null): void {
  bearerToken = t;
}

export class ApiClientError extends Error {
  constructor(public readonly problem: ApiProblem) {
    super(problem.title);
    this.name = 'ApiClientError';
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {
    Accept: 'application/json',
  };
  if (bearerToken) {
    headers['Authorization'] = `Bearer ${bearerToken}`;
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const problem = (await res.json()) as ApiProblem;
    throw new ApiClientError(problem);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  return (await res.json()) as T;
}

export function apiGet<T>(
  path: string,
  params?: Record<string, string>,
): Promise<T> {
  const qs = params ? '?' + new URLSearchParams(params).toString() : '';
  return request<T>('GET', `${path}${qs}`);
}

export function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>('POST', path, body);
}

export async function apiDelete(path: string): Promise<void> {
  await request<undefined>('DELETE', path);
}
