import { afterEach, describe, expect, it } from 'vitest';
import { loadConfig } from './config.js';

const ENV_KEYS = [
  'PORT',
  'SERVING_URL',
  'NODE_ENV',
  'MODEL_DEFAULT',
  'MODEL_ESCALATION',
  'OIDC_ISSUER',
  'OIDC_AUDIENCE',
  'MCP_URL',
  'ABSTAIN_THRESHOLD',
  'ESCALATION_THRESHOLD',
  'MAX_ITERATIONS',
  'MAX_TOKENS',
] as const;

function clearEnv(): void {
  for (const key of ENV_KEYS) {
    Reflect.deleteProperty(process.env, key);
  }
}

describe('loadConfig', () => {
  afterEach(() => {
    clearEnv();
  });

  it('applies defaults when env vars are unset', () => {
    clearEnv();
    const config = loadConfig();

    expect(config.PORT).toBe(3001);
    expect(config.MODEL_DEFAULT).toBe('claude-haiku-4-5-20251001');
    expect(config.MODEL_ESCALATION).toBe('claude-sonnet-4-6-20250514');
    expect(config.MCP_URL).toBe('http://localhost:8080/mcp');
    expect(config.ABSTAIN_THRESHOLD).toBe(0.3);
    expect(config.ESCALATION_THRESHOLD).toBe(0.5);
    expect(config.MAX_ITERATIONS).toBe(5);
    expect(config.MAX_TOKENS).toBe(4096);
  });

  it('coerces PORT from a string env var to a number', () => {
    clearEnv();
    process.env.PORT = '4000';
    expect(loadConfig().PORT).toBe(4000);
  });

  it('accepts OIDC_ISSUER as a valid URL', () => {
    clearEnv();
    process.env.OIDC_ISSUER = 'https://auth.example.com';
    process.env.OIDC_AUDIENCE = 'mise-reasoning';
    const config = loadConfig();
    expect(config.OIDC_ISSUER).toBe('https://auth.example.com');
    expect(config.OIDC_AUDIENCE).toBe('mise-reasoning');
  });

  it('rejects OIDC_ISSUER with a non-URL value', () => {
    clearEnv();
    process.env.OIDC_ISSUER = 'not-a-url';
    expect(() => loadConfig()).toThrow();
  });

  it('coerces threshold from string to number', () => {
    clearEnv();
    process.env.ABSTAIN_THRESHOLD = '0.7';
    expect(loadConfig().ABSTAIN_THRESHOLD).toBe(0.7);
  });

  it('rejects threshold > 1', () => {
    clearEnv();
    process.env.ABSTAIN_THRESHOLD = '1.5';
    expect(() => loadConfig()).toThrow();
  });

  it('reads model overrides', () => {
    clearEnv();
    process.env.MODEL_DEFAULT = 'custom-model';
    process.env.MODEL_ESCALATION = 'custom-escalation';
    const config = loadConfig();
    expect(config.MODEL_DEFAULT).toBe('custom-model');
    expect(config.MODEL_ESCALATION).toBe('custom-escalation');
  });

  it('rejects an invalid NODE_ENV', () => {
    clearEnv();
    process.env.NODE_ENV = 'bogus';
    expect(() => loadConfig()).toThrow();
  });
});
