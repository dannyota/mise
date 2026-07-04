import { describe, expect, it } from 'vitest';
import { runAgentLoop } from './loop.js';
import type { ModelCall } from './loop.js';
import type { McpClient } from '../tools/mcp-client.js';
import type { Config } from '../config.js';

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

function fakeMcp(sections: unknown[] = []): McpClient {
  return {
    async search() {
      return { sections } as never;
    },
    async document() {
      return { document: {}, sections: [] } as never;
    },
    async graph() {
      return { chain: [] };
    },
  };
}

function fakeModelCall(response: string): ModelCall {
  return async () => response;
}

const SECTION = {
  corpus_id: 'vn-reg',
  document_id: '00000000-0000-0000-0000-000000000001',
  section_id: '00000000-0000-0000-0000-000000000002',
  citation_path: 'Dieu 7 Khoan 1',
  text: 'IT risk management requirements',
  validity_status: 'in_force',
  score: 0.95,
  source_url: 'https://vbpl.vn/example',
};

describe('runAgentLoop', () => {
  it('returns abstain when search returns no results', async () => {
    const result = await runAgentLoop({
      question: 'What is X?',
      mcp: fakeMcp([]),
      config: testConfig(),
      modelCall: fakeModelCall('should not be called'),
    });
    expect(result.kind).toBe('abstain');
    expect(result.citations).toHaveLength(0);
  });

  it('returns answer with citations when evidence exists', async () => {
    const result = await runAgentLoop({
      question: 'What are IT risk requirements?',
      mcp: fakeMcp([SECTION]),
      config: testConfig(),
      modelCall: fakeModelCall(
        'Per [1] Dieu 7, banks must implement IT risk controls.',
      ),
    });
    expect(result.kind).toBe('answer');
    expect(result.citations).toHaveLength(1);
    expect(result.citations[0]?.citationPath).toBe('Dieu 7 Khoan 1');
    expect(result.iterations).toBe(1);
  });

  it('returns abstain when model says ABSTAIN', async () => {
    const result = await runAgentLoop({
      question: 'What about quantum computing rules?',
      mcp: fakeMcp([SECTION]),
      config: testConfig(),
      modelCall: fakeModelCall('ABSTAIN'),
    });
    expect(result.kind).toBe('abstain');
  });

  it('respects iteration cap', async () => {
    let calls = 0;
    const modelCall: ModelCall = async () => {
      calls++;
      return 'NEED_MORE_EVIDENCE';
    };
    await runAgentLoop({
      question: 'Complex question',
      mcp: fakeMcp([SECTION]),
      config: testConfig(),
      modelCall,
    });
    expect(calls).toBe(3);
  });

  it('cancels on AbortSignal', async () => {
    const controller = new AbortController();
    controller.abort();
    const result = await runAgentLoop({
      question: 'test',
      mcp: fakeMcp([SECTION]),
      config: testConfig(),
      modelCall: fakeModelCall('NEED_MORE_EVIDENCE'),
      signal: controller.signal,
    });
    expect(result.iterations).toBe(1);
  });
});
