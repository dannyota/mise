// Package sharepoint implements a SharePoint Online web-crawl ingest source
// for the internal corpora (group-std, local-policy, local-sop). It discovers
// documents from a SharePoint document library via the REST web surface,
// authenticated through an adopter-supplied scoped account (DEC 13).
//
// # Crawl endpoint
//
// Discovery uses the SharePoint REST API
// /_api/web/GetFolderByServerRelativeUrl('<lib>')/Files?$expand=ListItemAllFields
// applied recursively with pagination (odata.nextLink continuation). This
// endpoint returns every file in the library with its list-item metadata in a
// single expansion, avoiding a separate ListItemAllFields call per file.
// Recursive enumeration walks sub-folders via the sibling Folders endpoint
// (also paginated). Both file and folder listing follow odata.nextLink until
// exhausted, capped at maxFilesPerLibrary (10 000) to bound memory.
//
// # Content fingerprint
//
// ContentFingerprint is set from each file's ETag (the SharePoint entity tag),
// which changes on every content or metadata mutation. This lets the pipeline
// detect in-place edits at discovery without downloading file bytes.
//
// # Well-known list columns (the adopter config contract)
//
// The connector maps these SharePoint list columns to DiscoveredDoc fields when
// present. Column names are case-insensitive; absent columns are silently
// skipped.
//
//   - Title           -> DiscoveredDoc.Title (fallback: filename stem)
//   - DocumentNumber  -> DiscoveredDoc.Number
//   - SignerRole      -> DiscoveredDoc.SignerRole
//   - OwnerDepartment -> DiscoveredDoc.OwnerDepartment
//   - OwnerRole       -> DiscoveredDoc.OwnerRole
//   - Version0        -> DiscoveredDoc.Version (named Version0 to avoid
//     collision with SharePoint's built-in UIVersionString)
//   - Language        -> DiscoveredDoc.Language
//   - IssuedDate      -> DiscoveredDoc.IssuedAt (ISO 8601 date)
//   - EffectiveDate   -> DiscoveredDoc.EffectiveAt (ISO 8601 date)
//
// # Environment variables
//
//   - SHAREPOINT_SITE_URL         — absolute site URL
//   - SHAREPOINT_AUTH_COOKIE      — FedAuth/rtFa cookie value
//   - SHAREPOINT_AUTH_BEARER      — OAuth2 bearer token
//   - SHAREPOINT_LIB_GROUP_STD    — server-relative library for group-std
//   - SHAREPOINT_LIB_LOCAL_POLICY — server-relative library for local-policy
//   - SHAREPOINT_LIB_LOCAL_SOP    — server-relative library for local-sop
package sharepoint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/ingest"
)

// SourceID is the stable source identifier for the SharePoint connector.
const SourceID = "sharepoint"

const (
	maxRetries         = 3
	baseBackoff        = 2 * time.Second
	maxBackoff         = 30 * time.Second
	maxDepth           = 20
	maxFilesPerLibrary = 10_000
)

// supportedExts is the set of file extensions the connector ingests —
// matching library's set (extract.go types minus legacy .doc).
var supportedExts = map[string]bool{
	"pdf":  true,
	"docx": true,
	"html": true,
	"md":   true,
}

// Authenticator applies credentials to an outgoing HTTP request. The adopter
// supplies an implementation matching their scoped account (cookie or bearer).
type Authenticator interface {
	Apply(req *http.Request) error
}

// Source is a SharePoint document-library crawler for one internal corpus.
// The zero value is not usable; call New.
type Source struct {
	siteURL  string
	libPath  string
	corpusID corpus.ID
	tier     corpus.AccessTier
	auth     Authenticator
	http     *http.Client
	log      *slog.Logger
}

// New returns a SharePoint source that discovers documents from the library
// at libPath on the given site, filing them under corpusID. An unregistered
// corpus ID is an error.
func New(
	siteURL, libPath string,
	id corpus.ID,
	auth Authenticator,
	client *http.Client,
	log *slog.Logger,
) (*Source, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	desc, ok := corpus.Get(id)
	if !ok {
		return nil, fmt.Errorf("sharepoint: unregistered corpus %q", id)
	}
	return &Source{
		siteURL:  strings.TrimRight(siteURL, "/"),
		libPath:  libPath,
		corpusID: id,
		tier:     desc.AccessTier,
		auth:     auth,
		http:     client,
		log:      log,
	}, nil
}

// ID implements ingest.Source.
func (s *Source) ID() string { return SourceID }

// Discover enumerates files in the configured document library recursively.
// Results are newest-first (by TimeLastModified); only files with mtime
// strictly after since are returned. keyword case-insensitively filters
// against the document's title and number.
func (s *Source) Discover(
	ctx context.Context, since time.Time, keyword string,
) ([]ingest.DiscoveredDoc, error) {
	visited := make(map[string]bool)
	docs, err := s.crawlFolder(ctx, s.libPath, 0, visited)
	if err != nil {
		return nil, fmt.Errorf("sharepoint discover: %w", err)
	}

	kw := strings.ToLower(strings.TrimSpace(keyword))
	var filtered []ingest.DiscoveredDoc
	for i := range docs {
		d := &docs[i]
		if !since.IsZero() && !d.PublishedAt.After(since) {
			continue
		}
		if kw != "" && !matchesKeyword(kw, d.Title, d.Number) {
			continue
		}
		filtered = append(filtered, *d)
	}

	// Newest-first.
	slices.SortFunc(filtered, func(a, b ingest.DiscoveredDoc) int {
		switch {
		case a.PublishedAt.After(b.PublishedAt):
			return -1
		case a.PublishedAt.Before(b.PublishedAt):
			return 1
		default:
			return 0
		}
	})

	return filtered, nil
}

// FetchDetail fetches a single file's metadata by its server-relative path
// (stored in DetailRef.DetailURL) and returns the enriched DiscoveredDoc
// with one FileRef.
func (s *Source) FetchDetail(
	ctx context.Context, ref ingest.DetailRef,
) (*ingest.DiscoveredDoc, error) {
	relPath := ref.DetailURL
	if relPath == "" {
		return nil, errors.New(
			"sharepoint fetch detail: empty detail url" +
				" (server-relative path)",
		)
	}

	apiURL := s.fileAPIURL(relPath) + "?$expand=ListItemAllFields"
	body, err := s.doGet(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("sharepoint fetch detail: %w", err)
	}

	var file spFile
	if err := json.Unmarshal([]byte(body), &file); err != nil {
		return nil, fmt.Errorf(
			"sharepoint fetch detail: parse response: %w", err,
		)
	}

	doc := s.fileToDoc(file)
	return &doc, nil
}

// Download streams the file's binary content into w in a single attempt,
// returning byte count and lowercase-hex SHA-256. Downloads are not retried
// at the source level because a mid-stream failure leaves partial bytes in w
// (an io.Writer is not seekable); retrying would corrupt the output by
// appending a second attempt's bytes after the partial first. Temporal's
// activity retry handles transient failures with a fresh writer.
func (s *Source) Download(
	ctx context.Context, ref ingest.FileRef, w io.Writer,
) (int64, string, error) {
	if ref.URL == "" {
		return 0, "", errors.New("sharepoint download: empty url")
	}
	return s.doDownload(ctx, ref.URL, w)
}

func (s *Source) doDownload(
	ctx context.Context, rawURL string, w io.Writer,
) (int64, string, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, rawURL, nil,
	)
	if err != nil {
		return 0, "", fmt.Errorf(
			"sharepoint download: build request: %w", err,
		)
	}
	if err := s.auth.Apply(req); err != nil {
		return 0, "", fmt.Errorf("sharepoint download: auth: %w", err)
	}

	resp, err := s.http.Do(req) //nolint:bodyclose // closed in handleDownloadResp
	if err != nil {
		return 0, "", fmt.Errorf("sharepoint download: %w", err)
	}
	return s.handleDownloadResp(resp, w)
}

func (s *Source) handleDownloadResp(
	resp *http.Response, w io.Writer,
) (int64, string, error) {
	defer drainClose(resp.Body)
	if err := s.checkAuthStatus(resp.StatusCode); err != nil {
		return 0, "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", fmt.Errorf(
			"sharepoint download: status %d", resp.StatusCode,
		)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), resp.Body)
	if err != nil {
		return n, "", fmt.Errorf("sharepoint download: copy: %w", err)
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// crawlFolder recursively enumerates files in a library folder. depth and
// visited guard against cycles and unbounded recursion.
func (s *Source) crawlFolder(
	ctx context.Context, folderPath string,
	depth int, visited map[string]bool,
) ([]ingest.DiscoveredDoc, error) {
	if depth > maxDepth {
		s.log.Warn("sharepoint: max depth reached, stopping recursion",
			"folder", folderPath, "corpus", string(s.corpusID))
		return nil, nil
	}
	if visited[folderPath] {
		s.log.Warn("sharepoint: cycle detected, skipping folder",
			"folder", folderPath, "corpus", string(s.corpusID))
		return nil, nil
	}
	visited[folderPath] = true

	docs, err := s.fetchAllFiles(ctx, folderPath)
	if err != nil {
		return nil, err
	}

	subFolders, err := s.fetchAllFolders(ctx, folderPath)
	if err != nil {
		return nil, err
	}

	for _, sub := range subFolders {
		if strings.HasPrefix(sub.Name, "_") ||
			strings.HasPrefix(sub.Name, "Forms") {
			continue
		}
		subDocs, err := s.crawlFolder(
			ctx, sub.ServerRelativeURL, depth+1, visited,
		)
		if err != nil {
			s.log.Warn("sharepoint: skipping unreadable sub-folder",
				"folder", sub.ServerRelativeURL,
				"corpus", string(s.corpusID), "error", err)
			continue
		}
		docs = append(docs, subDocs...)
	}

	return docs, nil
}

// fetchAllFiles fetches all file pages from the Files endpoint.
func (s *Source) fetchAllFiles(
	ctx context.Context, folderPath string,
) ([]ingest.DiscoveredDoc, error) {
	nextURL := s.folderAPIURL(folderPath) +
		"/Files?$expand=ListItemAllFields"

	var docs []ingest.DiscoveredDoc
	for nextURL != "" {
		body, err := s.doGet(ctx, nextURL)
		if err != nil {
			return nil, err
		}

		var page spFilesResponse
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			return nil, fmt.Errorf("parse files response: %w", err)
		}

		for _, f := range page.Value {
			if !supportedExts[extOf(f.Name)] {
				continue
			}
			docs = append(docs, s.fileToDoc(f))
		}

		nextURL = page.NextLink
		if len(docs) >= maxFilesPerLibrary {
			s.log.Warn("sharepoint: file cap reached",
				"folder", folderPath,
				"cap", maxFilesPerLibrary,
				"corpus", string(s.corpusID))
			break
		}
	}
	return docs, nil
}

// fetchAllFolders fetches all folder pages from the Folders endpoint.
func (s *Source) fetchAllFolders(
	ctx context.Context, folderPath string,
) ([]spFolder, error) {
	nextURL := s.folderAPIURL(folderPath) + "/Folders"

	var folders []spFolder
	for nextURL != "" {
		body, err := s.doGet(ctx, nextURL)
		if err != nil {
			return nil, err
		}

		var page spFoldersResponse
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			return nil, fmt.Errorf("parse folders response: %w", err)
		}

		folders = append(folders, page.Value...)
		nextURL = page.NextLink
		if len(folders) >= maxFilesPerLibrary {
			break
		}
	}
	return folders, nil
}

// fileToDoc maps a SharePoint file API record to a DiscoveredDoc.
func (s *Source) fileToDoc(f spFile) ingest.DiscoveredDoc {
	ext := extOf(f.Name)
	mtime := f.TimeLastModified
	relPath := f.ServerRelativeURL

	title := titleFromFilename(f.Name)
	number := ""
	var signerRole, ownerDept, ownerRole, version, language string
	var issuedAt, effectiveAt time.Time

	// Map well-known list columns from ListItemAllFields.
	if f.ListItemAllFields != nil {
		if v := getField(f.ListItemAllFields, "Title"); v != "" {
			title = v
		}
		number = getField(f.ListItemAllFields, "DocumentNumber")
		signerRole = getField(f.ListItemAllFields, "SignerRole")
		ownerDept = getField(f.ListItemAllFields, "OwnerDepartment")
		ownerRole = getField(f.ListItemAllFields, "OwnerRole")
		version = getField(f.ListItemAllFields, "Version0")
		language = getField(f.ListItemAllFields, "Language")
		if v := getField(f.ListItemAllFields, "IssuedDate"); v != "" {
			issuedAt = parseDate(v)
		}
		if v := getField(f.ListItemAllFields, "EffectiveDate"); v != "" {
			effectiveAt = parseDate(v)
		}
	}

	// Fall back to TimeLastModified when IssuedDate is absent, so the
	// pipeline cursor (which advances on IssuedAt) never stalls at zero.
	if issuedAt.IsZero() {
		issuedAt = mtime
	}

	downloadURL := s.fileAPIURL(relPath) + "/$value"

	doc := ingest.DiscoveredDoc{
		SourceID:           SourceID,
		ExternalID:         f.UniqueID,
		Title:              title,
		Number:             number,
		PublishedAt:        mtime,
		IssuedAt:           issuedAt,
		EffectiveAt:        effectiveAt,
		DetailURL:          relPath,
		ContentFingerprint: normalizeETag(f.ETag),
		SignerRole:         signerRole,
		OwnerDepartment:    ownerDept,
		OwnerRole:          ownerRole,
		Version:            version,
		Language:           language,
		Files: []ingest.FileRef{{
			URL:      downloadURL,
			Name:     f.Name,
			Ext:      ext,
			Kind:     "main",
			MIMEType: mimeForExt(ext),
		}},
	}

	s.log.Info("sharepoint: discovered",
		"file", relPath,
		"corpus", string(s.corpusID),
		"tier", string(s.tier),
		"mtime", mtime.Format(time.RFC3339),
	)

	return doc
}

// Ensure Source implements ingest.Source at compile time.
var _ ingest.Source = (*Source)(nil)
