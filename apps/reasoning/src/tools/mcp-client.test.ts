import { afterEach, describe, expect, it, vi } from 'vitest';
import { createMcpClient } from './mcp-client.js';
import type { Caller } from '../auth/caller.js';
import type { Config } from '../config.js';

function fakeCaller(overrides: Partial<Caller> = {}): Caller {
  return {
    sub: 'test-user',
    roles: ['analyst'],
    tier: 'mise_local',
    correlationId: '00000000-0000-0000-0000-000000000001',
    ...overrides,
  };
}

function fakeConfig(): Config {
  return {
    PORT: 3001,
    SERVING_URL: 'http://localhost:8080',
    NODE_ENV: 'test',
    MODEL_DEFAULT: 'claude-haiku-4-5-20251001',
    MODEL_ESCALATION: 'claude-sonnet-4-6-20250514',
    MCP_URL: 'http://localhost:9999/mcp',
    ABSTAIN_THRESHOLD: 0.3,
    ESCALATION_THRESHOLD: 0.5,
    MAX_ITERATIONS: 5,
    MAX_TOKENS: 4096,
  };
}

describe('createMcpClient', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('sends caller tier in X-Caller-Tier header', async () => {
    const fetchSpy = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(
        new Response(JSON.stringify({ result: { output: { sections: [] } } })),
      );

    const client = createMcpClient(
      fakeConfig(),
      fakeCaller({ tier: 'mise_public' }),
    );
    await client.search({ query: 'test' });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const call = fetchSpy.mock.calls[0];
    expect(call).toBeDefined();
    const [, init] = call ?? [];
    const headers = init?.headers as Record<string, string>;
    expect(headers['X-Caller-Tier']).toBe('mise_public');
    expect(headers['X-Caller-Sub']).toBe('test-user');
  });

  it('throws on non-ok response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('error', { status: 500 }),
    );

    const client = createMcpClient(fakeConfig(), fakeCaller());
    await expect(client.search({ query: 'test' })).rejects.toThrow(
      'MCP search failed: 500',
    );
  });
});
