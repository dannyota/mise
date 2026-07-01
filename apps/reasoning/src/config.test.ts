import { afterEach, describe, expect, it } from 'vitest';
import { loadConfig } from './config.js';

const ENV_KEYS = ['PORT', 'SERVING_URL', 'NODE_ENV'] as const;

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
    expect(config.SERVING_URL).toBe('http://localhost:8080');
    expect(config.NODE_ENV).toBe('development');
  });

  it('coerces PORT from a string env var to a number', () => {
    clearEnv();
    process.env.PORT = '4000';

    const config = loadConfig();

    expect(config.PORT).toBe(4000);
    expect(typeof config.PORT).toBe('number');
  });

  it('reads SERVING_URL and NODE_ENV from the environment', () => {
    clearEnv();
    process.env.SERVING_URL = 'http://serving.internal:9000';
    process.env.NODE_ENV = 'production';

    const config = loadConfig();

    expect(config.SERVING_URL).toBe('http://serving.internal:9000');
    expect(config.NODE_ENV).toBe('production');
  });

  it('rejects a non-numeric PORT', () => {
    clearEnv();
    process.env.PORT = 'not-a-port';

    expect(() => loadConfig()).toThrow();
  });

  it('rejects an invalid NODE_ENV', () => {
    clearEnv();
    process.env.NODE_ENV = 'bogus';

    expect(() => loadConfig()).toThrow();
  });
});
