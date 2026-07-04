import { describe, expect, it } from 'vitest';
import { Hono } from 'hono';
import { oidcMiddleware } from './oidc.js';
import { getCaller } from './caller.js';
import type { Config } from '../config.js';

function fakeConfig(overrides: Partial<Config> = {}): Config {
  return {
    PORT: 3001,
    SERVING_URL: 'http://localhost:8080',
    NODE_ENV: 'test',
    MODEL_DEFAULT: 'claude-haiku-4-5-20251001',
    MODEL_ESCALATION: 'claude-sonnet-4-6-20250514',
    MCP_URL: 'http://localhost:8080/mcp',
    ABSTAIN_THRESHOLD: 0.3,
    ESCALATION_THRESHOLD: 0.5,
    MAX_ITERATIONS: 5,
    MAX_TOKENS: 4096,
    ...overrides,
  };
}

describe('oidcMiddleware', () => {
  it('uses fake caller when OIDC_ISSUER is not set', async () => {
    const app = new Hono();
    app.use('*', oidcMiddleware(fakeConfig()));
    app.get('/test', (c) => c.json(getCaller(c)));

    const res = await app.request('/test');
    expect(res.status).toBe(200);
    const body = (await res.json()) as Record<string, unknown>;
    expect(body.sub).toBe('dev-user');
    expect(body.tier).toBe('mise_local');
  });

  it('reads X-Fake-Tier header in dev mode', async () => {
    const app = new Hono();
    app.use('*', oidcMiddleware(fakeConfig()));
    app.get('/test', (c) => c.json(getCaller(c)));

    const res = await app.request('/test', {
      headers: { 'X-Fake-Tier': 'mise_public' },
    });
    const body = (await res.json()) as Record<string, unknown>;
    expect(body.tier).toBe('mise_public');
  });

  it('rejects missing bearer token when OIDC is configured', async () => {
    const app = new Hono();
    app.use(
      '*',
      oidcMiddleware(
        fakeConfig({
          OIDC_ISSUER: 'https://auth.example.com',
          OIDC_AUDIENCE: 'mise',
        }),
      ),
    );
    app.get('/test', (c) => c.text('ok'));

    const res = await app.request('/test');
    expect(res.status).toBe(401);
  });

  it('rejects invalid bearer token when OIDC is configured', async () => {
    const app = new Hono();
    app.use(
      '*',
      oidcMiddleware(
        fakeConfig({
          OIDC_ISSUER: 'https://auth.example.com',
          OIDC_AUDIENCE: 'mise',
        }),
      ),
    );
    app.get('/test', (c) => c.text('ok'));

    const res = await app.request('/test', {
      headers: { Authorization: 'Bearer invalid.jwt.token' },
    });
    expect(res.status).toBe(401);
  });
});
