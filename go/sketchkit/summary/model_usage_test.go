// SPDX-License-Identifier: Apache-2.0
package summary

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/modelusage"
)

const modelVectors = "../../../vectors/model_usage"

func vectorBytes(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(modelVectors, path))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func vectorJSON(t testing.TB, path string, target any) {
	t.Helper()
	if err := json.Unmarshal(vectorBytes(t, path), target); err != nil {
		t.Fatal(err)
	}
}
func vectorDoc(t testing.TB, path string) Envelope {
	t.Helper()
	e, err := Parse(vectorBytes(t, path))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return e
}
func modelReason(err error) string {
	var e *modelusage.Error
	if errors.As(err, &e) {
		return e.Reason
	}
	for _, p := range [][2]string{{"batch size", "batch_count"}, {"contract", "contract"}, {"unexpected", "producer"}, {"conflicting", "replay_conflict"}, {"regressed", "regression"}, {"overflow", "overflow"}, {"overlapping", "epoch_overlap"}, {"observation", "interval"}, {"size", "size"}} {
		if strings.Contains(err.Error(), p[0]) {
			return p[1]
		}
	}
	return "structure"
}

type vectorCase struct {
	Name               string   `json:"name"`
	File               string   `json:"file"`
	Accept             bool     `json:"accept"`
	Reason             string   `json:"reason"`
	Inputs             []string `json:"inputs"`
	ExpectedProducers  []string `json:"expected_producers"`
	ExpectedProjection string   `json:"expected_projection"`
}

func TestModelUsageParseVectors(t *testing.T) {
	for _, path := range []string{"cases/parse.json", "cases/parse_additions.json"} {
		var suite struct{ Cases []vectorCase }
		vectorJSON(t, path, &suite)
		for _, c := range suite.Cases {
			t.Run(c.Name, func(t *testing.T) {
				data := vectorBytes(t, c.File)
				doc, err := Parse(data)
				if !c.Accept {
					if err == nil || modelReason(err) != c.Reason {
						t.Fatalf("want %s got %v", c.Reason, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				out, err := doc.MarshalBinary()
				if err != nil || !bytes.Equal(out, data) {
					t.Fatalf("round trip: %v", err)
				}
			})
		}
	}
}
func permute(values []string, visit func([]string)) {
	var f func(int)
	f = func(i int) {
		if i == len(values) {
			visit(values)
			return
		}
		for j := i; j < len(values); j++ {
			values[i], values[j] = values[j], values[i]
			f(i + 1)
			values[i], values[j] = values[j], values[i]
		}
	}
	f(0)
}
func TestModelUsageMergeVectors(t *testing.T) {
	var suite struct{ Cases []vectorCase }
	vectorJSON(t, "cases/merge.json", &suite)
	for _, c := range suite.Cases {
		t.Run(c.Name, func(t *testing.T) {
			permute(append([]string{}, c.Inputs...), func(paths []string) {
				docs := make([]Envelope, len(paths))
				for i, p := range paths {
					docs[i] = vectorDoc(t, p)
				}
				result, err := Combine(docs, c.ExpectedProducers)
				for i, d := range docs {
					data, e := d.MarshalBinary()
					if e != nil || !bytes.Equal(data, vectorBytes(t, paths[i])) {
						t.Fatal("input mutation")
					}
				}
				if !c.Accept {
					if err == nil || modelReason(err) != c.Reason {
						t.Fatalf("want %s got %v", c.Reason, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				sources := make([][]any, 0, len(result.Sources))
				for _, s := range result.Sources {
					sources = append(sources, []any{s.ProducerID, s.Epoch, s.Sequence})
				}
				projection := map[string]any{"counters": result.Counters, "model_usage": result.ModelUsage, "selected_sources": sources, "missing": result.Missing, "partial": result.Partial}
				data, _ := json.Marshal(projection)
				if !bytes.Equal(data, vectorBytes(t, c.ExpectedProjection)) {
					t.Fatalf("projection mismatch\ngot %s\nwant %s", data, vectorBytes(t, c.ExpectedProjection))
				}
				repeated, e := Combine(append(docs, docs...), c.ExpectedProducers)
				if e != nil || !reflect.DeepEqual(repeated, result) {
					t.Fatal("replay mismatch", e)
				}
			})
		})
	}
}
func TestModelUsageCollectionVectors(t *testing.T) {
	var contract struct {
		ContractID string            `json:"contract_id"`
		ModelMap   map[string]string `json:"model_map"`
	}
	vectorJSON(t, "contract.json", &contract)
	type event struct {
		Input, Output, CacheRead, CacheWrite *uint64
		InputConflict                        bool `json:"input_conflict"`
		Provenance                           string
		Route                                string
		ObservedModel                        string `json:"observed_model"`
	}
	var suite struct {
		Cases []struct {
			Name            string
			Events          []json.RawMessage
			ExpectedModelID string           `json:"expected_model_id"`
			ExpectedRow     modelusage.Model `json:"expected_row"`
			ForbiddenOutput string           `json:"forbidden_output"`
		}
	}
	vectorJSON(t, "cases/collection.json", &suite)
	for _, c := range suite.Cases {
		t.Run(c.Name, func(t *testing.T) {
			row := modelusage.Model{}
			id := ""
			for _, raw := range c.Events {
				var e event
				if err := json.Unmarshal(raw, &e); err != nil {
					t.Fatal(err)
				}
				var fields map[string]*json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				// Explicit wire names avoid conflating absent cache fields with zero.
				for k, p := range map[string]**uint64{"cache_read": &e.CacheRead, "cache_write": &e.CacheWrite} {
					if fields[k] != nil {
						_ = json.Unmarshal(*fields[k], p)
					}
				}
				if e.ObservedModel == "" {
					e.ObservedModel = "source-a"
				}
				var err error
				id, err = modelusage.ClassifyModel(e.ObservedModel, contract.ModelMap, e.Route)
				if err != nil {
					t.Fatal(err)
				}
				before, _ := json.Marshal(row)
				newRow, err := modelusage.AddAttempt(row, modelusage.Attempt{Input: modelusage.Observation{Value: e.Input, Conflict: e.InputConflict}, Output: modelusage.Observation{Value: e.Output}, CacheRead: modelusage.Observation{Value: e.CacheRead}, CacheWrite: modelusage.Observation{Value: e.CacheWrite}, InputOrigin: e.Provenance, OutputOrigin: e.Provenance})
				if err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(row)
				if !bytes.Equal(before, after) {
					t.Fatal("input mutation")
				}
				row = newRow
			}
			if id != c.ExpectedModelID || !reflect.DeepEqual(row, c.ExpectedRow) {
				got, _ := json.Marshal(row)
				want, _ := json.Marshal(c.ExpectedRow)
				t.Fatalf("row mismatch %s\n%s", got, want)
			}
			data, _ := json.Marshal(row)
			if c.ForbiddenOutput != "" && bytes.Contains(data, []byte(c.ForbiddenOutput)) {
				t.Fatal("privacy sentinel leaked")
			}
		})
	}
}
func TestModelUsageContractVectors(t *testing.T) {
	for _, path := range []string{"contract.json", "contract-unicode.json"} {
		t.Run(path, func(t *testing.T) {
			var v struct {
				ContractID string            `json:"contract_id"`
				ModelMap   map[string]string `json:"model_map"`
				Routing    modelusage.Routing
				Contract   struct {
					AccountingID string `json:"accounting_id"`
				}
			}
			vectorJSON(t, path, &v)
			id, err := modelusage.ContractID(v.Contract.AccountingID, v.ModelMap, v.Routing)
			if err != nil || id != v.ContractID {
				t.Fatalf("contract %s %v", id, err)
			}
			v.Routing.Slices[0].FromResourceAttributes = nil
			id, err = modelusage.ContractID(v.Contract.AccountingID, v.ModelMap, v.Routing)
			if err != nil || id != v.ContractID {
				t.Fatalf("resolved contract %s %v", id, err)
			}
		})
	}
}
