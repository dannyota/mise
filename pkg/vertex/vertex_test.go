package vertex_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
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

func TestFixtureParserReturnsCannedText(t *testing.T) {
	dir := t.TempDir()
	content := []byte("%PDF-1.4 fake law document bytes")
	sum := sha256.Sum256(content)
	fixture := "Điều 1. Phạm vi điều chỉnh\nThông tư này quy định."
	name := hex.EncodeToString(sum[:]) + ".txt"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}

	p := vertex.NewFixtureParser(dir)
	result, err := p.Parse(context.Background(), content, "application/pdf")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Sections) != 1 {
		t.Fatalf("Parse() sections = %d, want 1", len(result.Sections))
	}
	if got := result.Sections[0].Text; got != fixture {
		t.Errorf("Parse() text = %q, want %q", got, fixture)
	}
}

func TestFixtureParserMissingFixtureErrors(t *testing.T) {
	p := vertex.NewFixtureParser(t.TempDir())
	if _, err := p.Parse(context.Background(), []byte("no fixture for me"), "application/pdf"); err == nil {
		t.Error("Parse() error = nil, want error for missing fixture")
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
