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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := blob.Key(tt.sha, tt.ext); got != tt.want {
				t.Errorf("Key(%q, %q) = %q, want %q", tt.sha, tt.ext, got, tt.want)
			}
		})
	}
}

func TestFSPutFirstWriteSucceeds(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()

	wrote, err := store.Put(ctx, "raw/ab/abcd1234.pdf", strings.NewReader("hello"))
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
	key := "raw/ab/abcd1234.pdf"

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
	key := "raw/cd/cdef5678.docx"
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

	if _, err := store.Get(ctx, "raw/00/does-not-exist.pdf"); err == nil {
		t.Error("Get() error = nil, want error for missing key")
	}
}

func TestFSExists(t *testing.T) {
	store := blob.NewFS(t.TempDir())
	ctx := context.Background()
	key := "raw/ef/ef123456.pdf"

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
	key := "raw/ab/atomic.pdf"

	if _, err := store.Put(ctx, key, strings.NewReader("atomic content")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	dir := filepath.Join(root, "raw", "ab")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "atomic.pdf" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("destination dir entries = %v, want only [atomic.pdf]", names)
	}
}
