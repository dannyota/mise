<script setup lang="ts">
import { ref } from 'vue';
import { useTranslate } from '@/composables/useTranslate.js';
import { Languages } from 'lucide-vue-next';

const props = defineProps<{
  text: string;
  sourceLang: string;
  targetLang: string;
}>();

const emit = defineEmits<{
  translated: [text: string];
}>();

const { translate, isTranslating, canTranslate } = useTranslate();
const translated = ref(false);

async function handleToggle(): Promise<void> {
  if (!canTranslate.value || translated.value) return;
  const result = await translate(props.text, props.sourceLang, props.targetLang);
  translated.value = true;
  emit('translated', result);
}
</script>

<template>
  <div>
    <button
      v-if="canTranslate.value"
      :disabled="isTranslating.value || translated.value"
      class="flex items-center gap-1 rounded px-2 py-1 text-xs hover:bg-gray-100 disabled:opacity-50"
      @click="handleToggle"
    >
      <Languages :size="14" />
      {{ translated ? 'Translated' : 'Translate' }}
    </button>
    <span v-else class="text-xs text-gray-400" title="Translation not available for this tier">
      Translation restricted
    </span>
  </div>
</template>
