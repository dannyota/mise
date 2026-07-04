import type { AgentResult } from '../agent/loop.js';

export type QaGoldenCase = {
  readonly question: string;
  readonly expected_kind: 'answer' | 'abstain';
  readonly expected_corpus: string | null;
  readonly expected_citation_contains: string | null;
  readonly notes: string;
};

export type QaEvalResult = {
  readonly total: number;
  readonly abstentionCorrect: number;
  readonly abstentionTotal: number;
  readonly citationCorrect: number;
  readonly citationTotal: number;
};

export function evaluateCase(
  golden: QaGoldenCase,
  result: AgentResult,
): { abstentionCorrect: boolean; citationCorrect: boolean } {
  const abstentionCorrect = golden.expected_kind === result.kind;

  let citationCorrect = true;
  if (golden.expected_kind === 'answer' && result.kind === 'answer') {
    if (golden.expected_citation_contains) {
      citationCorrect = result.citations.some((c) =>
        c.citationPath.includes(golden.expected_citation_contains as string),
      );
    }
  }

  return { abstentionCorrect, citationCorrect };
}

export function abstentionAccuracy(result: QaEvalResult): number {
  if (result.abstentionTotal === 0) return 1;
  return result.abstentionCorrect / result.abstentionTotal;
}

export function citationCorrectness(result: QaEvalResult): number {
  if (result.citationTotal === 0) return 1;
  return result.citationCorrect / result.citationTotal;
}
