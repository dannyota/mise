package store_test

import (
	"errors"
	"testing"

	"danny.vn/mise/pkg/corpus"
	"danny.vn/mise/pkg/store"
)

func TestCheckGraphRole_SourceNotAllowed(t *testing.T) {
	err := store.CheckGraphRole(string(corpus.VNReg), string(corpus.LocalPolicy))
	if !errors.Is(err, store.ErrGraphRoleViolation) {
		t.Fatalf("vn-reg cannot source edges, expected ErrGraphRoleViolation, got %v", err)
	}
}

func TestCheckGraphRole_TargetNotAllowed(t *testing.T) {
	err := store.CheckGraphRole(string(corpus.LocalSOP), string(corpus.LocalSOP))
	if !errors.Is(err, store.ErrGraphRoleViolation) {
		t.Fatalf("local-sop cannot be targeted, expected ErrGraphRoleViolation, got %v", err)
	}
}

func TestCheckGraphRole_ValidPair(t *testing.T) {
	err := store.CheckGraphRole(string(corpus.LocalPolicy), string(corpus.VNReg))
	if err != nil {
		t.Fatalf("local-policy -> vn-reg should be valid, got %v", err)
	}
}

func TestCheckGraphRole_UnregisteredCorpus(t *testing.T) {
	err := store.CheckGraphRole("unknown", string(corpus.VNReg))
	if !errors.Is(err, store.ErrUnregisteredCorpus) {
		t.Fatalf("expected ErrUnregisteredCorpus, got %v", err)
	}
}
