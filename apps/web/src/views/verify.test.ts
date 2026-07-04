import { beforeEach, describe, expect, it } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useAuthStore } from '@/auth/store.js';
import { useCapabilities } from '@/composables/useCapabilities.js';

describe('tier-based visibility', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('hides translate when not allowed', () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: false,
      admin_allowed: false,
    });
    const { translateAllowed } = useCapabilities();
    expect(translateAllowed.value).toBe(false);
  });

  it('hides admin when not allowed', () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: false,
      admin_allowed: false,
    });
    const { adminAllowed } = useCapabilities();
    expect(adminAllowed.value).toBe(false);
  });

  it('shows admin when allowed', () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: false,
      admin_allowed: true,
    });
    const { adminAllowed } = useCapabilities();
    expect(adminAllowed.value).toBe(true);
  });
});

describe('no-model-client gate', () => {
  it('ESLint no-restricted-imports blocks model SDKs', async () => {
    const fs = await import('node:fs');
    const path = await import('node:path');
    const root = path.resolve(process.cwd(), 'eslint.config.js');
    const config = fs.readFileSync(root, 'utf8');
    expect(config).toContain('@anthropic-ai/*');
    expect(config).toContain('openai');
    expect(config).toContain('claude-agent-sdk');
  });
});
