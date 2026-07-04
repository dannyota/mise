import { serve } from '@hono/node-server';
import { Hono } from 'hono';
import pino from 'pino';
import { loadConfig } from './config.js';
import { oidcMiddleware } from './auth/oidc.js';
import { chatRoute } from './http/chat.js';
import type { ModelCall } from './agent/loop.js';

const config = loadConfig();
const logger = pino({ name: 'reasoning' });
const app = new Hono();

app.get('/healthz', (c) => c.text('ok'));

const stubModelCall: ModelCall = async () => 'ABSTAIN';

app.use('/chat', oidcMiddleware(config));
app.route('/chat', chatRoute(config, stubModelCall));

serve({ fetch: app.fetch, port: config.PORT }, (info) => {
  logger.info({ port: info.port }, 'reasoning: listening');
});

export { app };
