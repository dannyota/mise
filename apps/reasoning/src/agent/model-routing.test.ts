import { describe, expect, it } from 'vitest';
import { selectModel } from './model-routing.js';
import type { Config } from '../config.js';

function testConfig(overrides: Partial<Config> = {}): Config {
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

describe('selectModel', () => {
  it('returns default model when support is above threshold', () => {
    expect(selectModel(0.8, testConfig())).toBe('claude-haiku-4-5-20251001');
  });

  it('returns escalation model when support is below threshold', () => {
    expect(selectModel(0.3, testConfig())).toBe('claude-sonnet-4-6-20250514');
  });

  it('returns default at exactly the threshold', () => {
    expect(selectModel(0.5, testConfig())).toBe('claude-haiku-4-5-20251001');
  });

  it('uses config overrides', () => {
    const config = testConfig({
      MODEL_DEFAULT: 'custom-default',
      MODEL_ESCALATION: 'custom-escalation',
      ESCALATION_THRESHOLD: 0.7,
    });
    expect(selectModel(0.6, config)).toBe('custom-escalation');
    expect(selectModel(0.7, config)).toBe('custom-default');
  });
});
