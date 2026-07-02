package graph_test

import (
	"slices"
	"testing"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/graph"
)

type parseControlRefsTestCase struct {
	name   string
	header graph.DocControlHeader
	want   []graph.RawControlRef
}

// parseControlRefsTests is a package-level table (rather than a local var
// inside TestParseControlRefs) so the test function body itself stays short;
// see each case's name for what it covers.
var parseControlRefsTests = []parseControlRefsTestCase{
	{
		name: "implements and derives refs with quoted spans are parsed",
		header: graph.DocControlHeader{
			Corpus:    corpus.LocalSOP,
			DocNumber: "SOP-114",
			ControlRefs: []graph.RawControlRef{
				{
					Relation:     "implements",
					TargetNumber: "POL-7",
					TargetTitle:  "Access Control Policy",
					QuotedSpan:   "This SOP implements POL-7 Access Control Policy.",
				},
				{
					Relation:    "derives",
					TargetTitle: "Group IT Security Standard",
					QuotedSpan:  "Derives from the Group IT Security Standard.",
				},
			},
		},
		want: []graph.RawControlRef{
			{
				Relation:     "implements",
				TargetNumber: "POL-7",
				TargetTitle:  "Access Control Policy",
				QuotedSpan:   "This SOP implements POL-7 Access Control Policy.",
			},
			{
				Relation:    "derives",
				TargetTitle: "Group IT Security Standard",
				QuotedSpan:  "Derives from the Group IT Security Standard.",
			},
		},
	},
	{
		name:   "absent header with zero control refs yields empty result, never a guess",
		header: graph.DocControlHeader{},
		want:   []graph.RawControlRef{},
	},
	{
		name: "ref with empty relation is dropped not guessed",
		header: graph.DocControlHeader{
			ControlRefs: []graph.RawControlRef{
				{Relation: "", TargetNumber: "POL-7", QuotedSpan: "..."},
			},
		},
		want: []graph.RawControlRef{},
	},
	{
		name: "ref with empty target number and title is dropped not guessed",
		header: graph.DocControlHeader{
			ControlRefs: []graph.RawControlRef{
				{Relation: "implements", QuotedSpan: "implements ???"},
			},
		},
		want: []graph.RawControlRef{},
	},
	{
		name: "unrecognized relation verb is rejected not guessed",
		header: graph.DocControlHeader{
			ControlRefs: []graph.RawControlRef{
				{Relation: "references", TargetNumber: "POL-9", QuotedSpan: "references POL-9"},
			},
		},
		want: []graph.RawControlRef{},
	},
	{
		name: "relation verb case and whitespace is normalized",
		header: graph.DocControlHeader{
			ControlRefs: []graph.RawControlRef{
				{Relation: "  Implements  ", TargetNumber: "POL-3", QuotedSpan: "a"},
				{Relation: "Derives From", TargetNumber: "POL-4", QuotedSpan: "b"},
				{Relation: "DERIVES", TargetNumber: "POL-5", QuotedSpan: "c"},
			},
		},
		want: []graph.RawControlRef{
			{Relation: "implements", TargetNumber: "POL-3", QuotedSpan: "a"},
			{Relation: "derives", TargetNumber: "POL-4", QuotedSpan: "b"},
			{Relation: "derives", TargetNumber: "POL-5", QuotedSpan: "c"},
		},
	},
	{
		name: "qualifying and malformed refs in the same header: only qualifying ones survive",
		header: graph.DocControlHeader{
			ControlRefs: []graph.RawControlRef{
				{Relation: "implements", TargetNumber: "POL-1", QuotedSpan: "keep-1"},
				{Relation: "", TargetNumber: "POL-2", QuotedSpan: "drop-empty-relation"},
				{Relation: "implements", QuotedSpan: "drop-empty-target"},
				{Relation: "derives", TargetTitle: "Some Standard", QuotedSpan: "keep-2"},
			},
		},
		want: []graph.RawControlRef{
			{Relation: "implements", TargetNumber: "POL-1", QuotedSpan: "keep-1"},
			{Relation: "derives", TargetTitle: "Some Standard", QuotedSpan: "keep-2"},
		},
	},
}

func TestParseControlRefs(t *testing.T) {
	for _, tt := range parseControlRefsTests {
		t.Run(tt.name, func(t *testing.T) {
			got := graph.ParseControlRefs(tt.header)
			if got == nil {
				t.Fatal("ParseControlRefs() returned nil, want a non-nil (possibly empty) slice")
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseControlRefs() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
