package blob_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/mise/pkg/blob"
)

func TestKey(t *testing.T) {
	tests := []struct {
		name string
		sha  string
		ext  string
		want string
	}{
		{
			name: "pdf",
			sha:  "abcd1234efgh5678",
			ext:  ".pdf",
			want: "raw/ab/abcd1234efgh5678.pdf",
		},
		{
			name: "docx",
			sha:  "00ff11ee22dd33cc",
			ext:  ".docx",
			want: "raw/00/00ff11ee22dd33cc.docx",
		},
		{
			name: "no extension",
			sha:  "deadbeefcafe0000",
			ext:  "",
			want: "raw/de/deadbeefcafe0000",
		},
		{
			// A malformed (too-short) sha must degrade gracefully, not panic
			// on the [:2] shard slice — no real caller passes one, but Key
			// itself doesn't validate, so it must stay panic-safe regardless.
			name: "one-char sha does not panic",
			sha:  "a",
			ext:  ".pdf",
			want: "raw/a/a.pdf",
		},
		{
			name: "empty sha does not panic",
			sha:  "",
			ext:  ".pdf",
			want: "raw//.pdf",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blob.Key(tt.sha, tt.ext); got != tt.want {
				t.Errorf("Key(%q, %q) = %q, want %q", tt.sha, tt.ext, got, tt.want)
			}
		})
	}
}

// fakeSHA returns a 64-char lowercase-hex string starting with prefix, padded
// with '0' — a syntactically valid (if not really the SHA-256 of anything)
// key component for tests that only care that fs.go's key-shape validation
// accepts it, not about a real hash.
func fakeSHA(prefix string) string {
	return prefix + strings.Repeat("0", 64-len(prefix))
}

func TestFSPutFirstWriteSucceeds(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()

	wrote, err := store.Put(ctx, blob.Key(fakeSHA("ab"), ".pdf"), strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if !wrote {
		t.Error("Put() wrote = false, want true on first write")
	}
}

func TestFSPutSecondWriteIsNoop(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()
	key := blob.Key(fakeSHA("ab"), ".pdf")

	if _, err := store.Put(ctx, key, strings.NewReader("hello")); err != nil {
		t.Fatalf("first Put() error = %v", err)
	}

	wrote, err := store.Put(ctx, key, strings.NewReader("goodbye"))
	if err != nil {
		t.Fatalf("second Put() error = %v", err)
	}
	if wrote {
		t.Error("Put() wrote = true, want false on existing key (immutable, put-once)")
	}

	r, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content after second Put() = %q, want unchanged %q", got, "hello")
	}
}

func TestFSGetRoundTrips(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()
	key := blob.Key(fakeSHA("cd"), ".docx")
	want := "round trip content"

	if _, err := store.Put(ctx, key, strings.NewReader(want)); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	r, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer func() { _ = r.Close() }()

	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != want {
		t.Errorf("Get() content = %q, want %q", got, want)
	}
}

func TestFSGetMissingKeyErrors(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()

	if _, err := store.Get(ctx, blob.Key(fakeSHA("00"), ".pdf")); err == nil {
		t.Error("Get() error = nil, want error for missing key")
	}
}

func TestFSExists(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()
	key := blob.Key(fakeSHA("ef"), ".pdf")

	ok, err := store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if ok {
		t.Error("Exists() = true before Put, want false")
	}

	if _, err := store.Put(ctx, key, strings.NewReader("x")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	ok, err = store.Exists(ctx, key)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !ok {
		t.Error("Exists() = false after Put, want true")
	}
}

func TestFSPutWritesAtomicallyNoTempLeftovers(t *testing.T) {
	root := t.TempDir()
	store := blob.NewFS(root)
	ctx := context.Background()
	sha := fakeSHA("ab")
	key := blob.Key(sha, ".pdf")

	if _, err := store.Put(ctx, key, strings.NewReader("atomic content")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	dir := filepath.Join(root, "raw", "ab")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	wantName := sha + ".pdf"
	if len(entries) != 1 || entries[0].Name() != wantName {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("destination dir entries = %v, want only [%s]", names, wantName)
	}
}

// TestFSRejectsPathTraversalKeys pins the fs.go key-shape guard: Put/Get/
// Exists all reject a key that doesn't match Key()'s own raw/<2
// hex>/<64-hex-sha>[.ext] shape, closing the path-containment gap a
// hand-crafted key could otherwise exploit against the local FS root.
func TestFSRejectsPathTraversalKeys(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()
	sha := fakeSHA("ab")

	tests := []struct {
		name string
		key  string
	}{
		{name: "dot-dot segment", key: "raw/../../../etc/passwd"},
		{name: "trailing escape after a valid-looking key", key: "raw/ab/" + sha + ".pdf/../x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := store.Put(ctx, tt.key, strings.NewReader("x")); err == nil {
				t.Errorf("Put(%q) error = nil, want error", tt.key)
			}
			if _, err := store.Get(ctx, tt.key); err == nil {
				t.Errorf("Get(%q) error = nil, want error", tt.key)
			}
			if _, err := store.Exists(ctx, tt.key); err == nil {
				t.Errorf("Exists(%q) error = nil, want error", tt.key)
			}
		})
	}
}
