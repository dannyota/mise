import { serve } from '@hono/node-server';
import { Hono } from 'hono';
import pino from 'pino';
import { loadConfig } from './config.js';

const config = loadConfig();
const logger = pino({ name: 'reasoning' });
const app = new Hono();

app.get('/healthz', (c) => c.text('ok'));

serve({ fetch: app.fetch, port: config.PORT }, (info) => {
  logger.info({ port: info.port }, 'reasoning: listening');
});
