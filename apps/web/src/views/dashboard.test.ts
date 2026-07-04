import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import DashboardView from './DashboardView.vue';
import type { DashboardSummary } from '@mise/contract';

const summary: DashboardSummary = {
  total_documents: 42,
  open_findings: 5,
  resolved_findings: 12,
  pending_reviews: 3,
  corpora: [
    {
      corpus_id: 'vn-law',
      status: 'ready',
      document_count: 30,
      last_ingest: '2026-07-01T00:00:00Z',
    },
  ],
};

describe('DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });
  afterEach(() => vi.restoreAllMocks());

  it('renders summary tiles after fetch', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(summary)),
    );
    const w = mount(DashboardView);
    await flushPromises();
    expect(w.text()).toContain('42');
    expect(w.text()).toContain('Total Documents');
    expect(w.text()).toContain('vn-law');
    expect(w.text()).toContain('ready');
  });

  it('renders error on failure', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('Network error'));
    const w = mount(DashboardView);
    await flushPromises();
    expect(w.text()).toContain('Network error');
  });
});
