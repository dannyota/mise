export type SseTokenEvent = {
  readonly event: 'token';
  readonly data: { readonly text: string };
};

export type SseCitationEvent = {
  readonly event: 'citation';
  readonly data: {
    readonly index: number;
    readonly corpus_id: string;
    readonly document_id: string;
    readonly citation_path: string;
    readonly source_url: string;
  };
};

export type SseChainEvent = {
  readonly event: 'chain';
  readonly data: {
    readonly corpusId: string;
    readonly documentId: string;
    readonly edgeType: string;
    readonly citation: string;
    readonly confidence: number;
  };
};

export type SseEvidenceCheckedEvent = {
  readonly event: 'evidence_checked';
  readonly data: {
    readonly corpus_id: string;
    readonly citation_path: string;
    readonly source_url: string;
  };
};

export type SseAbstainEvent = {
  readonly event: 'abstain';
  readonly data: { readonly reason: string };
};

export type SseDoneEvent = {
  readonly event: 'done';
  readonly data: {
    readonly model: string;
    readonly iterations: number;
  };
};

export type SseErrorEvent = {
  readonly event: 'error';
  readonly data: {
    readonly type: string;
    readonly detail: string;
  };
};

export type SseEvent =
  | SseTokenEvent
  | SseCitationEvent
  | SseChainEvent
  | SseEvidenceCheckedEvent
  | SseAbstainEvent
  | SseDoneEvent
  | SseErrorEvent;

export const SSE_EVENT_TYPES = [
  'token',
  'citation',
  'chain',
  'evidence_checked',
  'abstain',
  'done',
  'error',
] as const;
