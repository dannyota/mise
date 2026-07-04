<script setup lang="ts">
import { useChat } from '@/composables/useChat.js';
import ChatInput from '@/components/ChatInput.vue';
import ChatStream from '@/components/ChatStream.vue';
import ErrorBanner from '@/components/ErrorBanner.vue';

const { messages, streaming, error, send } = useChat();
</script>

<template>
  <div class="flex h-full flex-col">
    <h2 class="mb-4 text-xl font-semibold">Audit Q&amp;A</h2>
    <ErrorBanner v-if="error" :error="new Error(error)" class="mb-4" />
    <div class="flex-1 overflow-y-auto">
      <ChatStream :messages="messages" />
    </div>
    <div class="mt-4">
      <ChatInput :disabled="streaming" @submit="send" />
    </div>
  </div>
</template>
