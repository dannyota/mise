// Package corpus defines the corpus descriptor registry. A corpus is a typed
// descriptor; adding one needs only a descriptor + source plugin — no core change.
package corpus

// ID uniquely identifies a corpus.
type ID string

// Predefined corpus IDs.
const (
	VNReg       ID = "vn-reg"
	MYReg       ID = "my-reg"
	GroupStd    ID = "group-std"
	LocalPolicy ID = "local-policy"
	LocalSOP    ID = "local-sop"
)

// Kind classifies the corpus content.
type Kind string

// Predefined corpus kinds.
const (
	KindLaw      Kind = "law"
	KindStandard Kind = "standard"
	KindPolicy   Kind = "policy"
	KindSOP      Kind = "sop"
	KindReport   Kind = "report"
	KindDiagram  Kind = "diagram"
)

// AccessTier controls RLS row visibility.
type AccessTier string

// Predefined access tiers.
const (
	TierPublic            AccessTier = "public"
	TierGroupConfidential AccessTier = "group-confidential"
	TierLocalConfidential AccessTier = "local-confidential"
)

// Tier distinguishes group-level from local-level internal docs.
type Tier string

// Predefined tiers.
const (
	TierGroup Tier = "group"
	TierLocal Tier = "local"
)

// EmbedConfig pins the embedding model + dimensions for a corpus.
type EmbedConfig struct {
	Model    string
	Dims     int
	TaskType string
}

// GraphRole defines how a corpus participates in the compliance graph.
type GraphRole struct {
	CanSource       bool
	CanTarget       bool
	DefaultEdges    []string
	SatisfiesTarget ID
}

// MetadataConfig holds per-source metadata defaults and parse locations.
type MetadataConfig struct {
	Defaults       map[string]string
	ParseLocations map[string]string
}

// Descriptor is the typed definition of a corpus.
type Descriptor struct {
	ID             ID
	Kind           Kind
	SchemaName     string
	CitationScheme string
	Embed          EmbedConfig
	AccessTier     AccessTier
	Tier           Tier
	Jurisdiction   string
	GraphRole      GraphRole
	MetadataConfig MetadataConfig
}

var registry = map[ID]Descriptor{
	VNReg: {
		ID: VNReg, Kind: KindLaw, SchemaName: "vn_reg",
		CitationScheme: "dieu-khoan-diem",
		Embed:          SharedEmbed,
		AccessTier:     TierPublic,
		Jurisdiction:   "vn",
		GraphRole:      GraphRole{CanTarget: true},
	},
	MYReg: {
		ID: MYReg, Kind: KindLaw, SchemaName: "my_reg",
		CitationScheme: "part-section-subsec",
		Embed:          SharedEmbed,
		AccessTier:     TierPublic,
		Jurisdiction:   "my",
		GraphRole:      GraphRole{CanTarget: true},
	},
	GroupStd: {
		ID: GroupStd, Kind: KindStandard, SchemaName: "group_std",
		CitationScheme: "standard-clause",
		Embed:          SharedEmbed,
		AccessTier:     TierGroupConfidential,
		Tier:           TierGroup,
		Jurisdiction:   "my",
		GraphRole: GraphRole{
			CanSource: true, CanTarget: true,
			DefaultEdges:    []string{"implements"},
			SatisfiesTarget: MYReg,
		},
	},
	LocalPolicy: {
		ID: LocalPolicy, Kind: KindPolicy, SchemaName: "local_policy",
		CitationScheme: "policy-section",
		Embed:          SharedEmbed,
		AccessTier:     TierLocalConfidential,
		Tier:           TierLocal,
		Jurisdiction:   "vn",
		GraphRole: GraphRole{
			CanSource: true, CanTarget: true,
			DefaultEdges:    []string{"satisfies", "implements"},
			SatisfiesTarget: VNReg,
		},
	},
	LocalSOP: {
		ID: LocalSOP, Kind: KindSOP, SchemaName: "local_sop",
		CitationScheme: "sop-step",
		Embed:          SharedEmbed,
		AccessTier:     TierLocalConfidential,
		Tier:           TierLocal,
		Jurisdiction:   "vn",
		GraphRole: GraphRole{
			CanSource:    true,
			DefaultEdges: []string{"derives"},
		},
	},
}

// SharedEmbed is the locked embedding config all corpora must match.
var SharedEmbed = EmbedConfig{
	Model:    "gemini-embedding-001",
	Dims:     1536,
	TaskType: "RETRIEVAL_DOCUMENT",
}

// All returns every registered descriptor.
func All() []Descriptor {
	out := make([]Descriptor, 0, len(registry))
	for _, d := range registry {
		out = append(out, d)
	}
	return out
}

// Get returns the descriptor for the given corpus ID.
func Get(id ID) (Descriptor, bool) {
	d, ok := registry[id]
	return d, ok
}
