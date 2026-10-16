// SPDX-License-Identifier: Apache-2.0
package summary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/modelusage"
)

func TestModelUsageLimitVectors(t *testing.T) {
	var suite struct {
		Cases []struct {
			Name, Template, Fixture, Reject string
			Accept                          bool
			Repeat                          int
			Construction                    struct {
				Count, Start int
				KeyFormat    string `json:"key_format"`
				Ranges       [][2]int
				ProducerIDs  []string `json:"producer_ids"`
			}
		}
	}
	vectorJSON(t, "cases/limits.json", &suite)
	for _, c := range suite.Cases {
		t.Run(c.Name, func(t *testing.T) {
			var err error
			switch c.Name {
			case "wire_bytes":
				_, err = Parse(bytes.Repeat([]byte{' '}, c.Construction.Count))
			case "input_batch_count":
				doc := vectorDoc(t, c.Template)
				docs := make([]Envelope, c.Repeat)
				for i := range docs {
					docs[i] = doc
				}
				_, err = Combine(docs, []string{doc.ProducerID})
			case "int64_max_accepted":
				_, err = Parse(vectorBytes(t, c.Fixture))
			case "merged_union_limit":
				docs := []Envelope{}
				for i, r := range c.Construction.Ranges {
					e := vectorDoc(t, c.Template)
					row := e.ModelUsage.Models["model-a"]
					e.ModelUsage.Models = map[string]modelusage.Model{}
					for n := r[0]; n <= r[1]; n++ {
						e.ModelUsage.Models[fmt.Sprintf(c.Construction.KeyFormat, n)] = row
					}
					e.ProducerID = c.Construction.ProducerIDs[i]
					for k, v := range e.Counters {
						e.Counters[k] = v * uint64(r[1]-r[0]+1)
					}
					data, e2 := e.MarshalBinary()
					if e2 != nil {
						t.Fatal(e2)
					}
					e, e2 = Parse(data)
					if e2 != nil {
						t.Fatal(e2)
					}
					docs = append(docs, e)
				}
				before, _ := json.Marshal(docs)
				_, err = Combine(docs, c.Construction.ProducerIDs)
				after, _ := json.Marshal(docs)
				if !bytes.Equal(before, after) {
					t.Fatal("input mutation")
				}
			case "real_model_limit_before_rows":
				var doc map[string]any
				vectorJSON(t, c.Template, &doc)
				models := map[string]any{}
				for i := c.Construction.Start; i < c.Construction.Count; i++ {
					models[fmt.Sprintf(c.Construction.KeyFormat, i)] = map[string]any{}
				}
				doc["model_usage"].(map[string]any)["models"] = models
				data, _ := json.Marshal(doc)
				_, err = Parse(data)
			case "input_limits_before_allocation":
				var doc map[string]any
				vectorJSON(t, c.Template, &doc)
				doc["model_usage"].(map[string]any)["models"] = [][][]int{{{0}}}
				data, _ := json.Marshal(doc)
				_, err = Parse(data)
			case "too_long_model_id":
				e := vectorDoc(t, c.Template)
				row := e.ModelUsage.Models["model-a"]
				e.ModelUsage.Models = map[string]modelusage.Model{strings.Repeat("m", c.Construction.Count): row}
				data, _ := json.Marshal(e)
				_, err = Parse(data)
			case "reserved_budget":
				_, err = Parse(vectorBytes(t, c.Fixture))
			default:
				t.Fatal("unhandled vector")
			}
			if c.Accept {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			want := c.Reject
			switch want {
			case "model_id":
				want = "structure"
			}
			if err == nil || modelReason(err) != want {
				t.Fatalf("want %s got %v", want, err)
			}
		})
	}
}

func TestModelUsageCompatibilityVectors(t *testing.T) {
	var suite struct {
		Cases []struct {
			Name, Fixture, Bytes, Diagnostic, Reject string
			Inputs                                   []string
			MustNotEmit                              []string `json:"must_not_emit"`
		}
	}
	vectorJSON(t, "cases/compatibility.json", &suite)
	for _, c := range suite.Cases {
		t.Run(c.Name, func(t *testing.T) {
			switch c.Name {
			case "existing_readers_reject":
				var root map[string]json.RawMessage
				vectorJSON(t, c.Fixture, &root)
				if len(root) != 15 {
					t.Fatal("legacy bound changed")
				}
			case "capability_preflight":
				present, err := HasModelUsage(vectorBytes(t, c.Fixture))
				if err != nil || !present || c.Diagnostic != "summary includes model_usage; upgrade fleetdiff" {
					t.Fatal(err)
				}
			case "disabled_unchanged":
				data := vectorBytes(t, c.Fixture)
				present, err := HasModelUsage(data)
				if err != nil || present {
					t.Fatal(err)
				}
				doc := vectorDoc(t, c.Fixture)
				out, _ := doc.MarshalBinary()
				if !bytes.Equal(out, data) {
					t.Fatal("default bytes changed")
				}
			case "no_substring_preflight", "malformed_model_no_echo":
				_, err := HasModelUsage([]byte(c.Bytes))
				if err == nil {
					t.Fatal("accepted malformed preflight")
				}
				for _, s := range c.MustNotEmit {
					if strings.Contains(err.Error(), s) {
						t.Fatal("diagnostic leak")
					}
				}
			case "key_team_scope_unavailable":
				doc := vectorDoc(t, c.Fixture)
				data, _ := json.Marshal(doc.ModelUsage)
				var block map[string]any
				_ = json.Unmarshal(data, &block)
				if len(block) != 4 {
					t.Fatal("unexpected intersection")
				}
			default:
				var err error
				if len(c.Inputs) > 0 {
					docs := []Envelope{}
					for _, p := range c.Inputs {
						docs = append(docs, vectorDoc(t, p))
					}
					_, err = Combine(docs, []string{"p1", "p2"})
				} else {
					_, err = Parse(vectorBytes(t, c.Fixture))
				}
				if err == nil {
					t.Fatal("expected rejection")
				}
				messages := map[string]string{"unsupported_extension": "summary includes unsupported model_usage; upgrade fleetdiff", "presence": "summaries mix model_usage-enabled and legacy files; select one compatible period or re-export with matching settings", "contract": "summaries have incompatible model_usage contracts; use matching producer settings"}
				if messages[modelReason(err)] != c.Diagnostic {
					t.Fatalf("reason %v", err)
				}
			}
		})
	}
}

type pricingCase struct {
	Name, Fixture, Reject string
	InputFactor           string `json:"input_factor"`
	OutputFactor          string `json:"output_factor"`
	Fraction              string
	Expected              map[string]string `json:"expected_micro_usd"`
	Tiers                 []struct {
		Upper *uint64
		Rates map[string]string
	}
}

func rational(t *testing.T, s string) *big.Rat {
	t.Helper()
	v, ok := new(big.Rat).SetString(s)
	if !ok {
		t.Fatal("invalid rational fixture")
	}
	return v
}
func ratInt(v uint64) *big.Rat { return new(big.Rat).SetInt(new(big.Int).SetUint64(v)) }

// Test-only rational interpretation; this is not a production pricing engine.
func pricingExample(t *testing.T, block *modelusage.Block, c pricingCase) (*big.Rat, *big.Rat, string) {
	t.Helper()
	lower, upper := new(big.Rat), new(big.Rat)
	for id := range block.Models {
		if modelusage.IsReserved(id) {
			return nil, nil, "unpriced_reserved_bucket"
		}
	}
	f, g, share := rational(t, c.InputFactor), rational(t, c.OutputFactor), rational(t, c.Fraction)
	for _, row := range block.Models {
		for _, bin := range row.Histogram {
			b := bin.Complete
			if b.Requests == 0 {
				continue
			}
			lo, hi := new(big.Rat).Mul(f, ratInt(*b.InputMin)), new(big.Rat).Mul(f, ratInt(*b.InputMax))
			var previous *uint64
			intersecting := []map[string]string{}
			covered := false
			for _, tier := range c.Tiers {
				if (previous == nil || hi.Cmp(ratInt(*previous)) > 0) && (tier.Upper == nil || lo.Cmp(ratInt(*tier.Upper)) <= 0) {
					if len(tier.Rates) != 4 {
						return nil, nil, "unknown_intersecting_tier"
					}
					intersecting = append(intersecting, tier.Rates)
				}
				if tier.Upper == nil || hi.Cmp(ratInt(*tier.Upper)) <= 0 {
					covered = true
					break
				}
				previous = tier.Upper
			}
			if !covered {
				return nil, nil, "unknown_intersecting_tier"
			}
			components := map[string]*big.Rat{"uncached_input": new(big.Rat).Mul(f, ratInt(b.InputTokens-b.CacheReadInputTokens-b.CacheWriteInputTokens)), "cache_read": new(big.Rat).Mul(f, ratInt(b.CacheReadInputTokens)), "cache_write": new(big.Rat).Mul(f, ratInt(b.CacheWriteInputTokens)), "output": new(big.Rat).Mul(g, ratInt(b.OutputTokens))}
			for name, v := range components {
				var low, high *big.Rat
				for _, rates := range intersecting {
					s, ok := rates[name]
					if !ok {
						return nil, nil, "unknown_intersecting_tier"
					}
					r := rational(t, s)
					if low == nil || r.Cmp(low) < 0 {
						low = r
					}
					if high == nil || r.Cmp(high) > 0 {
						high = r
					}
				}
				lower.Add(lower, new(big.Rat).Mul(share, new(big.Rat).Mul(v, low)))
				upper.Add(upper, new(big.Rat).Mul(share, new(big.Rat).Mul(v, high)))
			}
		}
	}
	return lower, upper, ""
}
func TestModelUsagePricingInterpretationVectors(t *testing.T) {
	var suite struct{ Cases []pricingCase }
	vectorJSON(t, "cases/pricing.json", &suite)
	for _, c := range suite.Cases {
		t.Run(c.Name, func(t *testing.T) {
			doc := vectorDoc(t, c.Fixture)
			lo, hi, reject := pricingExample(t, doc.ModelUsage, c)
			if reject != c.Reject {
				t.Fatal("rejection", reject)
			}
			if reject == "" && (lo.Cmp(rational(t, c.Expected["lower"])) != 0 || hi.Cmp(rational(t, c.Expected["upper"])) != 0) {
				t.Fatalf("bounds %s %s", lo, hi)
			}
		})
	}
}

func TestModelUsagePublicDataManifest(t *testing.T) {
	manifest := sha256.Sum256(vectorBytes(t, "manifest.json"))
	if hex.EncodeToString(manifest[:]) != "ffabccaaa18b6fb38d947d463abbf2d644f85767efe808b1a820fa56722e2562" {
		t.Fatal("manifest changed")
	}
	var public struct {
		PayloadFiles int `json:"payload_files"`
		WireFixtures int `json:"wire_fixtures"`
		Files        []struct {
			Path   string
			Size   int
			SHA256 string
		}
	}
	vectorJSON(t, "manifest.json", &public)
	if public.PayloadFiles != 82 || len(public.Files) != 82 || public.WireFixtures != 65 {
		t.Fatal("manifest counts")
	}
	for _, record := range public.Files {
		data := vectorBytes(t, record.Path)
		sum := sha256.Sum256(data)
		if len(data) != record.Size || hex.EncodeToString(sum[:]) != record.SHA256 {
			t.Fatal("payload changed", record.Path)
		}
	}
	paths, err := filepath.Glob(filepath.Join(modelVectors, "cases/*.json"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var suite struct{ Cases []json.RawMessage }
		if err := json.Unmarshal(data, &suite); err != nil {
			t.Fatal(err)
		}
		n += len(suite.Cases)
	}
	if n != 112 {
		t.Fatal("case count", n)
	}
}

func TestModelUsageAtomicCloneAndPrivacy(t *testing.T) {
	doc := vectorDoc(t, "fixtures/max_integer.json")
	before, _ := doc.MarshalBinary()
	row := doc.ModelUsage.Models["model-a"]
	other := *doc.ModelUsage
	other.Models = map[string]modelusage.Model{"model-b": row}
	if combined, err := modelusage.Merge(doc.ModelUsage, &other); err == nil || combined != nil || modelReason(err) != "overflow" {
		t.Fatal("disjoint-name overflow must fail atomically", err)
	}
	one := uint64(1)
	_, err := modelusage.AddAttempt(row, modelusage.Attempt{Input: modelusage.Observation{Value: &one}, Output: modelusage.Observation{Value: &one}, CacheRead: modelusage.Observation{Value: &one}, CacheWrite: modelusage.Observation{Value: &one}})
	if err == nil || modelReason(err) != "overflow" {
		t.Fatal(err)
	}
	after, _ := doc.MarshalBinary()
	if !bytes.Equal(before, after) {
		t.Fatal("failed addition mutated input")
	}
	clone, err := modelusage.Add(modelusage.Model{}, row)
	if err != nil {
		t.Fatal(err)
	}
	*clone.Complete.InputMin = 0
	*clone.Histogram[3].Complete.InputMin = 0
	after, _ = doc.MarshalBinary()
	if !bytes.Equal(before, after) {
		t.Fatal("clone aliases extrema")
	}
	var cfg struct{ Routing modelusage.Routing }
	vectorJSON(t, "contract.json", &cfg)
	for _, name := range []string{"__overflow__", "MODEL_PRIVACY_SENTINEL@example.invalid"} {
		_, err := modelusage.ContractID("acct-v1", map[string]string{"source": name}, cfg.Routing)
		if err == nil || strings.Contains(err.Error(), "PRIVACY_SENTINEL") {
			t.Fatal("unsafe contract diagnostic")
		}
	}
	_, err = modelusage.ContractID("acct-v1", map[string]string{string([]byte{0xff}): "model-a"}, cfg.Routing)
	if err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if reflect.DeepEqual(row, clone) {
		t.Fatal("clone control")
	}
}

func TestModelUsagePreallocationBounds(t *testing.T) {
	data := append([]byte(`{"model_usage":{"models":`), bytes.Repeat([]byte{'['}, 100000)...)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := Parse(data)
	runtime.ReadMemStats(&after)
	if err == nil || after.TotalAlloc-before.TotalAlloc > 1<<20 {
		t.Fatalf("shape allocation %d, %v", after.TotalAlloc-before.TotalAlloc, err)
	}
}

func TestModelUsagePreflightMalformedEncoding(t *testing.T) {
	valid := vectorBytes(t, "fixtures/one.json")
	invalid := map[string][]byte{
		"invalid_utf8":         bytes.Replace(valid, []byte("key-v1"), []byte{0xff}, 1),
		"nan":                  bytes.Replace(valid, []byte(`"sequence":1`), []byte(`"sequence":NaN`), 1),
		"infinity":             bytes.Replace(valid, []byte(`"sequence":1`), []byte(`"sequence":Infinity`), 1),
		"negative_infinity":    bytes.Replace(valid, []byte(`"sequence":1`), []byte(`"sequence":-Infinity`), 1),
		"unpaired_high":        bytes.Replace(valid, []byte("key-v1"), []byte(`\ud800`), 1),
		"unpaired_low":         bytes.Replace(valid, []byte("key-v1"), []byte(`\udfff`), 1),
		"wrong_surrogate_pair": bytes.Replace(valid, []byte("key-v1"), []byte(`\ud800\u0041`), 1),
	}
	for name, data := range invalid {
		t.Run(name, func(t *testing.T) {
			if present, err := HasModelUsage(data); err == nil || present {
				t.Fatalf("preflight accepted malformed encoding: %v", err)
			}
			if _, err := Parse(data); err == nil {
				t.Fatal("parse accepted malformed encoding")
			}
		})
	}
	if present, err := HasModelUsage(valid); err != nil || !present {
		t.Fatal("valid control", err)
	}
	for _, replacement := range []string{`\ud83d\ude00`, `\\ud800`, `\ufffd`, "\U0001f600"} {
		data := bytes.Replace(valid, []byte("key-v1"), []byte(replacement), 1)
		if present, err := HasModelUsage(data); err != nil || !present {
			t.Fatalf("valid Unicode rejected: %v", err)
		}
	}
}

func FuzzModelUsage(f *testing.F) {
	for _, name := range []string{"one", "empty", "max_integer", "unknown_version", "null_block", "bad_cohort_sum"} {
		f.Add(vectorBytes(f, "fixtures/"+name+".json"))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Parse(data)
		if err != nil {
			return
		}
		canonical, err := doc.MarshalBinary()
		if err != nil || !bytes.Equal(canonical, data) {
			t.Fatal("round trip")
		}
		one, e1 := Combine([]Envelope{doc}, []string{doc.ProducerID})
		two, e2 := Combine([]Envelope{doc, doc}, []string{doc.ProducerID})
		if e1 != nil || e2 != nil || !reflect.DeepEqual(one, two) {
			t.Fatal("replay")
		}
	})
}

func BenchmarkModelUsageParse(b *testing.B) {
	for _, n := range []int{1, 2000, 5000} {
		b.Run(fmt.Sprintf("models_%d", n), func(b *testing.B) {
			e := vectorDoc(b, "fixtures/one.json")
			row := e.ModelUsage.Models["model-a"]
			e.ModelUsage.Models = map[string]modelusage.Model{}
			for i := 0; i < n; i++ {
				e.ModelUsage.Models[fmt.Sprintf("m%04d", i)] = row
			}
			for k, v := range e.Counters {
				e.Counters[k] = v * uint64(n)
			}
			wire, err := json.Marshal(e)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := Parse(wire)
				if (err != nil) != (len(wire) > MaxBytes) {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(len(wire)), "wire_bytes")
		})
	}
}
