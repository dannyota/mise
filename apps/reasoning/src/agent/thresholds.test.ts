import { describe, expect, it } from 'vitest';
import { shouldAbstain, shouldEscalate } from './thresholds.js';
import type { Config } from '../config.js';

function testConfig(overrides: Partial<Config> = {}): Config {
  return {
    PORT: 3001,
    SERVING_URL: 'http://localhost:8080',
    NODE_ENV: 'test',
    MODEL_DEFAULT: 'test-haiku',
    MODEL_ESCALATION: 'test-sonnet',
    MCP_URL: 'http://localhost:8080/mcp',
    ABSTAIN_THRESHOLD: 0.3,
    ESCALATION_THRESHOLD: 0.5,
    MAX_ITERATIONS: 5,
    MAX_TOKENS: 4096,
    ...overrides,
  };
}

describe('shouldAbstain', () => {
  it('returns true when support is below threshold', () => {
    expect(shouldAbstain(0.2, testConfig())).toBe(true);
  });

  it('returns false when support is at threshold', () => {
    expect(shouldAbstain(0.3, testConfig())).toBe(false);
  });

  it('returns false when support is above threshold', () => {
    expect(shouldAbstain(0.8, testConfig())).toBe(false);
  });
});

describe('shouldEscalate', () => {
  it('returns true when support is below threshold', () => {
    expect(shouldEscalate(0.4, testConfig())).toBe(true);
  });

  it('returns false at threshold', () => {
    expect(shouldEscalate(0.5, testConfig())).toBe(false);
  });

  it('uses config override', () => {
    const config = testConfig({ ESCALATION_THRESHOLD: 0.8 });
    expect(shouldEscalate(0.7, config)).toBe(true);
    expect(shouldEscalate(0.8, config)).toBe(false);
  });
});
