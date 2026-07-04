import { describe, expect, it } from 'vitest';
import pino from 'pino';
import { createAuditHooks } from './hooks.js';
import type { Caller } from '../auth/caller.js';

function testCaller(): Caller {
  return {
    sub: 'audit-test-user',
    roles: ['analyst'],
    tier: 'mise_local',
    correlationId: '00000000-0000-0000-0000-000000000099',
  };
}

function captureLogger(): { logger: pino.Logger; lines: unknown[] } {
  const lines: unknown[] = [];
  const logger = pino(
    { level: 'info' },
    {
      write(msg: string) {
        lines.push(JSON.parse(msg));
      },
    },
  );
  return { logger, lines };
}

describe('createAuditHooks', () => {
  it('logs pre and post tool use', () => {
    const { logger, lines } = captureLogger();
    const hooks = createAuditHooks(logger, testCaller());

    const record = hooks.onPreToolUse('mcp__mise__search');
    expect(record.toolName).toBe('mcp__mise__search');
    expect(record.caller).toBe('audit-test-user');

    hooks.onPostToolUse(record);
    expect(lines).toHaveLength(2);
    expect((lines[0] as Record<string, unknown>).audit).toBe('pre_tool_use');
    expect((lines[1] as Record<string, unknown>).audit).toBe('post_tool_use');
  });

  it('logs model turn with prompt hash', () => {
    const { logger, lines } = captureLogger();
    const hooks = createAuditHooks(logger, testCaller());

    hooks.onModelTurn('test-model', 'What is the capital?');
    expect(lines).toHaveLength(1);

    const entry = lines[0] as Record<string, unknown>;
    expect(entry.audit).toBe('model_turn');
    expect(entry.model).toBe('test-model');
    expect(typeof entry.promptHash).toBe('string');
    expect((entry.promptHash as string).length).toBe(16);
  });

  it('includes correlation ID in all records', () => {
    const { logger, lines } = captureLogger();
    const hooks = createAuditHooks(logger, testCaller());

    hooks.onPreToolUse('mcp__mise__search');
    hooks.onModelTurn('test-model', 'prompt');

    for (const line of lines) {
      expect((line as Record<string, unknown>).correlationId).toBe(
        '00000000-0000-0000-0000-000000000099',
      );
    }
  });
});
