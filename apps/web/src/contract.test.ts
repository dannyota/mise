import { describe, expect, it } from 'vitest';
import type {
  BootstrapResponse,
  DashboardSummary,
  Finding,
  CursorPage,
  ApiProblem,
  GraphResponse,
} from '@mise/contract';

describe('contract types compile', () => {
  it('BootstrapResponse shape', () => {
    const b: BootstrapResponse = {
      tier: 'mise_public',
      capabilities: {
        translate_allowed: true,
        admin_allowed: false,
      },
    };
    expect(b.tier).toBe('mise_public');
  });

  it('DashboardSummary shape', () => {
    const d: DashboardSummary = {
      coverage_pct: 85.2,
      open_conflicts: 3,
      staleness_alerts: 1,
      review_queue_depth: 12,
      corpora: [
        {
          corpus_id: 'vn-reg',
          name: 'VN Regulation',
          status: 'healthy',
          last_ingest: '2026-07-01T00:00:00Z',
          document_count: 42,
        },
      ],
    };
    expect(d.coverage_pct).toBe(85.2);
  });

  it('CursorPage wraps Finding', () => {
    const page: CursorPage<Finding> = {
      items: [
        {
          id: 'f1',
          kind: 'gap',
          severity: 'medium',
          status: 'open',
          corpus_id: 'vn-reg',
          document_id: 'd1',
          citation_path: 'Điều 7',
          description: 'gap',
          created_at: '2026-07-01T00:00:00Z',
        },
      ],
      cursor: null,
    };
    expect(page.items).toHaveLength(1);
  });

  it('GraphResponse shape', () => {
    const g: GraphResponse = {
      nodes: [
        {
          id: 'n1',
          corpus_id: 'vn-reg',
          document_id: 'd1',
          label: 'Law',
          tier: 'mise_public',
          node_type: 'regulation',
        },
      ],
      edges: [],
    };
    expect(g.nodes).toHaveLength(1);
  });

  it('ApiProblem shape', () => {
    const p: ApiProblem = {
      type: 'about:blank',
      title: 'Not Found',
      status: 404,
      detail: 'Resource not found',
    };
    expect(p.status).toBe(404);
  });
});
