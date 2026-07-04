package vertex_test

import (
	"context"
	"testing"

	"danny.vn/mise/pkg/vertex"
)

func TestFakeCaptioner(t *testing.T) {
	c := vertex.NewFakeCaptioner()
	result, err := c.Caption(context.Background(), []byte("fake-image"), "image/png")
	if err != nil {
		t.Fatalf("Caption: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty caption text")
	}
	if result.Model == "" {
		t.Error("expected non-empty model record")
	}
	if result.Model != "fake-vision" {
		t.Errorf("model = %q, want fake-vision", result.Model)
	}
}
