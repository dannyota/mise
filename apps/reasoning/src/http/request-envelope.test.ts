import { describe, expect, it } from 'vitest';
import { parseChatRequest } from './request-envelope.js';

describe('parseChatRequest', () => {
  it('parses a valid request with defaults', () => {
    const req = parseChatRequest({ question: 'What is RMiT?' });
    expect(req.question).toBe('What is RMiT?');
    expect(req.locale).toBe('en');
    expect(req.corpora).toBeUndefined();
  });

  it('parses a request with all fields', () => {
    const req = parseChatRequest({
      question: 'Cloud outsourcing rules',
      corpora: ['vn-reg', 'my-reg'],
      locale: 'vi',
      idempotencyKey: '550e8400-e29b-41d4-a716-446655440000',
    });
    expect(req.corpora).toEqual(['vn-reg', 'my-reg']);
    expect(req.locale).toBe('vi');
  });

  it('rejects empty question', () => {
    expect(() => parseChatRequest({ question: '' })).toThrow();
  });

  it('rejects missing question', () => {
    expect(() => parseChatRequest({})).toThrow();
  });

  it('rejects question over 2000 chars', () => {
    expect(() => parseChatRequest({ question: 'x'.repeat(2001) })).toThrow();
  });

  it('rejects invalid locale', () => {
    expect(() =>
      parseChatRequest({ question: 'test', locale: 'fr' }),
    ).toThrow();
  });
});
