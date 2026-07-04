import { ref } from 'vue';
import { parseSseStream } from '@/api/sse.js';
import { useAuthStore } from '@/auth/store.js';

export type ChatMessage = {
  role: 'user' | 'assistant';
  content: string;
  citations?: string[];
};

export function useChat() {
  const messages = ref<ChatMessage[]>([]);
  const streaming = ref(false);
  const error = ref<string | null>(null);

  async function send(question: string): Promise<void> {
    messages.value.push({ role: 'user', content: question });
    const assistant: ChatMessage = {
      role: 'assistant',
      content: '',
      citations: [],
    };
    messages.value.push(assistant);
    streaming.value = true;
    error.value = null;

    const store = useAuthStore();
    const base = import.meta.env.VITE_REASONING_URL ?? '/reasoning';

    try {
      const res = await fetch(`${base}/chat`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(store.token ? { Authorization: `Bearer ${store.token}` } : {}),
        },
        body: JSON.stringify({ question }),
      });

      if (!res.ok) {
        throw new Error(`Chat failed: ${res.status}`);
      }

      for await (const evt of parseSseStream(res)) {
        if (evt.event === 'token') {
          assistant.content += evt.data;
        } else if (evt.event === 'citation') {
          assistant.citations?.push(evt.data);
        }
      }
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      streaming.value = false;
    }
  }

  function clear(): void {
    messages.value = [];
    error.value = null;
  }

  return { messages, streaming, error, send, clear };
}
