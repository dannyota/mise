import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  apiGet,
  apiPost,
  apiDelete,
  setBearerToken,
  ApiClientError,
} from './client.js';

describe('API client', () => {
  beforeEach(() => {
    setBearerToken('test-token');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    setBearerToken(null);
  });

  it('GET sends bearer token and parses JSON', async () => {
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ tier: 'public' }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      }),
    );
    const result = await apiGet<{ tier: string }>('/bootstrap');
    expect(result.tier).toBe('public');
    const call = spy.mock.calls[0];
    expect(call).toBeDefined();
    const [url, init] = call ?? [];
    expect(String(url)).toContain('/bootstrap');
    const headers = init?.headers as Record<string, string>;
    expect(headers?.['Authorization']).toBe('Bearer test-token');
  });

  it('GET appends query params', async () => {
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('[]', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await apiGet('/search', { q: 'test', top_k: '5' });
    const call = spy.mock.calls[0];
    const [url] = call ?? [];
    expect(String(url)).toContain('q=test');
    expect(String(url)).toContain('top_k=5');
  });

  it('POST sends JSON body', async () => {
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await apiPost('/reviews/e1/promote', { reason: 'ok' });
    const call = spy.mock.calls[0];
    const [, init] = call ?? [];
    expect(init?.method).toBe('POST');
    expect(init?.body).toBe(JSON.stringify({ reason: 'ok' }));
  });

  it('DELETE sends request', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(null, { status: 204 }),
    );
    await apiDelete('/webhooks/w1');
  });

  it('sends no auth header when token is null', async () => {
    setBearerToken(null);
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('{}', {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    await apiGet('/healthz');
    const call = spy.mock.calls[0];
    const [, init] = call ?? [];
    const headers = init?.headers as Record<string, string>;
    expect(headers?.['Authorization']).toBeUndefined();
  });
});

describe('API client error handling', () => {
  beforeEach(() => {
    setBearerToken('test-token');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    setBearerToken(null);
  });

  it('throws ApiClientError on RFC 9457', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          type: 'about:blank',
          title: 'Not Found',
          status: 404,
          detail: 'Resource not found',
        }),
        { status: 404 },
      ),
    );
    await expect(apiGet('/missing')).rejects.toThrow(ApiClientError);
  });

  it('throws readable error on non-JSON 404 (HTML body)', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('<html><body>Not Found</body></html>', {
        status: 404,
        headers: { 'Content-Type': 'text/html' },
      }),
    );
    const err = await apiGet('/dashboard').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiClientError);
    const apiErr = err as InstanceType<typeof ApiClientError>;
    expect(apiErr.message).toBe('HTTP 404 for /dashboard');
    expect(apiErr.problem.status).toBe(404);
  });

  it('throws readable error when ok response has non-JSON content-type', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('<html>ok</html>', {
        status: 200,
        headers: { 'Content-Type': 'text/html' },
      }),
    );
    await expect(apiGet('/graph')).rejects.toThrow(
      /Expected JSON response for \/graph/,
    );
  });
});
