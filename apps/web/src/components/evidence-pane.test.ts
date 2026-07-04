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
        sourceText: 'The bank shall maintain...',
        sourceCitation: 'Điều 7 Khoản 1',
        sourceUrl: 'https://vbpl.vn/law/123',
      },
    });
    expect(w.text()).toContain('The bank shall maintain');
    expect(w.text()).toContain('Điều 7 Khoản 1');
  });

  it('renders side-by-side when target is provided', () => {
    const w = mount(EvidencePane, {
      props: {
        sourceText: 'Source law text',
        sourceCitation: 'Article 7',
        sourceUrl: 'https://example.com/1',
        targetText: 'Target policy text',
        targetCitation: 'POL-001 §3',
        targetUrl: 'https://example.com/2',
      },
    });
    expect(w.text()).toContain('Source law text');
    expect(w.text()).toContain('Target policy text');
    expect(w.text()).toContain('POL-001 §3');
  });

  it('shows validity badge when provided', () => {
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
});
