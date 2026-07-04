import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useCapabilities } from './useCapabilities.js';
import { useTranslate } from './useTranslate.js';
import { useAuthStore } from '@/auth/store.js';

describe('useCapabilities', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });
  afterEach(() => vi.restoreAllMocks());

  it('refresh fetches bootstrap and updates store', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          tier: 'mise_internal',
          capabilities: {
            translate_allowed: false,
            admin_allowed: true,
          },
        }),
      ),
    );
    const caps = useCapabilities();
    await caps.refresh();
    expect(caps.tier.value).toBe('mise_internal');
    expect(caps.translateAllowed.value).toBe(false);
    expect(caps.adminAllowed.value).toBe(true);
    expect(caps.loaded.value).toBe(true);
  });
});

describe('useTranslate', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });
  afterEach(() => vi.restoreAllMocks());

  it('translates when allowed', async () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: true,
      admin_allowed: false,
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          translated_text: 'hello',
          source_lang: 'vi',
          target_lang: 'en',
        }),
      ),
    );
    const { translate } = useTranslate();
    const result = await translate('xin chào', 'vi', 'en');
    expect(result).toBe('hello');
  });

  it('throws when not allowed', async () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: false,
      admin_allowed: false,
    });
    const { translate } = useTranslate();
    await expect(translate('text', 'vi', 'en')).rejects.toThrow('Translation not permitted');
  });

  it('caches results', async () => {
    const store = useAuthStore();
    store.setCapabilities({
      translate_allowed: true,
      admin_allowed: false,
    });
    const spy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          translated_text: 'hi',
          source_lang: 'vi',
          target_lang: 'en',
        }),
      ),
    );
    const { translate } = useTranslate();
    await translate('a', 'vi', 'en');
    await translate('a', 'vi', 'en');
    expect(spy).toHaveBeenCalledTimes(1);
  });
});
