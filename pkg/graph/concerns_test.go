package graph_test

import (
	"testing"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

func TestReportsCorpusConcernsEdge(t *testing.T) {
	edgeType, ok := graph.EdgeTypeForPair(corpus.Reports, corpus.VNReg)
	if !ok {
		t.Fatal("EdgeTypeForPair(reports, vn-reg) should find an edge type")
	}
	if edgeType != "concerns" {
		t.Errorf("edge type = %q, want concerns", edgeType)
	}
}

func TestReportsCorpusConcernsEdgeMY(t *testing.T) {
	edgeType, ok := graph.EdgeTypeForPair(corpus.Reports, corpus.MYReg)
	if !ok {
		t.Fatal("EdgeTypeForPair(reports, my-reg) should find an edge type")
	}
	if edgeType != "concerns" {
		t.Errorf("edge type = %q, want concerns", edgeType)
	}
}
