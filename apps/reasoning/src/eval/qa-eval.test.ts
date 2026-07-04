import { describe, expect, it } from 'vitest';
import {
  evaluateCase,
  abstentionAccuracy,
  citationCorrectness,
} from './qa-eval.js';
import type { QaGoldenCase, QaEvalResult } from './qa-eval.js';
import type { AgentResult } from '../agent/loop.js';

function answerResult(citationPath: string): AgentResult {
  return {
    kind: 'answer',
    text: 'answer text',
    citations: [
      {
        corpusId: 'vn-reg',
        documentId: 'test',
        citationPath,
        sourceUrl: 'https://example.com',
        text: 'cited text',
      },
    ],
    chain: [],
    model: 'test',
    iterations: 1,
  };
}

function abstainResult(): AgentResult {
  return {
    kind: 'abstain',
    text: 'Insufficient evidence.',
    citations: [],
    chain: [],
    model: 'test',
    iterations: 1,
  };
}

describe('evaluateCase', () => {
  it('correct answer with matching citation', () => {
    const golden: QaGoldenCase = {
      question: 'test',
      expected_kind: 'answer',
      expected_corpus: 'vn-reg',
      expected_citation_contains: 'Điều 7',
      notes: 'test',
    };
    const result = evaluateCase(golden, answerResult('Điều 7 Khoản 1'));
    expect(result.abstentionCorrect).toBe(true);
    expect(result.citationCorrect).toBe(true);
  });

  it('correct abstain', () => {
    const golden: QaGoldenCase = {
      question: 'test',
      expected_kind: 'abstain',
      expected_corpus: null,
      expected_citation_contains: null,
      notes: 'test',
    };
    const result = evaluateCase(golden, abstainResult());
    expect(result.abstentionCorrect).toBe(true);
  });

  it('wrong: answered when should abstain', () => {
    const golden: QaGoldenCase = {
      question: 'test',
      expected_kind: 'abstain',
      expected_corpus: null,
      expected_citation_contains: null,
      notes: 'test',
    };
    const result = evaluateCase(golden, answerResult('Dieu 1'));
    expect(result.abstentionCorrect).toBe(false);
  });
});

describe('metrics', () => {
  it('abstentionAccuracy computes correctly', () => {
    const r: QaEvalResult = {
      total: 5,
      abstentionCorrect: 4,
      abstentionTotal: 5,
      citationCorrect: 3,
      citationTotal: 3,
    };
    expect(abstentionAccuracy(r)).toBe(0.8);
  });

  it('citationCorrectness returns 1 for zero denominator', () => {
    const r: QaEvalResult = {
      total: 2,
      abstentionCorrect: 2,
      abstentionTotal: 2,
      citationCorrect: 0,
      citationTotal: 0,
    };
    expect(citationCorrectness(r)).toBe(1);
  });
});
