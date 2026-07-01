package corpus_test

import (
	"testing"

	"danny.vn/mise/pkg/corpus"
)

func TestAllDescriptorsShareEmbedSpace(t *testing.T) {
	for _, d := range corpus.All() {
		if d.Embed.Model != "gemini-embedding-001" {
			t.Errorf("corpus %s: embed model = %q, want gemini-embedding-001", d.ID, d.Embed.Model)
		}
		if d.Embed.Dims != 1536 {
			t.Errorf("corpus %s: embed dims = %d, want 1536", d.ID, d.Embed.Dims)
		}
	}
}

func TestAllFiveDescriptorsExist(t *testing.T) {
	want := []corpus.ID{
		corpus.VNReg, corpus.MYReg, corpus.GroupStd,
		corpus.LocalPolicy, corpus.LocalSOP,
	}
	for _, id := range want {
		d, ok := corpus.Get(id)
		if !ok {
			t.Fatalf("corpus %s not found in registry", id)
		}
		if d.ID != id {
			t.Errorf("corpus %s: ID = %s", id, d.ID)
		}
	}
}

func TestPublicCorpusAccessTier(t *testing.T) {
	for _, id := range []corpus.ID{corpus.VNReg, corpus.MYReg} {
		d, _ := corpus.Get(id)
		if d.AccessTier != corpus.TierPublic {
			t.Errorf("corpus %s: tier = %s, want public", id, d.AccessTier)
		}
	}
}

func TestJurisdictionMapping(t *testing.T) {
	tests := []struct {
		id           corpus.ID
		jurisdiction string
	}{
		{corpus.VNReg, "vn"},
		{corpus.MYReg, "my"},
		{corpus.GroupStd, "my"},
		{corpus.LocalPolicy, "vn"},
		{corpus.LocalSOP, "vn"},
	}
	for _, tt := range tests {
		d, _ := corpus.Get(tt.id)
		if d.Jurisdiction != tt.jurisdiction {
			t.Errorf("corpus %s: jurisdiction = %q, want %q", tt.id, d.Jurisdiction, tt.jurisdiction)
		}
	}
}

func TestSatisfiesMapsByJurisdiction(t *testing.T) {
	localPolicy, _ := corpus.Get(corpus.LocalPolicy)
	if localPolicy.GraphRole.SatisfiesTarget != corpus.VNReg {
		t.Errorf("local-policy satisfies target = %s, want vn-reg", localPolicy.GraphRole.SatisfiesTarget)
	}
	groupStd, _ := corpus.Get(corpus.GroupStd)
	if groupStd.GraphRole.SatisfiesTarget != corpus.MYReg {
		t.Errorf("group-std satisfies target = %s, want my-reg", groupStd.GraphRole.SatisfiesTarget)
	}
}

func TestSchemaName(t *testing.T) {
	tests := []struct {
		id   corpus.ID
		want string
	}{
		{corpus.VNReg, "vn_reg"},
		{corpus.MYReg, "my_reg"},
		{corpus.GroupStd, "group_std"},
		{corpus.LocalPolicy, "local_policy"},
		{corpus.LocalSOP, "local_sop"},
	}
	for _, tt := range tests {
		d, _ := corpus.Get(tt.id)
		if d.SchemaName != tt.want {
			t.Errorf("corpus %s: schema = %q, want %q", tt.id, d.SchemaName, tt.want)
		}
	}
}
