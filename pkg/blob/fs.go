package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// FS is a Store backed by a local directory tree (local dev; LOCAL-DEV.md).
// Keys map to paths relative to root; Put stages content in a temp file and
// renames it into place so a reader never observes a partially written object.
type FS struct {
	root string
}

// NewFS returns a Store rooted at root. The directory tree is created lazily
// as keys are written.
func NewFS(root string) Store {
	return &FS{root: root}
}

// Put implements Store.
func (f *FS) Put(_ context.Context, key string, r io.Reader) (bool, error) {
	dest := filepath.Join(f.root, filepath.FromSlash(key))
	switch _, err := os.Stat(dest); {
	case err == nil:
		return false, nil // already exists — immutable, put-once
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("stat %s: %w", key, err)
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", dir, err)
	}

	wrote, err := writeTemp(dir, dest, r)
	if err != nil {
		return false, fmt.Errorf("put %s: %w", key, err)
	}
	return wrote, nil
}

// writeTemp stages r's content in a temp file under dir and renames it to
// dest, unless dest was created by a concurrent Put in the meantime — in which
// case the temp file is discarded (content-addressed keys make the two writes
// byte-identical, so losing this race is harmless).
func writeTemp(dir, dest string, r io.Reader) (bool, error) {
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return false, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed away

	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close temp file: %w", err)
	}

	if _, err := os.Stat(dest); err == nil {
		return false, nil // lost the race to a concurrent Put
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return false, fmt.Errorf("rename into place: %w", err)
	}
	return true, nil
}

// Get implements Store.
func (f *FS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	file, err := os.Open(filepath.Join(f.root, filepath.FromSlash(key)))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", key, err)
	}
	return file, nil
}

// Exists implements Store.
func (f *FS) Exists(_ context.Context, key string) (bool, error) {
	_, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(key)))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat %s: %w", key, err)
	}
}
