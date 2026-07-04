import { beforeEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import EvidencePane from './EvidencePane.vue';

describe('EvidencePane', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('renders source text and citation', () => {
    const w = mount(EvidencePane, {
      props: {
        sourceText: 'Article 10 provision',
        sourceCitation: 'Law 59/2010, Art.10',
        sourceUrl: 'https://example.com',
      },
    });
    expect(w.text()).toContain('Article 10 provision');
    expect(w.text()).toContain('Law 59/2010, Art.10');
  });

  it('shows validity status badge', () => {
    const w = mount(EvidencePane, {
      props: {
        sourceText: 'text',
        sourceCitation: 'cite',
        sourceUrl: 'https://example.com',
        validityStatus: 'in_force',
      },
    });
    expect(w.text()).toContain('in_force');
  });

  it('renders target text when provided', () => {
    const w = mount(EvidencePane, {
      props: {
        sourceText: 'source',
        sourceCitation: 'cite-s',
        sourceUrl: 'https://example.com/s',
        targetText: 'target',
        targetCitation: 'cite-t',
        targetUrl: 'https://example.com/t',
      },
    });
    expect(w.text()).toContain('target');
    expect(w.text()).toContain('cite-t');
  });
});
