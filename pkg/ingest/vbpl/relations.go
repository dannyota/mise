package vbpl

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"danny.vn/mise/pkg/ingest"
)

type vbplReference struct {
	ReferenceType int           `json:"referenceType"`
	Target        vbplRefTarget `json:"targetDocument"`
}

type vbplRefTarget struct {
	ID     json.RawMessage `json:"id"`
	DocNum string          `json:"docNum"`
	Title  string          `json:"title"`
}

type diagramResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DocumentNamesByType map[string][]diagramDocument `json:"documentNamesByType"`
	} `json:"data"`
}

type diagramDocument struct {
	ID   json.RawMessage `json:"id"`
	Name string          `json:"name"`
}

func (s *Source) vbplRelations(refs []vbplReference) []ingest.Relation {
	out := make([]ingest.Relation, 0, len(refs))
	for _, ref := range refs {
		targetNumber := strings.TrimSpace(ref.Target.DocNum)
		targetID := rawJSONID(ref.Target.ID)
		if targetNumber == "" && targetID == "" {
			continue
		}
		targetURL := ""
		if targetID != "" {
			targetURL = detailURL(targetID)
		}
		out = append(out, ingest.Relation{
			Type:         s.relationLabel(ref.ReferenceType),
			TypeRaw:      ref.ReferenceType,
			TargetNumber: targetNumber,
			TargetID:     targetID,
			TargetTitle:  strings.TrimSpace(ref.Target.Title),
			TargetURL:    targetURL,
		})
	}
	return out
}

func (s *Source) vbplDiagramRelations(byType map[string][]diagramDocument) []ingest.Relation {
	var out []ingest.Relation
	for rawType, docs := range byType {
		referenceType, err := strconv.Atoi(rawType)
		if err != nil {
			continue
		}
		for _, doc := range docs {
			targetID := rawJSONID(doc.ID)
			targetTitle := strings.TrimSpace(doc.Name)
			targetNumber := docNumberFromDiagramName(targetTitle)
			if targetNumber == "" && targetID == "" {
				continue
			}
			targetURL := ""
			if targetID != "" {
				targetURL = detailURL(targetID)
			}
			out = append(out, ingest.Relation{
				Type:         s.relationLabel(referenceType),
				TypeRaw:      referenceType,
				TargetNumber: targetNumber,
				TargetID:     targetID,
				TargetTitle:  targetTitle,
				TargetURL:    targetURL,
			})
		}
	}
	return out
}

func mergeVBPLRelations(groups ...[]ingest.Relation) []ingest.Relation {
	seen := map[string]bool{}
	var out []ingest.Relation
	for _, group := range groups {
		for _, rel := range group {
			keys := vbplRelationKeys(rel)
			if len(keys) == 0 || anySeen(seen, keys) {
				continue
			}
			for _, key := range keys {
				seen[key] = true
			}
			out = append(out, rel)
		}
	}
	return out
}

func anySeen(seen map[string]bool, keys []string) bool {
	for _, key := range keys {
		if seen[key] {
			return true
		}
	}
	return false
}

func vbplRelationKeys(rel ingest.Relation) []string {
	typeKey := rel.Type
	if rel.TypeRaw != 0 {
		typeKey = strconv.Itoa(rel.TypeRaw)
	}
	typeKey = strings.TrimSpace(typeKey)
	if typeKey == "" {
		return nil
	}
	if targetID := strings.TrimSpace(rel.TargetID); targetID != "" {
		return []string{typeKey + "|id|" + targetID}
	}
	if targetNumber := canonicalVBPLDocNumber(rel.TargetNumber); targetNumber != "" {
		return []string{typeKey + "|num|" + targetNumber}
	}
	if targetTitle := strings.TrimSpace(rel.TargetTitle); targetTitle != "" {
		return []string{typeKey + "|title|" + strings.ToUpper(targetTitle)}
	}
	return nil
}

// relationLabel resolves a vbpl referenceType code to a relation_type label via
// the operator-editable config.relation_type map, falling back to the built-in
// vbplReferenceType defaults when the map lacks the code or is unset.
func (s *Source) relationLabel(t int) string {
	if s.relationTypes != nil {
		if label, ok := s.relationTypes[t]; ok && strings.TrimSpace(label) != "" {
			return label
		}
	}
	return vbplReferenceType(t)
}

func vbplReferenceType(t int) string {
	switch t {
	case 3:
		return "legal_basis"
	case 10:
		return "amends_supplements"
	case 12:
		return "replaces"
	default:
		return fmt.Sprintf("vbpl_type_%d", t)
	}
}

func rawJSONID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var out string
		if err := json.Unmarshal(raw, &out); err == nil {
			return strings.TrimSpace(out)
		}
	}
	return strings.TrimSpace(s)
}

var diagramDocNumberRe = regexp.MustCompile(
	`(?i)(?:\b\d{1,4}\s*/\s*\d{4}\s*/\s*[\pL\d]+(?:\s*-\s*[\pL\d]+)*|\b\d{1,4}\s*/\s*[\pL][\pL\d]*(?:\s*-\s*[\pL\d]+)*)`,
)

func docNumberFromDiagramName(name string) string {
	match := diagramDocNumberRe.FindString(name)
	return canonicalVBPLDocNumber(match)
}

func canonicalVBPLDocNumber(number string) string {
	number = strings.TrimSpace(number)
	if number == "" {
		return ""
	}
	number = regexp.MustCompile(`\s*([/-])\s*`).ReplaceAllString(number, "$1")
	number = strings.Trim(number, " \t\r\n,.;:()[]{}")
	return strings.ToUpper(number)
}
