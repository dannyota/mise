package vertex_test

import (
	"context"
	"testing"

	"danny.vn/mise/pkg/vertex"
)

func TestFakeParser(t *testing.T) {
	p := vertex.NewFakeParser()
	result, err := p.Parse(context.Background(), []byte("test content"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
}

func TestFakeJudge(t *testing.T) {
	j := vertex.NewFakeJudge()
	result, err := j.Judge(context.Background(), "from text", "to text")
	if err != nil {
		t.Fatal(err)
	}
	if result.Confidence == 0 {
		t.Fatal("expected non-zero confidence")
	}
}

func TestFakeGrounder(t *testing.T) {
	g := vertex.NewFakeGrounder()
	result, err := g.Ground(context.Background(), "claim", "source")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Grounded {
		t.Fatal("expected grounded")
	}
}
