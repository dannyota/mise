package law_test

import (
	"testing"

	"danny.vn/mise/pkg/parse/law"
)

type flattenTestCase struct {
	name  string
	nodes []*law.Node
	want  []law.FlatSection
}

// flattenTests is a package-level table (rather than a local var inside
// TestFlatten) so the test function body itself stays short; see each case's
// name for what it covers.
var flattenTests = []flattenTestCase{
	{
		name:  "no nodes",
		nodes: nil,
		want:  nil,
	},
	{
		name: "three level tree depth first with heading path",
		nodes: []*law.Node{
			{
				Kind: "chuong", Label: "Chương I", Heading: "QUY ĐỊNH CHUNG",
				CitationPath: "chuong-I",
				Children: []*law.Node{
					{
						Kind: "dieu", Label: "Điều 1", Heading: "Phạm vi điều chỉnh",
						Content:      "Thông tư này quy định về hoạt động cho vay.",
						CitationPath: "chuong-I/dieu-1",
						Children: []*law.Node{
							{
								Kind:         "khoan",
								Label:        "1.",
								Content:      "Ngân hàng thương mại nhà nước.",
								CitationPath: "chuong-I/dieu-1/khoan-1",
							},
						},
					},
				},
			},
		},
		want: []law.FlatSection{
			{
				CitationPath: "chuong-I/dieu-1",
				HeadingPath:  "Chương I — QUY ĐỊNH CHUNG > Điều 1 — Phạm vi điều chỉnh",
				Body:         "Thông tư này quy định về hoạt động cho vay.",
			},
			{
				CitationPath: "chuong-I/dieu-1/khoan-1",
				HeadingPath:  "Chương I — QUY ĐỊNH CHUNG > Điều 1 — Phạm vi điều chỉnh > 1.",
				Body:         "Ngân hàng thương mại nhà nước.",
			},
		},
	},
	{
		name: "node with no content of its own is skipped but children still flatten",
		nodes: []*law.Node{
			{
				Label:        "Chương I", // no Heading, no Content
				CitationPath: "chuong-I",
				Children: []*law.Node{
					{Label: "Điều 1", Content: "Body.", CitationPath: "chuong-I/dieu-1"},
				},
			},
		},
		want: []law.FlatSection{
			{CitationPath: "chuong-I/dieu-1", HeadingPath: "Chương I > Điều 1", Body: "Body."},
		},
	},
	{
		name: "whitespace-only content counts as empty",
		nodes: []*law.Node{
			{Label: "Điều 1", Content: "   \n\t  ", CitationPath: "dieu-1"},
		},
		want: nil,
	},
	{
		name: "siblings keep independent heading paths",
		nodes: []*law.Node{
			{Label: "Điều 1", Content: "First.", CitationPath: "dieu-1"},
			{Label: "Điều 2", Content: "Second.", CitationPath: "dieu-2"},
		},
		want: []law.FlatSection{
			{CitationPath: "dieu-1", HeadingPath: "Điều 1", Body: "First."},
			{CitationPath: "dieu-2", HeadingPath: "Điều 2", Body: "Second."},
		},
	},
}

func TestFlatten(t *testing.T) {
	for _, tt := range flattenTests {
		t.Run(tt.name, func(t *testing.T) {
			got := law.Flatten(tt.nodes)
			if len(got) != len(tt.want) {
				t.Fatalf("Flatten() returned %d sections, want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("section[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
