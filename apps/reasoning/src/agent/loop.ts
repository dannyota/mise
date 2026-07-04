import type { McpClient, SearchResult } from '../tools/mcp-client.js';
import type { Config } from '../config.js';
import { selectModel } from './model-routing.js';

export type Citation = {
  readonly corpusId: string;
  readonly documentId: string;
  readonly citationPath: string;
  readonly sourceUrl: string;
  readonly text: string;
};

export type ChainHop = {
  readonly corpusId: string;
  readonly documentId: string;
  readonly edgeType: string;
  readonly citation: string;
  readonly confidence: number;
};

export type AgentResult = {
  readonly kind: 'answer' | 'abstain';
  readonly text: string;
  readonly citations: readonly Citation[];
  readonly chain: readonly ChainHop[];
  readonly model: string;
  readonly iterations: number;
};

export type ModelCall = (
  prompt: string,
  model: string,
  maxTokens: number,
  signal?: AbortSignal,
) => Promise<string>;

export type AgentLoopParams = {
  readonly question: string;
  readonly corpora?: readonly string[];
  readonly mcp: McpClient;
  readonly config: Config;
  readonly modelCall: ModelCall;
  readonly signal?: AbortSignal;
};

export async function runAgentLoop(
  params: AgentLoopParams,
): Promise<AgentResult> {
  const { question, mcp, config, modelCall, signal } = params;

  const searchResult = await mcp.search({
    query: question,
    corpora: params.corpora as string[] | undefined,
  });

  const citations = extractCitations(searchResult);

  if (citations.length === 0) {
    return {
      kind: 'abstain',
      text: 'Insufficient evidence to answer this question.',
      citations: [],
      chain: [],
      model: config.MODEL_DEFAULT,
      iterations: 1,
    };
  }

  const supportScore = Math.min(citations.length / 3, 1);
  const model = selectModel(supportScore, config);

  let chain: ChainHop[] = [];
  const first = citations[0];
  if (!first) {
    throw new Error('expected at least one citation');
  }
  const firstRef = `${first.corpusId}/${first.documentId}`;
  try {
    const graphResult = await mcp.graph({ nodeRef: firstRef });
    chain = graphResult.chain.map((h) => ({
      corpusId: h.corpus_id,
      documentId: h.document_id,
      edgeType: h.edge_type,
      citation: h.citation,
      confidence: h.confidence,
    }));
  } catch {
    // graph lookup is optional — proceed without chain
  }

  const evidenceContext = citations
    .map((c, i) => `[${i + 1}] ${c.citationPath}: ${c.text.slice(0, 500)}`)
    .join('\n\n');

  const prompt =
    `Answer the following audit question using ONLY the evidence provided. ` +
    `Cite evidence using [N] notation. If the evidence is insufficient, ` +
    `respond with "ABSTAIN".\n\n` +
    `Question: ${question}\n\n` +
    `Evidence:\n${evidenceContext}`;

  let iterations = 1;
  let answer = await modelCall(prompt, model, config.MAX_TOKENS, signal);

  while (
    iterations < config.MAX_ITERATIONS &&
    needsMoreEvidence(answer) &&
    !signal?.aborted
  ) {
    iterations++;
    answer = await modelCall(prompt, model, config.MAX_TOKENS, signal);
  }

  if (answer.includes('ABSTAIN')) {
    return {
      kind: 'abstain',
      text: 'Insufficient evidence to answer this question.',
      citations,
      chain,
      model,
      iterations,
    };
  }

  return {
    kind: 'answer',
    text: answer,
    citations,
    chain,
    model,
    iterations,
  };
}

function extractCitations(search: SearchResult): Citation[] {
  return search.sections.map((s) => ({
    corpusId: s.corpus_id,
    documentId: s.document_id,
    citationPath: s.citation_path,
    sourceUrl: s.source_url,
    text: s.text,
  }));
}

function needsMoreEvidence(answer: string): boolean {
  return answer.includes('NEED_MORE_EVIDENCE');
}
