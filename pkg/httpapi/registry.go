package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"danny.vn/mise/pkg/corpus"
)

// CorpusDescriptorWire is the wire form of corpus.Descriptor.
type CorpusDescriptorWire struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	SchemaName     string `json:"schema_name"`
	CitationScheme string `json:"citation_scheme"`
	AccessTier     string `json:"access_tier"`
	Tier           string `json:"tier,omitempty"`
	Jurisdiction   string `json:"jurisdiction"`
	EmbedModel     string `json:"embed_model"`
	EmbedDims      int    `json:"embed_dims"`
	CanSource      bool   `json:"can_source"`
	CanTarget      bool   `json:"can_target"`
}

func descriptorToWire(d corpus.Descriptor) CorpusDescriptorWire {
	return CorpusDescriptorWire{
		ID: string(d.ID), Kind: string(d.Kind),
		SchemaName: d.SchemaName, CitationScheme: d.CitationScheme,
		AccessTier: string(d.AccessTier), Tier: string(d.Tier),
		Jurisdiction: d.Jurisdiction,
		EmbedModel: d.Embed.Model, EmbedDims: d.Embed.Dims,
		CanSource: d.GraphRole.CanSource, CanTarget: d.GraphRole.CanTarget,
	}
}

// RegistryListOutput is GET /registry's response.
type RegistryListOutput struct {
	Body struct {
		Items []CorpusDescriptorWire `json:"items"`
	}
}

// RegistryGetInput is GET /registry/{id}'s path parameter.
type RegistryGetInput struct {
	ID string `path:"id"`
}

// RegistryGetOutput is GET /registry/{id}'s response.
type RegistryGetOutput struct {
	Body CorpusDescriptorWire
}

// RegisterRegistry mounts the corpus registry admin endpoints.
func RegisterRegistry(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-registry",
		Method:      http.MethodGet,
		Path:        "/registry",
		Summary:     "List all registered corpus descriptors",
		Tags:        []string{"Registry"},
	}, func(_ context.Context, _ *struct{}) (*RegistryListOutput, error) {
		all := corpus.All()
		out := &RegistryListOutput{}
		out.Body.Items = make([]CorpusDescriptorWire, len(all))
		for i, d := range all {
			out.Body.Items[i] = descriptorToWire(d)
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-registry-corpus",
		Method:      http.MethodGet,
		Path:        "/registry/{id}",
		Summary:     "Get a corpus descriptor by ID",
		Tags:        []string{"Registry"},
		Errors:      []int{http.StatusNotFound},
	}, func(_ context.Context, input *RegistryGetInput) (*RegistryGetOutput, error) {
		d, ok := corpus.Get(corpus.ID(input.ID))
		if !ok {
			return nil, huma.Error404NotFound(fmt.Sprintf("corpus %q not found", input.ID))
		}
		return &RegistryGetOutput{Body: descriptorToWire(d)}, nil
	})
}
