// Package library implements a filesystem document-library ingest source for
// the three internal corpora (group-std, local-policy, local-sop). Each corpus
// maps to a sub-directory under LIBRARY_ROOT; documents are regular files with
// supported extensions (.pdf/.docx/.html/.md — legacy .doc needs a conversion
// step the pipeline does not have) and optional .meta.json sidecars carrying
// doc-control metadata. The Source reads no network — it is designed for
// operator-managed drop folders synced from a bucket or mounted as a Podman
// volume.
package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
)

// SourceID is the stable source identifier for the library connector.
const SourceID = "library"

// supportedExts is the set of file extensions (without dot, lowercase) that
// constitute ingestable documents — exactly the types the Extractor can turn
// into text (pkg/ingest/extract.go). Everything else, notably legacy .doc,
// is skipped: admitting a type the Extractor rejects would strand the
// document as a permanent ledger failure instead of a visible skip.
var supportedExts = map[string]bool{
	"pdf":  true,
	"docx": true,
	"html": true,
	"md":   true,
}

// Source is a filesystem-backed document source for one internal corpus.
// The zero value is not usable; call New.
type Source struct {
	root     string
	corpusID corpus.ID
	tier     corpus.AccessTier
	log      *slog.Logger
}

// New returns a library source that discovers documents under root for the
// given corpus. The descriptor resolves the corpus's access tier, logged per
// discovered file as the misfiling mitigation — an unregistered id is an
// error, because silently logging an empty tier would defeat that mitigation.
func New(root string, id corpus.ID, log *slog.Logger) (*Source, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	desc, ok := corpus.Get(id)
	if !ok {
		return nil, fmt.Errorf("library: unregistered corpus %q", id)
	}
	return &Source{
		root:     root,
		corpusID: id,
		tier:     desc.AccessTier,
		log:      log,
	}, nil
}

// ID implements ingest.Source.
func (s *Source) ID() string { return SourceID }

// Discover walks the corpus root recursively and returns documents with mtime
// strictly after since (zero time = all). Results are newest-first. keyword
// (if non-empty) case-insensitively filters against the document's title
// (filename stem) and sidecar number.
func (s *Source) Discover(ctx context.Context, since time.Time, keyword string) ([]ingest.DiscoveredDoc, error) {
	if _, err := os.Stat(s.root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // absent root = no documents, not an error
		}
		return nil, fmt.Errorf("library discover: stat root: %w", err)
	}

	kw := strings.ToLower(strings.TrimSpace(keyword))
	var docs []ingest.DiscoveredDoc

	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Library is a corpus's only source, so partial success is the
			// drop-folder expectation: one unreadable entry must not abort the
			// whole walk (its cursor would never advance past healthy files).
			s.log.Warn("library: skipping unreadable entry",
				"path", path, "corpus", string(s.corpusID), "error", err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return s.visitDir(path, d)
		}
		doc, ok := s.visitFile(path, d, since, kw)
		if ok {
			docs = append(docs, doc)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("library discover: walk: %w", err)
	}

	// Newest-first.
	slices.SortFunc(docs, func(a, b ingest.DiscoveredDoc) int {
		switch {
		case a.PublishedAt.After(b.PublishedAt):
			return -1
		case a.PublishedAt.Before(b.PublishedAt):
			return 1
		default:
			return 0
		}
	})

	return docs, nil
}

// visitDir decides whether to descend into a directory.
func (s *Source) visitDir(path string, d fs.DirEntry) error {
	if strings.HasPrefix(d.Name(), ".") && path != s.root {
		return fs.SkipDir
	}
	return nil
}

// visitFile evaluates a single file entry. Returns the discovered doc and true
// when the file passes all filters (extension, watermark, keyword). It stats
// through symlinks (os.Stat, not DirEntry.Info) so a dangling link is skipped
// with a warning instead of being discovered as an unreadable document.
func (s *Source) visitFile(path string, d fs.DirEntry, since time.Time, kw string) (ingest.DiscoveredDoc, bool) {
	if !isDocument(d.Name()) {
		return ingest.DiscoveredDoc{}, false
	}
	info, err := os.Stat(path)
	if err != nil {
		s.log.Warn("library: skipping unreadable file",
			"path", path, "corpus", string(s.corpusID), "error", err)
		return ingest.DiscoveredDoc{}, false
	}
	mtime := info.ModTime().UTC()
	if !since.IsZero() && !mtime.After(since) {
		return ingest.DiscoveredDoc{}, false
	}

	title := titleFromFilename(d.Name())
	ext := extOf(d.Name())

	// For keyword filtering, read sidecar number eagerly (cheap: small JSON).
	var sidecarNumber string
	if kw != "" {
		if meta, mErr := readSidecar(sidecarPath(path)); mErr == nil && meta != nil {
			sidecarNumber = meta.Number
		}
	}
	if kw != "" && !matchesKeyword(kw, title, sidecarNumber) {
		return ingest.DiscoveredDoc{}, false
	}

	fp, err := fileSHA256(path)
	if err != nil {
		s.log.Warn("library: skipping unreadable file",
			"path", path, "corpus", string(s.corpusID), "error", err)
		return ingest.DiscoveredDoc{}, false
	}

	// Use the relative path from root as a stable external ID.
	rel, _ := filepath.Rel(s.root, path)

	doc := ingest.DiscoveredDoc{
		SourceID:           SourceID,
		ExternalID:         rel,
		Title:              title,
		PublishedAt:        mtime,
		IssuedAt:           mtime,
		DetailURL:          path,
		ContentFingerprint: fp,
		Files: []ingest.FileRef{{
			URL:      path,
			Name:     d.Name(),
			Ext:      ext,
			Kind:     "main",
			MIMEType: mimeForExt(ext),
		}},
	}

	s.log.Info("library: discovered",
		"file", rel,
		"corpus", string(s.corpusID),
		"tier", string(s.tier),
		"mtime", mtime.Format(time.RFC3339),
	)

	return doc, true
}

// FetchDetail reads the optional sidecar and merges metadata onto the
// discovered document. The DetailRef.ExternalID is the relative path from
// root; DetailRef.DetailURL is the absolute file path.
func (s *Source) FetchDetail(ctx context.Context, ref ingest.DetailRef) (*ingest.DiscoveredDoc, error) {
	_ = ctx
	path := ref.DetailURL
	if path == "" {
		path = filepath.Join(s.root, ref.ExternalID)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("library fetch detail %s: %w", ref.ExternalID, err)
	}

	name := filepath.Base(path)
	ext := extOf(name)
	title := titleFromFilename(name)
	mtime := info.ModTime().UTC()

	fp, err := fileSHA256(path)
	if err != nil {
		return nil, fmt.Errorf("library fetch detail %s: fingerprint: %w", ref.ExternalID, err)
	}

	doc := &ingest.DiscoveredDoc{
		SourceID:           SourceID,
		ExternalID:         ref.ExternalID,
		Title:              title,
		PublishedAt:        mtime,
		IssuedAt:           mtime,
		DetailURL:          path,
		ContentFingerprint: fp,
		Files: []ingest.FileRef{{
			URL:      path,
			Name:     name,
			Ext:      ext,
			Kind:     "main",
			MIMEType: mimeForExt(ext),
		}},
	}

	// Merge sidecar metadata when present.
	meta, err := readSidecar(sidecarPath(path))
	if err != nil {
		return nil, fmt.Errorf("library fetch detail %s: sidecar: %w", ref.ExternalID, err)
	}
	if meta != nil {
		applySidecar(doc, meta)
	}

	return doc, nil
}

// Download copies the referenced file into w, returning byte count and
// lowercase-hex SHA-256.
func (s *Source) Download(_ context.Context, ref ingest.FileRef, w io.Writer) (int64, string, error) {
	if ref.URL == "" {
		return 0, "", errors.New("library download: empty path")
	}
	f, err := os.Open(filepath.Clean(ref.URL)) //nolint:gosec // path is operator-controlled (drop folder)
	if err != nil {
		return 0, "", fmt.Errorf("library download %s: %w", ref.Name, err)
	}
	defer func() { _ = f.Close() }()

	n, sha, err := hashCopy(w, f)
	if err != nil {
		return n, "", fmt.Errorf("library download %s: copy: %w", ref.Name, err)
	}
	return n, sha, nil
}

// hashCopy copies r into w while computing the running SHA-256, returning the
// byte count and lowercase-hex digest — the one hashing recipe Download and
// fileSHA256 share, so the discovery fingerprint and the download sha can
// never drift apart.
func hashCopy(w io.Writer, r io.Reader) (int64, string, error) {
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), r)
	if err != nil {
		return n, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// fileSHA256 returns the lowercase-hex SHA-256 of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path)) //nolint:gosec // path is operator-controlled (drop folder)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	_, sha, err := hashCopy(io.Discard, f)
	return sha, err
}

// sidecarMeta is the JSON contract for <file>.meta.json. All fields are
// optional; unknown keys are rejected (strict decode) so typos surface at
// ingest.
type sidecarMeta struct {
	Number          string            `json:"number"`
	Title           string            `json:"title"`
	DocType         string            `json:"doc_type"`
	Language        string            `json:"language"`
	SignerName      string            `json:"signer_name"`
	SignerRole      string            `json:"signer_role"`
	OwnerDepartment string            `json:"owner_department"`
	OwnerRole       string            `json:"owner_role"`
	Version         string            `json:"version"`
	IssuedDate      string            `json:"issued_date"`
	EffectiveDate   string            `json:"effective_date"`
	Relations       []sidecarRelation `json:"relations"`
}

type sidecarRelation struct {
	Type         string `json:"type"`
	TargetNumber string `json:"target_number"`
}

// sidecarPath returns the path to the sidecar for a document file.
func sidecarPath(filePath string) string {
	return filePath + ".meta.json"
}

// readSidecar reads and strictly decodes the sidecar JSON. Returns nil, nil
// when the sidecar does not exist. Returns an error for malformed JSON or
// unknown fields.
func readSidecar(path string) (*sidecarMeta, error) {
	f, err := os.Open(filepath.Clean(path)) //nolint:gosec // path is operator-controlled (drop folder)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var meta sidecarMeta
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return &meta, nil
}

// applySidecar merges sidecar metadata onto doc. Sidecar fields override only
// when non-empty; the filename-derived title is the fallback.
func applySidecar(doc *ingest.DiscoveredDoc, meta *sidecarMeta) {
	if meta.Title != "" {
		doc.Title = meta.Title
	}
	if meta.Number != "" {
		doc.Number = meta.Number
	}
	if meta.DocType != "" {
		doc.DocType = ingest.DocType(meta.DocType)
	}
	if meta.Language != "" {
		doc.Language = meta.Language
	}
	if meta.SignerName != "" {
		doc.Signer = meta.SignerName
	}
	if meta.SignerRole != "" {
		doc.SignerRole = meta.SignerRole
	}
	if meta.OwnerDepartment != "" {
		doc.OwnerDepartment = meta.OwnerDepartment
	}
	if meta.OwnerRole != "" {
		doc.OwnerRole = meta.OwnerRole
	}
	if meta.Version != "" {
		doc.Version = meta.Version
	}
	if meta.IssuedDate != "" {
		if t := parseDate(meta.IssuedDate); !t.IsZero() {
			doc.IssuedAt = t
			doc.PublishedAt = t
		}
	}
	if meta.EffectiveDate != "" {
		if t := parseDate(meta.EffectiveDate); !t.IsZero() {
			doc.EffectiveAt = t
		}
	}
	for _, r := range meta.Relations {
		doc.Relations = append(doc.Relations, ingest.Relation{
			Type:         r.Type,
			TargetNumber: r.TargetNumber,
		})
	}
}

// parseDate parses a YYYY-MM-DD date string.
func parseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// isDocument reports whether the filename has a supported extension and is not
// a hidden file or sidecar.
func isDocument(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	if strings.HasSuffix(name, ".meta.json") {
		return false
	}
	return supportedExts[extOf(name)]
}

// extOf returns the lowercase extension without the dot.
func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

// titleFromFilename derives a human title from the filename stem.
func titleFromFilename(name string) string {
	// Strip the last extension (e.g. ".pdf").
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		name = name[:i]
	}
	return name
}

// matchesKeyword does a case-insensitive substring match on title or number.
func matchesKeyword(kw, title, number string) bool {
	return strings.Contains(strings.ToLower(title), kw) ||
		strings.Contains(strings.ToLower(number), kw)
}

// mimeForExt returns the MIME type for supported extensions — each one a type
// the Extractor dispatches on (pkg/ingest/extract.go).
func mimeForExt(ext string) string {
	switch ext {
	case "pdf":
		return "application/pdf"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "html":
		return "text/html"
	case "md":
		return "text/markdown"
	default:
		return ""
	}
}
