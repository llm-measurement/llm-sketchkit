// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package summary

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
)

func fixture(t *testing.T, producer, epoch string, seq, count uint64) Envelope {
	t.Helper()
	h, err := hllpp.New("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	f, err := frequentitems.New("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	b, err := bloom.New("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	m, err := minhash.New("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= count; i++ {
		value := i * 0x9e3779b97f4a7c15
		h.AddHash(value)
		if err := f.AddHash(value, 1); err != nil {
			t.Fatal(err)
		}
		if err := b.AddHash(value); err != nil {
			t.Fatal(err)
		}
		if err := m.AddHash(value); err != nil {
			t.Fatal(err)
		}
	}
	sketches := map[string]Payload{}
	for kind, s := range map[string]interface{ MarshalBinary() ([]byte, error) }{"hllpp": h, "frequent_items": f, "bloom": b, "minhash": m} {
		data, err := s.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		sketches[kind] = Payload{Data: data, Kind: kind}
	}
	return Envelope{AccountingID: "test-v1", Counters: map[string]uint64{"requests": count}, EmittedAt: 120, Epoch: epoch, KeyID: "test-key-v1", ObservedEnd: 120, ObservedStart: 60, ProducerID: producer, ScopeID: "example", Sequence: seq, Sketches: sketches, Version: 1, WindowDuration: 60, WindowStart: 60}
}

func TestSharedVectors(t *testing.T) {
	data, err := os.ReadFile("../../../vectors/summaries/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name      string
			Documents [][]json.RawMessage
			Requests  uint64
			Missing   []string
			Error     bool
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			docs := []Envelope{}
			for _, fields := range v.Documents {
				var producer, epoch string
				var seq, count uint64
				for i, target := range []any{&producer, &epoch, &seq, &count} {
					if err := json.Unmarshal(fields[i], target); err != nil {
						t.Fatal(err)
					}
				}
				docs = append(docs, fixture(t, producer, epoch, seq, count))
			}
			result, err := Combine(docs, []string{"a", "b"})
			if v.Error {
				if err == nil {
					t.Fatal("accepted invalid combination")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.Counters["requests"] != v.Requests || !reflect.DeepEqual(result.Missing, v.Missing) {
				t.Fatalf("wrong result: %+v", result)
			}
			fi, err := frequentitems.Parse(result.Sketches["frequent_items"].Data)
			if err != nil {
				t.Fatal(err)
			}
			bl, err := bloom.Parse(result.Sketches["bloom"].Data)
			if err != nil {
				t.Fatal(err)
			}
			mh, err := minhash.Parse(result.Sketches["minhash"].Data)
			if err != nil {
				t.Fatal(err)
			}
			if uint64(fi.TotalWeight()) != v.Requests || bl.InsertedCount() != v.Requests || mh.PopulatedCount() != v.Requests {
				t.Fatal("replay inflated sketch state")
			}
			if v.Name == "two_producers" {
				combined := fixture(t, "combined", "offline", 1, 0)
				combined.Counters, combined.Sketches = result.Counters, result.Sketches
				encoded, err := combined.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				golden, err := os.ReadFile("../../../vectors/summaries/combined.json")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(encoded, golden) {
					t.Fatal("cross-language combined bytes differ")
				}
			}
		})
	}
}

func TestCanonicalAndUntrustedInputs(t *testing.T) {
	e := fixture(t, "a", "one", 1, 2)
	data, err := e.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("../../../vectors/summaries/envelope.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, golden) {
		t.Fatal("cross-language envelope bytes differ")
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e, parsed) {
		t.Fatal("round trip differs")
	}
	for _, bad := range [][]byte{append(append([]byte{}, data...), '\n'), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":true`), 1), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"unknown":0`), 1), []byte(`{`), bytes.Repeat([]byte(" "), MaxBytes+1)} {
		if _, err := Parse(bad); err == nil {
			t.Fatal("accepted malformed summary")
		}
	}
	for _, change := range []func(*Envelope){func(e *Envelope) { e.ScopeID = "other" }, func(e *Envelope) { e.KeyID = "other" }, func(e *Envelope) { e.AccountingID = "other" }, func(e *Envelope) { e.Sketches["hllpp"] = Payload{Kind: "hllpp", Data: []byte("bad")} }} {
		other := fixture(t, "b", "two", 1, 2)
		change(&other)
		if _, err := Combine([]Envelope{e, other}, []string{"a", "b"}); err == nil {
			t.Fatal("accepted incompatible summary")
		}
	}
}

func TestRestartsCoverageAndAtomicity(t *testing.T) {
	a, b := fixture(t, "a", "one", 1, 2), fixture(t, "a", "two", 1, 3)
	a.ObservedEnd = 90
	b.ObservedStart = 90
	before, _ := a.MarshalBinary()
	r, err := Combine([]Envelope{b, a, a}, []string{"a", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Counters["requests"] != 5 || len(r.Partial) != 0 || len(r.Missing) != 1 {
		t.Fatalf("bad restart accounting: %+v", r)
	}
	b.ObservedStart = 91
	r, err = Combine([]Envelope{b, a}, []string{"a"})
	if err != nil || len(r.Partial) != 1 {
		t.Fatalf("gap not reported: %v %+v", err, r)
	}
	b.ObservedStart = 89
	if _, err := Combine([]Envelope{b, a}, []string{"a"}); err == nil {
		t.Fatal("overlap accepted")
	}
	after, _ := a.MarshalBinary()
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
	b = fixture(t, "b", "two", 1, 2)
	b.Counters["requests"] = math.MaxInt64
	if _, err := Combine([]Envelope{a, b}, []string{"a", "b"}); err == nil {
		t.Fatal("counter overflow accepted")
	}
	b = fixture(t, "b", "two", 1, 2)
	b.WindowStart += 60
	b.ObservedStart += 60
	b.ObservedEnd += 60
	b.EmittedAt += 60
	if err := Compatible(a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := Combine([]Envelope{a, b}, []string{"a", "b"}); err == nil {
		t.Fatal("different windows combined")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if e, err := Parse(data); err == nil {
			out, err := e.MarshalBinary()
			if err != nil || !bytes.Equal(data, out) {
				t.Fatal("unstable accepted encoding")
			}
		}
	})
}
