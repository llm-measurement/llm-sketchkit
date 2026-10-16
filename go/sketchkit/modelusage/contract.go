// SPDX-License-Identifier: Apache-2.0

package modelusage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

// Routing is resolved producer configuration. Slice/key order is significant.
type Routing struct {
	Algorithm     string  `json:"algorithm"`
	MaxLabelBytes int     `json:"max_label_bytes"`
	MaxSlices     int     `json:"max_slices"`
	SelectedSlice string  `json:"selected_slice"`
	Slices        []Slice `json:"slices"`
}
type Slice struct {
	FromResourceAttributes []string `json:"from_resource_attributes"`
	Keys                   []string `json:"keys"`
	Name                   string   `json:"name"`
}

// ContractID hashes the exact approved map and resolved routing. Empty resource
// policy expands to that slice's keys. All other defaults must be resolved by
// the existing producer. No Unicode normalization is performed.
func ContractID(accountingID string, mapping map[string]string, routing Routing) (string, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`).MatchString(accountingID) || mapping == nil || routing.Algorithm != "inactive-lru-v1" || routing.MaxLabelBytes != 256 || routing.MaxSlices < 1 || routing.MaxSlices > MaxModels || routing.Slices == nil || !utf8.ValidString(routing.SelectedSlice) {
		return "", fail("structure")
	}
	for k, v := range mapping {
		if !utf8.ValidString(k) || !IsPublicID(v) {
			return "", fail("structure")
		}
	}
	routing.Slices = append([]Slice{}, routing.Slices...)
	names := map[string]bool{}
	selected := false
	for i, s := range routing.Slices {
		if !utf8.ValidString(s.Name) || names[s.Name] || s.Keys == nil {
			return "", fail("structure")
		}
		names[s.Name] = true
		for _, list := range [][]string{s.Keys, s.FromResourceAttributes} {
			for _, v := range list {
				if !utf8.ValidString(v) {
					return "", fail("structure")
				}
			}
		}
		if len(s.FromResourceAttributes) == 0 {
			routing.Slices[i].FromResourceAttributes = append([]string{}, s.Keys...)
		}
		if s.Name == routing.SelectedSlice {
			if len(s.Keys) != 1 || s.Keys[0] != "gen_ai.request.model" {
				return "", fail("structure")
			}
			selected = true
		}
	}
	if !selected {
		return "", fail("structure")
	}
	hash := func(v any) string {
		data, _ := json.Marshal(v)
		s := sha256.Sum256(data)
		return hex.EncodeToString(s[:])
	}
	contract := map[string]any{"accounting_id": accountingID, "cohort_rule": "complete-split-v1", "histogram_id": HistogramID, "model_mapping_sha256": hash(mapping), "routing_sha256": hash(routing), "version": 1}
	return "model-usage-v1:" + hash(contract), nil
}

// ClassifyModel preserves the existing router/source classifier. classification
// is empty for a normal observed name, or overflow/missing/too-long. No implicit
// pass-through, parsing of formatted labels, or reconstruction from overflow.
func ClassifyModel(observed string, mapping map[string]string, classification string) (string, error) {
	switch classification {
	case "__overflow__", "__missing__", "__too_long__":
		return classification, nil
	case "":
	default:
		return "", fail("structure")
	}
	if id, ok := mapping[observed]; ok {
		if !IsPublicID(id) {
			return "", fail("structure")
		}
		return id, nil
	}
	return "__unmapped__", nil
}
