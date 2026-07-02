package vnlaw_test

import (
	"errors"
	"testing"

	"danny.vn/mise/pkg/parse/vnlaw"
)

const vbplTreeSample = `[
  {
    "id": "chapter-id",
    "key": "chapter-key",
    "title": "Chương I. QUY ĐỊNH CHUNG",
    "ptype": 2,
    "level": "Chapter",
    "orderIndex": 1,
    "content": {
      "title": "Chương I. QUY ĐỊNH CHUNG",
      "content": "Chương I. QUY ĐỊNH CHUNG"
    },
    "children": [
      {
        "id": "article-id",
        "key": "article-key",
        "title": "Điều 1. Phạm vi điều chỉnh",
        "ptype": 5,
        "level": "Article",
        "orderIndex": 1,
        "content": {
          "title": "Điều 1. Phạm vi điều chỉnh",
          "content": "Điều 1. Phạm vi điều chỉnh<br/>Nội dung riêng của điều.<br/>1. Khoản một.<br/>a) Điểm a."
        },
        "children": [
          {
            "id": "clause-id",
            "key": "clause-key",
            "title": "Khoản 1",
            "ptype": 6,
            "level": "Clause",
            "orderIndex": 1,
            "content": {
              "title": "Khoản 1",
              "content": "1. Khoản một.<br/>a) Điểm a."
            },
            "children": [
              {
                "id": "point-id",
                "key": "point-key",
                "title": "Điểm a",
                "ptype": 7,
                "level": "Point",
                "orderIndex": 1,
                "content": {
                  "title": "Điểm a",
                  "content": "a) Điểm a."
                },
                "children": []
              }
            ]
          }
        ]
      }
    ]
  }
]`

func TestParseTree(t *testing.T) {
	roots, err := vnlaw.ParseTree(vbplTreeSample)
	if err != nil {
		t.Fatalf("ParseTree() error = %v, want nil", err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}

	chapter := roots[0]
	if chapter.CitationPath != "chuong-1" {
		t.Fatalf("chapter path = %q, want chuong-1", chapter.CitationPath)
	}
	if len(chapter.Children) != 1 {
		t.Fatalf("chapter children = %d, want 1 article", len(chapter.Children))
	}
	article := chapter.Children[0]
	if article.Label != "Điều 1" || article.Heading != "Phạm vi điều chỉnh" {
		t.Fatalf("article title = %q/%q, want Điều 1/Phạm vi điều chỉnh", article.Label, article.Heading)
	}
	if article.Content != "Nội dung riêng của điều." {
		t.Fatalf("article content = %q, want own article text only", article.Content)
	}
	if len(article.Children) != 1 {
		t.Fatalf("article children = %d, want 1 clause", len(article.Children))
	}
	clause := article.Children[0]
	if clause.CitationPath != "chuong-1/dieu-1/khoan-1" || clause.Content != "Khoản một." {
		t.Fatalf(
			"clause = %q content %q, want path/content without point duplication",
			clause.CitationPath, clause.Content,
		)
	}
	if len(clause.Children) != 1 {
		t.Fatalf("clause children = %d, want 1 point", len(clause.Children))
	}
	point := clause.Children[0]
	if point.CitationPath != "chuong-1/dieu-1/khoan-1/diem-a" || point.Content != "Điểm a." {
		t.Fatalf("point = %q content %q, want point path/content", point.CitationPath, point.Content)
	}
}

func TestParseTreeEnvelope(t *testing.T) {
	roots, err := vnlaw.ParseTree(`{"success":true,"data":` + vbplTreeSample + `}`)
	if err != nil {
		t.Fatalf("ParseTree() error = %v, want nil", err)
	}
	if len(roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(roots))
	}
}

func TestParseTreeEmptyAndInvalid(t *testing.T) {
	if _, err := vnlaw.ParseTree(`[]`); !errors.Is(err, vnlaw.ErrEmptyProvisionTree) {
		t.Fatalf("empty tree err = %v, want ErrEmptyProvisionTree", err)
	}

	// Unlike banhmi (which flags this "ok=false" via a zero-Content stats
	// warning), ParseTree only errors on a shape/decode failure or on zero
	// nodes: a tree that decoded fine but whose one node carries no body text
	// is still a tree, just one Flatten will render as zero FlatSections.
	noContent := `[{"title":"Điều 1. Không có nội dung","ptype":5,"level":"Article",` +
		`"content":{"title":"Điều 1. Không có nội dung","content":""},"children":[]}]`
	roots, err := vnlaw.ParseTree(noContent)
	if err != nil {
		t.Fatalf("contentless tree err = %v, want nil", err)
	}
	if len(roots) != 1 || roots[0].Content != "" {
		t.Fatalf("roots = %#v, want one node with empty content", roots)
	}

	if _, err := vnlaw.ParseTree(`not-json`); !errors.Is(err, vnlaw.ErrInvalidProvisionTree) {
		t.Fatalf("invalid tree err = %v, want ErrInvalidProvisionTree", err)
	}
}
