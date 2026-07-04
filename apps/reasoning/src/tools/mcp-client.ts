import type { Caller } from '../auth/caller.js';
import type { Config } from '../config.js';

export type SearchParams = {
  readonly query: string;
  readonly corpora?: readonly string[];
  readonly topK?: number;
  readonly inForceOnly?: boolean;
};

export type SearchResult = {
  readonly sections: readonly {
    readonly corpus_id: string;
    readonly document_id: string;
    readonly section_id: string;
    readonly citation_path: string;
    readonly text: string;
    readonly validity_status: string;
    readonly score: number;
    readonly source_url: string;
  }[];
};

export type DocumentParams = {
  readonly corpusId: string;
  readonly documentId: string;
};

export type DocumentResult = {
  readonly document: {
    readonly title: string;
    readonly source_url: string;
    readonly citation_path: string;
    readonly validity_status: string;
  };
  readonly sections: readonly {
    readonly citation_path: string;
    readonly text: string;
    readonly validity_status: string;
  }[];
};

export type GraphParams = {
  readonly nodeRef: string;
  readonly depth?: number;
  readonly edgeTypes?: readonly string[];
};

export type GraphResult = {
  readonly chain: readonly {
    readonly corpus_id: string;
    readonly document_id: string;
    readonly edge_type: string;
    readonly citation: string;
    readonly confidence: number;
  }[];
};

export type McpClient = {
  search(params: SearchParams): Promise<SearchResult>;
  document(params: DocumentParams): Promise<DocumentResult>;
  graph(params: GraphParams): Promise<GraphResult>;
};

export function createMcpClient(config: Config, caller: Caller): McpClient {
  const baseUrl = config.MCP_URL;
  const headers = {
    'X-Caller-Sub': caller.sub,
    'X-Caller-Tier': caller.tier,
    'X-Correlation-Id': caller.correlationId,
    'Content-Type': 'application/json',
  };

  async function callTool<T>(
    toolName: string,
    args: Record<string, unknown>,
  ): Promise<T> {
    const res = await fetch(baseUrl, {
      method: 'POST',
      headers,
      body: JSON.stringify({
        jsonrpc: '2.0',
        method: 'tools/call',
        params: { name: toolName, arguments: args },
        id: crypto.randomUUID(),
      }),
    });
    if (!res.ok) {
      throw new Error(
        `MCP ${toolName} failed: ${res.status} ${res.statusText}`,
      );
    }
    const body = (await res.json()) as { result?: { output?: T } };
    if (!body.result?.output) {
      throw new Error(`MCP ${toolName}: no output in response`);
    }
    return body.result.output;
  }

  return {
    async search(params) {
      return callTool<SearchResult>('search', {
        query: params.query,
        corpora: params.corpora,
        top_k: params.topK,
        in_force_only: params.inForceOnly,
      });
    },
    async document(params) {
      return callTool<DocumentResult>('document', {
        corpus_id: params.corpusId,
        document_id: params.documentId,
      });
    },
    async graph(params) {
      return callTool<GraphResult>('graph', {
        node_ref: params.nodeRef,
        depth: params.depth,
        edge_types: params.edgeTypes,
      });
    },
  };
}
