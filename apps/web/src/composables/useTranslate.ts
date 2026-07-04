import { ref, computed } from 'vue';
import { useAuthStore } from '@/auth/store.js';
import { apiPost } from '@/api/client.js';
import type { TranslateResponse } from '@mise/contract';

const cache = new Map<string, string>();

export function useTranslate() {
  const store = useAuthStore();
  const isTranslating = ref(false);
  const canTranslate = computed(() => store.capabilities.translate_allowed);

  async function translate(
    text: string,
    sourceLang: string,
    targetLang: string,
  ): Promise<string> {
    if (!canTranslate.value) {
      throw new Error('Translation not permitted');
    }
    const key = `${sourceLang}:${targetLang}:${text}`;
    const cached = cache.get(key);
    if (cached) return cached;

    isTranslating.value = true;
    try {
      const res = await apiPost<TranslateResponse>('/translate', {
        text,
        source_lang: sourceLang,
        target_lang: targetLang,
      });
      cache.set(key, res.translated_text);
      return res.translated_text;
    } finally {
      isTranslating.value = false;
    }
  }

  return { translate, isTranslating, canTranslate };
}
