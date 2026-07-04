import { afterEach, describe, expect, it, vi } from 'vitest';
import { Hono } from 'hono';
import { oidcMiddleware } from './auth/oidc.js';
import { chatRoute } from './http/chat.js';
import type { Config } from './config.js';

function e2eConfig(): Config {
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

function e2eApp(modelResponse: string): Hono {
  const config = e2eConfig();
  const app = new Hono();
  app.get('/healthz', (c) => c.text('ok'));
  app.use('/chat', oidcMiddleware(config));
  app.route(
    '/chat',
    chatRoute(config, async () => modelResponse),
  );
  return app;
}

function mockEmptyMcp(): void {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(
    new Response(JSON.stringify({ result: { output: { sections: [] } } })),
  );
}

function parseSSEEvents(text: string): Array<{ event: string; data: string }> {
  const events: Array<{ event: string; data: string }> = [];
  const blocks = text.split('\n\n').filter(Boolean);
  for (const block of blocks) {
    const lines = block.split('\n');
    let event = '';
    let data = '';
    for (const line of lines) {
      if (line.startsWith('event:')) {
        event = line.slice(6).trim();
      } else if (line.startsWith('data:')) {
        data = line.slice(5).trim();
      }
    }
    if (event) {
      events.push({ event, data });
    }
  }
  return events;
}

describe('E2E: Audit Q&A', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('healthz returns ok', async () => {
    const app = e2eApp('test');
    const res = await app.request('/healthz');
    expect(res.status).toBe(200);
    expect(await res.text()).toBe('ok');
  });

  it('abstain path: no evidence produces abstain + done', async () => {
    mockEmptyMcp();
    const app = e2eApp('should not be called');
    const res = await app.request('/chat', {
      method: 'POST',
      body: JSON.stringify({ question: 'Nonexistent question' }),
      headers: { 'Content-Type': 'application/json' },
    });
    expect(res.status).toBe(200);

    const text = await res.text();
    const events = parseSSEEvents(text);
    const eventTypes = events.map((e) => e.event);
    expect(eventTypes).toContain('abstain');
    expect(eventTypes).toContain('done');
    expect(eventTypes).not.toContain('token');
  });

  it('rejects empty question', async () => {
    const app = e2eApp('test');
    const res = await app.request('/chat', {
      method: 'POST',
      body: JSON.stringify({ question: '' }),
      headers: { 'Content-Type': 'application/json' },
    });
    expect(res.status).toBe(422);
  });

  it('rejects missing body', async () => {
    const app = e2eApp('test');
    const res = await app.request('/chat', {
      method: 'POST',
      body: 'not json',
      headers: { 'Content-Type': 'text/plain' },
    });
    expect(res.status).toBe(400);
  });
});
