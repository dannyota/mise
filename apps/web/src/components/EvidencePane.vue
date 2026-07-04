<script setup lang="ts">
import { ref } from 'vue';
import TranslateToggle from './TranslateToggle.vue';
import { ExternalLink } from 'lucide-vue-next';

const props = defineProps<{
  sourceText: string;
  sourceCitation: string;
  sourceUrl: string;
  targetText?: string;
  targetCitation?: string;
  targetUrl?: string;
  sourceLang?: string;
  targetLang?: string;
  validityStatus?: string;
}>();

const translatedSource = ref<string | null>(null);
const translatedTarget = ref<string | null>(null);
</script>

<template>
  <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
    <div class="rounded border p-4">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs font-medium text-gray-500">
          {{ props.sourceCitation }}
        </span>
        <div class="flex items-center gap-2">
          <span
            v-if="props.validityStatus"
            class="rounded px-1.5 py-0.5 text-xs"
            :class="{
              'bg-green-100 text-green-700':
                props.validityStatus === 'in_force',
              'bg-red-100 text-red-700': props.validityStatus === 'repealed',
            }"
          >
            {{ props.validityStatus }}
          </span>
          <a
            :href="props.sourceUrl"
            target="_blank"
            rel="noopener"
            class="text-gray-400 hover:text-gray-600"
          >
            <ExternalLink :size="14" />
          </a>
        </div>
      </div>
      <p class="whitespace-pre-wrap text-sm">
        {{ translatedSource ?? props.sourceText }}
      </p>
      <TranslateToggle
        v-if="props.sourceLang && props.targetLang"
        :text="props.sourceText"
        :source-lang="props.sourceLang"
        :target-lang="props.targetLang ?? 'en'"
        class="mt-2"
        @translated="translatedSource = $event"
      />
    </div>
    <div v-if="props.targetText" class="rounded border p-4">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-xs font-medium text-gray-500">
          {{ props.targetCitation }}
        </span>
        <a
          v-if="props.targetUrl"
          :href="props.targetUrl"
          target="_blank"
          rel="noopener"
          class="text-gray-400 hover:text-gray-600"
        >
          <ExternalLink :size="14" />
        </a>
      </div>
      <p class="whitespace-pre-wrap text-sm">
        {{ translatedTarget ?? props.targetText }}
      </p>
      <TranslateToggle
        v-if="props.sourceLang && props.targetLang"
        :text="props.targetText"
        :source-lang="props.targetLang ?? 'en'"
        :target-lang="props.sourceLang"
        class="mt-2"
        @translated="translatedTarget = $event"
      />
    </div>
  </div>
</template>
