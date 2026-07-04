import { describe, expect, it } from 'vitest';
import type {
  SseTokenEvent,
  SseAbstainEvent,
  SseDoneEvent,
  SseErrorEvent,
  SearchOutput,
} from '@mise/contract';
import { SSE_EVENT_TYPES } from '@mise/contract';
import { chatRequestSchema } from './http/request-envelope.js';

describe('contract: SSE events', () => {
  it('SSE_EVENT_TYPES has exactly 7 event types', () => {
    expect(SSE_EVENT_TYPES).toHaveLength(7);
    expect(SSE_EVENT_TYPES).toContain('token');
    expect(SSE_EVENT_TYPES).toContain('citation');
    expect(SSE_EVENT_TYPES).toContain('chain');
    expect(SSE_EVENT_TYPES).toContain('evidence_checked');
    expect(SSE_EVENT_TYPES).toContain('abstain');
    expect(SSE_EVENT_TYPES).toContain('done');
    expect(SSE_EVENT_TYPES).toContain('error');
  });

  it('token event matches contract shape', () => {
    const event: SseTokenEvent = {
      event: 'token',
      data: { text: 'hello' },
    };
    expect(event.event).toBe('token');
    expect(event.data.text).toBe('hello');
  });

  it('abstain event matches contract shape', () => {
    const event: SseAbstainEvent = {
      event: 'abstain',
      data: { reason: 'insufficient evidence' },
    };
    expect(event.event).toBe('abstain');
  });

  it('done event matches contract shape', () => {
    const event: SseDoneEvent = {
      event: 'done',
      data: { model: 'test', iterations: 1 },
    };
    expect(event.data.model).toBe('test');
  });

  it('error event matches contract shape', () => {
    const event: SseErrorEvent = {
      event: 'error',
      data: { type: 'agent_error', detail: 'test error' },
    };
    expect(event.data.type).toBe('agent_error');
  });
});

describe('contract: chat request', () => {
  it('validates a minimal request', () => {
    const result = chatRequestSchema.safeParse({ question: 'What is X?' });
    expect(result.success).toBe(true);
  });

  it('rejects empty question', () => {
    const result = chatRequestSchema.safeParse({ question: '' });
    expect(result.success).toBe(false);
  });
});

describe('contract: MCP types', () => {
  it('search output sections array is typed', () => {
    const output: SearchOutput = {
      sections: [
        {
          corpus_id: 'vn-reg',
          document_id: 'test',
          section_id: 'test',
          doc_number: 'test',
          title: 'test',
          citation_path: 'Dieu 1',
          heading_path: 'test',
          text: 'test',
          validity_status: 'in_force',
          score: 0.9,
          source_url: 'https://example.com',
        },
      ],
    };
    expect(output.sections).toHaveLength(1);
  });
});
