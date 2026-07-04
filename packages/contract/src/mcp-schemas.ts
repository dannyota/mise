export type SearchInput = {
  readonly query: string;
  readonly corpora?: readonly string[];
  readonly top_k?: number;
  readonly as_of_date?: string;
  readonly in_force_only?: boolean;
};

export type SectionHit = {
  readonly corpus_id: string;
  readonly document_id: string;
  readonly section_id: string;
  readonly doc_number: string;
  readonly title: string;
  readonly citation_path: string;
  readonly heading_path: string;
  readonly text: string;
  readonly validity_status: string;
  readonly score: number;
  readonly source_url: string;
};

export type SearchOutput = {
  readonly sections: readonly SectionHit[];
};

export type DocumentInput = {
  readonly corpus_id: string;
  readonly document_id: string;
};

export type DocumentOutput = {
  readonly document: {
    readonly id: string;
    readonly corpus_id: string;
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

export type GraphInput = {
  readonly node_ref: string;
  readonly direction?: 'up' | 'down';
  readonly edge_types?: readonly string[];
  readonly depth?: number;
};

export type GraphOutput = {
  readonly nodes: readonly {
    readonly corpus_id: string;
    readonly document_id: string;
    readonly section_id?: string;
  }[];
  readonly edges: readonly {
    readonly id: string;
    readonly edge_type: string;
    readonly confidence: number;
    readonly grounding_score: number;
  }[];
  readonly chain: readonly {
    readonly corpus_id: string;
    readonly document_id: string;
    readonly edge_type: string;
    readonly citation: string;
    readonly confidence: number;
  }[];
};
