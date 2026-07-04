export type ApiProblem = {
  readonly type: string;
  readonly title: string;
  readonly status: number;
  readonly detail: string;
};

export type BootstrapResponse = {
  readonly tier: string;
  readonly capabilities: {
    readonly translate_allowed: boolean;
    readonly admin_allowed: boolean;
  };
};

export type CorpusStatus = {
  readonly corpus_id: string;
  readonly name: string;
  readonly status: 'healthy' | 'ingesting' | 'error';
  readonly last_ingest: string;
  readonly document_count: number;
};

export type DashboardSummary = {
  readonly coverage_pct: number;
  readonly open_conflicts: number;
  readonly staleness_alerts: number;
  readonly review_queue_depth: number;
  readonly corpora: readonly CorpusStatus[];
};

export type CursorPage<T> = {
  readonly items: readonly T[];
  readonly cursor: string | null;
};

export type ReviewCandidate = {
  readonly edge_id: string;
  readonly source_corpus_id: string;
  readonly source_document_id: string;
  readonly source_citation: string;
  readonly source_text: string;
  readonly target_corpus_id: string;
  readonly target_document_id: string;
  readonly target_citation: string;
  readonly target_text: string;
  readonly edge_type: string;
  readonly confidence: number;
  readonly grounding_score: number;
  readonly status: 'pending' | 'promoted' | 'rejected';
};

export type Finding = {
  readonly id: string;
  readonly kind: 'gap' | 'conflict' | 'staleness';
  readonly severity: 'critical' | 'high' | 'medium' | 'low';
  readonly status: 'open' | 'in_progress' | 'in_review' | 'closed';
  readonly corpus_id: string;
  readonly document_id: string;
  readonly citation_path: string;
  readonly description: string;
  readonly created_at: string;
};

export type FindingDetail = Finding & {
  readonly source_text: string;
  readonly target_text: string;
  readonly target_citation_path: string;
  readonly resolutions: readonly Resolution[];
};

export type Resolution = {
  readonly id: string;
  readonly finding_id: string;
  readonly disposition: 'map' | 'document' | 'accept' | 'escalate';
  readonly owner_department: string;
  readonly owner_role: string;
  readonly status: 'open' | 'in_progress' | 'in_review' | 'closed';
  readonly due_date: string | null;
  readonly notes: string;
  readonly created_at: string;
  readonly updated_at: string;
};

export type TimelineEvent = {
  readonly id: string;
  readonly kind: 'amendment' | 'detection' | 'review' | 'resolution';
  readonly corpus_id: string;
  readonly document_id: string | null;
  readonly description: string;
  readonly timestamp: string;
};

export type Notification = {
  readonly id: string;
  readonly type: 'conflict' | 'staleness' | 'overdue';
  readonly title: string;
  readonly finding_id: string;
  readonly read: boolean;
  readonly created_at: string;
};

export type Webhook = {
  readonly id: string;
  readonly url: string;
  readonly events: readonly string[];
  readonly active: boolean;
  readonly created_at: string;
};

export type TranslateResponse = {
  readonly translated_text: string;
  readonly source_lang: string;
  readonly target_lang: string;
};

export type CorpusAdmin = {
  readonly corpus_id: string;
  readonly name: string;
  readonly source_type: string;
  readonly status: 'healthy' | 'ingesting' | 'error';
  readonly last_ingest: string;
  readonly workflow_id: string | null;
  readonly document_count: number;
  readonly error_message: string | null;
};

export type RestGraphNode = {
  readonly id: string;
  readonly corpus_id: string;
  readonly document_id: string;
  readonly label: string;
  readonly tier: string;
  readonly node_type: string;
};

export type RestGraphEdge = {
  readonly id: string;
  readonly source: string;
  readonly target: string;
  readonly edge_type: string;
  readonly confidence: number;
  readonly grounding_score: number;
  readonly promoted: boolean;
};

export type GraphResponse = {
  readonly nodes: readonly RestGraphNode[];
  readonly edges: readonly RestGraphEdge[];
};

export type ChainHop = {
  readonly corpus_id: string;
  readonly document_id: string;
  readonly citation: string;
  readonly edge_type: string;
  readonly confidence: number;
};

export type ChainResponse = {
  readonly hops: readonly ChainHop[];
};

export type CorpusDescriptor = {
  readonly id: string;
  readonly kind: string;
  readonly schema_name: string;
  readonly citation_scheme: string;
  readonly access_tier: string;
  readonly tier?: string;
  readonly jurisdiction: string;
  readonly embed_model: string;
  readonly embed_dims: number;
  readonly can_source: boolean;
  readonly can_target: boolean;
};

export type RegistryListResponse = {
  readonly items: readonly CorpusDescriptor[];
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
  readonly image_ref?: string;
};

export type SearchResponse = {
  readonly sections: readonly SectionHit[];
};

export type DocumentEnvelope = {
  readonly id: string;
  readonly corpus_id: string;
  readonly title: string;
  readonly doc_number: string;
  readonly citation_scheme: string;
  readonly citation_path: string;
  readonly language: string;
  readonly validity_status: string;
  readonly issuing_authority: string;
  readonly signer_name: string;
  readonly version: string;
  readonly source_url: string;
  readonly source_system: string;
  readonly content_type: string;
  readonly access_tier: string;
  readonly issued_date?: string;
  readonly effective_date?: string;
  readonly expiry_date?: string;
  readonly ingest_run_id: string;
  readonly observed_at: string;
};

export type DocSection = {
  readonly id: string;
  readonly citation_path: string;
  readonly heading_path: string;
  readonly text: string;
  readonly validity_status: string;
  readonly position: number;
  readonly image_ref?: string;
};

export type Amendment = {
  readonly amending_doc_id?: string;
  readonly kind?: string;
  readonly clause: string;
  readonly event_date: string;
};

export type DocumentDetailResponse = {
  readonly document: DocumentEnvelope;
  readonly sections: readonly DocSection[];
  readonly amendments: readonly Amendment[];
};

export type IngestTriggerResponse = {
  readonly workflow_id: string;
};

export type CoverageGap = {
  readonly corpus_id: string;
  readonly document_id: string;
  readonly citation: string;
  readonly gap_type: string;
};

export type CoverageChain = {
  readonly law_ref: string;
  readonly policy_ref: string;
  readonly sop_ref: string;
};

export type CoverageReport = {
  readonly coverage_pct: number;
  readonly gaps: readonly CoverageGap[];
  readonly chains: readonly CoverageChain[];
  readonly generated_at: string;
};
