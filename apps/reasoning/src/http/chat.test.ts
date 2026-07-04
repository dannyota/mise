import { afterEach, describe, expect, it, vi } from 'vitest';
import { Hono } from 'hono';
import { chatRoute } from './chat.js';
import { oidcMiddleware } from '../auth/oidc.js';
import type { Config } from '../config.js';
import type { ModelCall } from '../agent/loop.js';

function testConfig(): Config {
  return {
    PORT: 3001,
    SERVING_URL: 'http://localhost:8080',
    NODE_ENV: 'test',
    MODEL_DEFAULT: 'test-haiku',
    MODEL_ESCALATION: 'test-sonnet',
    MCP_URL: 'http://localhost:8080/mcp',
    ABSTAIN_THRESHOLD: 0.3,
    ESCALATION_THRESHOLD: 0.5,
    MAX_ITERATIONS: 3,
    MAX_TOKENS: 1024,
  };
}

function fakeModelCall(response: string): ModelCall {
  return async () => response;
}

function buildApp(config: Config, modelCall: ModelCall): Hono {
  const app = new Hono();
  app.use('/chat', oidcMiddleware(config));
  app.route('/chat', chatRoute(config, modelCall));
  return app;
}

function mockMcpFetch(): void {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ result: { output: { sections: [] } } })),
  );
}

describe('chatRoute', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('rejects invalid JSON', async () => {
    const app = buildApp(testConfig(), fakeModelCall('ok'));
    const res = await app.request('/chat', {
      method: 'POST',
      body: 'not json',
      headers: { 'Content-Type': 'text/plain' },
    });
    expect(res.status).toBe(400);
  });

  it('rejects missing question', async () => {
    const app = buildApp(testConfig(), fakeModelCall('ok'));
    const res = await app.request('/chat', {
      method: 'POST',
      body: JSON.stringify({}),
      headers: { 'Content-Type': 'application/json' },
    });
    expect(res.status).toBe(422);
  });

  it('streams abstain when no evidence', async () => {
    mockMcpFetch();
    const app = buildApp(testConfig(), fakeModelCall('ok'));
    const res = await app.request('/chat', {
      method: 'POST',
      body: JSON.stringify({ question: 'What is X?' }),
      headers: { 'Content-Type': 'application/json' },
    });
    expect(res.status).toBe(200);
    const text = await res.text();
    expect(text).toContain('event: abstain');
    expect(text).toContain('event: done');
  });
});
