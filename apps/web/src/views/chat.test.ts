import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import ChatInput from '@/components/ChatInput.vue';
import CitationCard from '@/components/CitationCard.vue';

describe('ChatInput', () => {
  it('emits submit on form submit', async () => {
    const w = mount(ChatInput);
    const input = w.find('input');
    await input.setValue('What is RMiT?');
    await w.find('form').trigger('submit.prevent');
    expect(w.emitted('submit')?.[0]).toEqual(['What is RMiT?']);
  });

  it('does not emit on empty input', async () => {
    const w = mount(ChatInput);
    await w.find('form').trigger('submit.prevent');
    expect(w.emitted('submit')).toBeUndefined();
  });
});

describe('CitationCard', () => {
  it('renders citation text', () => {
    const w = mount(CitationCard, {
      props: { citation: 'Law 59/2010, Art.10' },
    });
    expect(w.text()).toContain('Law 59/2010, Art.10');
  });
});
