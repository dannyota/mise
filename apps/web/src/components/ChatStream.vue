<script setup lang="ts">
import type { ChatMessage } from '@/composables/useChat.js';
import CitationCard from './CitationCard.vue';

defineProps<{ messages: ChatMessage[] }>();
</script>

<template>
  <div class="flex flex-col gap-4">
    <div
      v-for="(msg, i) in messages"
      :key="i"
      class="rounded p-3 text-sm"
      :class="msg.role === 'user' ? 'ml-8 bg-violet-50' : 'mr-8 bg-gray-50'"
    >
      <p class="whitespace-pre-wrap">{{ msg.content }}</p>
      <div v-if="msg.citations?.length" class="mt-2 flex flex-wrap gap-2">
        <CitationCard
          v-for="(cite, j) in msg.citations"
          :key="j"
          :citation="cite"
        />
      </div>
    </div>
  </div>
</template>
