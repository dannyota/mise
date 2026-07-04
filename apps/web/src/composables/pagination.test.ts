import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { usePagination } from './usePagination.js';

describe('usePagination', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });
  afterEach(() => vi.restoreAllMocks());

  it('loads first page', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({ items: [{ id: '1' }, { id: '2' }], cursor: 'abc' }),
      ),
    );
    const { items, hasMore, loadMore } = usePagination('/test');
    await loadMore();
    expect(items.value).toHaveLength(2);
    expect(hasMore.value).toBe(true);
  });

  it('stops when no cursor', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ items: [{ id: '1' }], cursor: null })),
    );
    const { hasMore, loadMore } = usePagination('/test');
    await loadMore();
    expect(hasMore.value).toBe(false);
  });

  it('resets state', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ items: [{ id: '1' }], cursor: null })),
    );
    const { items, hasMore, loadMore, reset } = usePagination('/test');
    await loadMore();
    reset();
    expect(items.value).toHaveLength(0);
    expect(hasMore.value).toBe(true);
  });
});
