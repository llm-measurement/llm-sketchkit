// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

func TestIntervalVectors(t *testing.T) {
	data, err := os.ReadFile("../../../vectors/validation/summary_intervals.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name            string
			Repeat          int
			Start, End      int64
			Count           uint64
			SketchCount     uint64 `json:"sketch_count"`
			Sketches, Error bool
			Suffix          string
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors.Cases {
		t.Run(v.Name, func(t *testing.T) {
			docs := []Envelope{}
			for i := 0; i < v.Repeat; i++ {
				e := fixture(t, "a", fmt.Sprintf("e%d%s", i, v.Suffix), 1, v.Count)
				e.ObservedStart, e.ObservedEnd = v.Start, v.End
				if !v.Sketches {
					e.Sketches = map[string]Payload{}
				} else {
					e.Sketches = fixture(t, "a", "sketch", 1, v.SketchCount).Sketches
				}
				docs = append(docs, e)
			}
			if v.Suffix != "" {
				e := fixture(t, "a", "e0m", 1, 2)
				e.ObservedStart = 90
				e.Sketches = map[string]Payload{}
				docs = append(docs, e)
			}
			_, err := Combine(docs, []string{"a"})
			if (err != nil) != v.Error {
				t.Fatalf("error = %v, expected rejection %v", err, v.Error)
			}
		})
	}
}

func TestZeroLengthSketchState(t *testing.T) {
	empty := fixture(t, "a", "e", 1, 0)
	full := fixture(t, "a", "e", 1, 1)
	type testCase struct {
		name    string
		payload Payload
		empty   bool
	}
	var cases []testCase
	for _, kind := range []string{"hllpp", "frequent_items", "bloom", "minhash"} {
		cases = append(cases, testCase{"empty_" + kind, empty.Sketches[kind], true})
		cases = append(cases, testCase{"nonempty_" + kind, full.Sketches[kind], false})
	}
	for _, v := range []struct {
		name, kind string
		empty      bool
		mutate     func(*sketchpb.Sketch)
	}{
		{"empty_dense_hll", "hllpp", true, func(p *sketchpb.Sketch) {
			p.Metadata.RepresentationMode = sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE
			p.GetHllpp().DenseRegisters = make([]byte, 1<<p.Metadata.GetHllppNormalPrecision())
		}},
		{"nonempty_dense_hll", "hllpp", false, func(p *sketchpb.Sketch) {
			p.Metadata.RepresentationMode = sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE
			p.GetHllpp().DenseRegisters = make([]byte, 1<<p.Metadata.GetHllppNormalPrecision())
			p.GetHllpp().DenseRegisters[0] = 1
		}},
		{"fi_weight_without_entries", "frequent_items", false, func(p *sketchpb.Sketch) {
			p.GetFrequentItems().TotalWeight = 1
			p.GetFrequentItems().MaxError = 1
		}},
		{"bloom_count_without_bits", "bloom", false, func(p *sketchpb.Sketch) {
			p.GetBloom().InsertedCount = 1
		}},
		{"bloom_bits_without_count", "bloom", false, func(p *sketchpb.Sketch) {
			p.GetBloom().Bitset[0] = 1
		}},
		{"minhash_count_without_signature", "minhash", false, func(p *sketchpb.Sketch) {
			p.GetMinhash().PopulatedCount = 1
		}},
	} {
		payload := empty.Sketches[v.kind]
		var message sketchpb.Sketch
		if err := proto.Unmarshal(payload.Data, &message); err != nil {
			t.Fatal(err)
		}
		v.mutate(&message)
		var err error
		payload.Data, err = proto.MarshalOptions{Deterministic: true}.Marshal(&message)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, testCase{v.name, payload, v.empty})
	}
	for _, v := range cases {
		t.Run(v.name, func(t *testing.T) {
			e := fixture(t, "a", "e", 1, 0)
			e.Sketches = map[string]Payload{v.payload.Kind: v.payload}
			if err := e.Validate(); err != nil {
				t.Fatalf("positive-length control: %v", err)
			}
			e.ObservedStart, e.ObservedEnd = 90, 90
			if err := e.Validate(); (err == nil) != v.empty {
				t.Fatalf("Validate: %v, empty=%v", err, v.empty)
			}
			if _, err := e.MarshalBinary(); (err == nil) != v.empty {
				t.Fatalf("MarshalBinary: %v, empty=%v", err, v.empty)
			}
			wire, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Parse(wire); (err == nil) != v.empty {
				t.Fatalf("Parse: %v, empty=%v", err, v.empty)
			}
			if err := Compatible(e, e); (err == nil) != v.empty {
				t.Fatalf("Compatible: %v, empty=%v", err, v.empty)
			}
			result, err := Combine([]Envelope{e}, []string{"a"})
			if (err == nil) != v.empty {
				t.Fatalf("Combine: %v, empty=%v", err, v.empty)
			}
			if v.empty && !bytes.Equal(result.Sketches[v.payload.Kind].Data, v.payload.Data) {
				t.Fatal("Combine changed empty sketch bytes")
			}
		})
	}
}
