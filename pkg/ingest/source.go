// Package ingest defines the source contract shared by every crawler under
// pkg/ingest/{source}. A Source discovers newly published documents from a
// newest-first feed, fetches a document's server-rendered detail (metadata +
// downloadable file references or inline HTML body), and downloads the raw
// files. Each source package is self-contained; this file holds only the
// cross-source types.
package ingest

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

// DocType is the source-native legal document type/instrument label: Vietnamese
// loại văn bản ("Thông tư", "Nghị định", "Quyết định", "Văn bản hợp nhất") for
// vn-reg, or the Malaysian instrument class (Act, Regulations, Policy Document,
// Guideline) for my-reg.
type DocType string

// FileRef points to a downloadable raw file (PDF/DOCX/DOC) attached to a document.
// URL is the absolute download URL as scraped from the source; the bytes are
// retrieved lazily via Source.Download so discovery stays cheap.
type FileRef struct {
	URL      string // absolute download URL (e.g. the CDN stream link)
	Name     string // file name as advertised by the source
	Ext      string // lowercase extension without the dot: "pdf", "docx", "doc"
	Kind     string // file's role in ingest: main, appendix, original_scan, attachment
	MIMEType string // best-effort content type, may be empty until downloaded
}

// Relation is an edge in a document's amendment/replacement graph, populated
// only when the source exposes relations. Type uses the source's own label
// (normalized later); Target identifies the related document by its citation
// number and/or the source's external id.
type Relation struct {
	Type         string `json:"type"`          // e.g. "amends", "replaces", "guides" (source label)
	TypeRaw      int    `json:"type_raw"`      // source relation code when the source exposes one (0 = unknown)
	TargetNumber string `json:"target_number"` // citation number of the related document, when known
	TargetID     string `json:"target_id"`     // source external id of the related document, when known
	TargetTitle  string `json:"target_title"`  // target title, when known
	TargetURL    string `json:"target_url"`    // detail URL of the related document, when known
}

// DiscoveredDoc is one document observed from a source. Discovery populates the
// fields cheaply available from the feed; FetchDetail enriches it from the
// detail page. Some sources serve the body inline as HTML (vbpl-style sites),
// others only as downloadable Files (congbao, AGC LoM, BNM, SC).
type DiscoveredDoc struct {
	// SourceID identifies the crawler package: "vbpl", "congbao", "sbv_hanoi",
	// "vanban" (vn-reg); "agclom", "bnm", "sc" (my-reg); "library" (internal).
	SourceID    string
	ExternalID  string     // site id or UUID
	DocGUID     string     // cross-source opaque id when the source exposes one
	Number      string     // citation number: số ký hiệu (vn-reg, e.g. "11/2026/TT-NHNN") or Act/PU/PD number (my-reg)
	Title       string     // document title (vbpl: trích yếu)
	Abstract    string     // body/preamble text from the feed, used for scope matching (may be empty)
	DocType     DocType    // source-native document type/instrument label
	DocTypeCode string     // source code for DocType, when known
	Issuer      string     // issuing authority (cơ quan ban hành / SBV / BNM / AGC / SC)
	IssuerCode  string     // source code for Issuer, when known
	Signer      string     // signer name (người ký), when the source exposes one
	IssuedAt    time.Time  // date promulgated/signed (ngày ban hành)
	EffectiveAt time.Time  // date in-force from (ngày hiệu lực); zero when the source omits it
	ExpireAt    time.Time  // expiry date (ngày hết hiệu lực); zero when the source omits it
	Status      string     // source-native validity status (e.g. vbpl's CHL/HHL codes); empty if unknown
	DetailURL   string     // canonical human/detail URL
	HTML        string     // inline body when the source serves it
	Files       []FileRef  // downloadable PDF/DOCX/DOC (congbao/vbpl/AGC LoM/BNM/SC; attachments)
	Relations   []Relation // amends/replaces/... when the source exposes them
	HasContent  bool       // first-party content flag when the source exposes one

	// IsConsolidated marks consolidated/reprint documents (VBHN for vn-reg) when
	// the source exposes the flag or the document type/number makes it deterministic.
	IsConsolidated bool

	// PublishedAt is the feed timestamp used as the discovery watermark (e.g.
	// congbao RSS <pubDate>). It may differ from IssuedAt.
	PublishedAt time.Time

	// Gazette metadata (congbao, and any other gazette-style source): the
	// gazette number and its publish date.
	GazetteNumber string
	GazetteDate   time.Time

	// Doc-control metadata carried by internal-corpus sources (the library
	// sidecar contract): the signer's role/title, the owning department and
	// role, the document's own version label, and its declared language code
	// ("vi", "en"). All empty for sources without document control; an empty
	// Language is derived from the corpus jurisdiction at normalize time.
	SignerRole      string
	OwnerDepartment string
	OwnerRole       string
	Version         string
	Language        string

	// ContentFingerprint is the lowercase-hex SHA-256 of the document's raw
	// content, set by sources that can fingerprint content cheaply at discovery
	// (library hashes local files). Empty means content-change detection rides
	// the discovery metadata fields alone (pipeline.discoveryHash).
	ContentFingerprint string

	// RawMeta is the source's raw record for this document as returned by the
	// feed, carried through ingest as provenance. It preserves fields not mapped
	// to typed columns for audit and offline scope re-tuning.
	RawMeta json.RawMessage
}

// DetailRef identifies a document for the heavier detail/enrichment fetch.
// ExternalID is the source API identity discovered from the feed and must be
// treated as opaque text (e.g. vbpl uses both numeric ItemIDs and UUIDs).
// DetailURL is the human/source URL when a source needs it, or for operator
// inspection.
type DetailRef struct {
	ExternalID string
	DetailURL  string
}

// Source is a self-contained crawler for one official site. Discovery is
// newest-first and watermark-bounded so the hourly Discover schedule stays
// nearly free; FetchDetail and Download are the heavier stages run per genuinely
// new document. Temporal bounds fetch concurrency; sources may also apply
// operator-configured pacing.
type Source interface {
	// ID returns the stable source identifier (see DiscoveredDoc.SourceID).
	ID() string

	// Discover reads the newest-first feed for a query keyword and returns
	// documents published strictly after the watermark (pass the zero time to
	// take the whole slice). keyword is the per-source query term — e.g. a vbpl
	// search keyword; sources with a single global feed (congbao RSS) ignore it.
	// Results are newest-first as the feed orders them.
	Discover(ctx context.Context, since time.Time, keyword string) ([]DiscoveredDoc, error)

	// FetchDetail fetches a document's detail page/API record and returns the
	// parsed metadata plus any downloadable file references (and inline HTML when
	// the source serves it).
	FetchDetail(ctx context.Context, ref DetailRef) (*DiscoveredDoc, error)

	// Download retrieves a file's bytes into w and returns the number of bytes
	// written and their SHA-256, lowercase hex.
	Download(ctx context.Context, ref FileRef, w io.Writer) (n int64, sha256Hex string, err error)
}

// NumberSearcher is implemented by sources that can look up one document by its
// exact citation number (số ký hiệu / Act no.). It is used for cross-source
// backfills, e.g. a stale VBPL placeholder can enqueue the matching congbao
// gazette file without widening the normal discovery crawl. titleHint is the
// caller's known title, passed as a disambiguating term for fuzzy source search
// APIs; implementations may ignore it but must always re-verify normalized
// citation-number equality before returning a hit.
type NumberSearcher interface {
	SearchByNumber(ctx context.Context, number, titleHint string) (*DiscoveredDoc, bool, error)
}

// TreeProvider is implemented by sources that expose a first-party provision
// tree. The returned content is source-native JSON; ok=false means the source has
// no usable tree for this document yet and callers should fall back to text
// parsing without treating the document as failed.
type TreeProvider interface {
	FetchTree(ctx context.Context, ref DetailRef) (content string, ok bool, err error)
}
