import { createHash } from 'node:crypto';
import type { Logger } from 'pino';
import type { Caller } from '../auth/caller.js';

export type ToolCallRecord = {
  readonly toolName: string;
  readonly startedAt: string;
  readonly completedAt?: string;
  readonly durationMs?: number;
  readonly caller: string;
  readonly correlationId: string;
};

export type AuditHooks = {
  onPreToolUse(toolName: string): ToolCallRecord;
  onPostToolUse(record: ToolCallRecord): void;
  onModelTurn(model: string, prompt: string): void;
};

export function createAuditHooks(logger: Logger, caller: Caller): AuditHooks {
  return {
    onPreToolUse(toolName: string): ToolCallRecord {
      const record: ToolCallRecord = {
        toolName,
        startedAt: new Date().toISOString(),
        caller: caller.sub,
        correlationId: caller.correlationId,
      };
      logger.info({ audit: 'pre_tool_use', ...record }, 'tool call started');
      return record;
    },

    onPostToolUse(record: ToolCallRecord): void {
      const completedAt = new Date().toISOString();
      const durationMs =
        new Date(completedAt).getTime() - new Date(record.startedAt).getTime();
      logger.info(
        { audit: 'post_tool_use', ...record, completedAt, durationMs },
        'tool call completed',
      );
    },

    onModelTurn(model: string, prompt: string): void {
      const promptHash = createHash('sha256')
        .update(prompt)
        .digest('hex')
        .slice(0, 16);
      logger.info(
        {
          audit: 'model_turn',
          model,
          promptHash,
          caller: caller.sub,
          correlationId: caller.correlationId,
          timestamp: new Date().toISOString(),
        },
        'model turn',
      );
    },
  };
}
