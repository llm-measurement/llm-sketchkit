// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package fuzz_test

import (
	"bytes"
	"encoding"
	"math"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"google.golang.org/protobuf/proto"
)

func TestLargestProfileRoundTrips(t *testing.T) {
	h, _ := hllpp.New("default", sketchhash.RetrievalDocV1, sketchhash.HMACSHA25664)
	h.ForceDense()
	f, _ := frequentitems.New("default", sketchhash.RetrievalDocV1, sketchhash.HMACSHA25664)
	b, _ := bloom.New("default", sketchhash.RetrievalDocV1, sketchhash.HMACSHA25664)
	m, _ := minhash.New("k256", sketchhash.RetrievalDocV1, sketchhash.HMACSHA25664)
	cases := []struct {
		name   string
		value  encoding.BinaryMarshaler
		mutate func(*sketchpb.Sketch)
		parse  func([]byte) (encoding.BinaryMarshaler, error)
	}{
		{"hll_dense", h, func(w *sketchpb.Sketch) {
			for i := range w.GetHllpp().DenseRegisters {
				w.GetHllpp().DenseRegisters[i] = 50
			}
		}, func(d []byte) (encoding.BinaryMarshaler, error) { return hllpp.Parse(d) }},
		{"hll_sparse", h, func(w *sketchpb.Sketch) {
			w.Metadata.RepresentationMode = sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE
			w.GetHllpp().DenseRegisters = nil
			for i := uint32(0); i < 2048; i++ {
				w.GetHllpp().SparseRegisters = append(w.GetHllpp().SparseRegisters, &sketchpb.HllppSparseRegister{Index: (1 << 20) - 2048 + i, Value: 45})
			}
		}, func(d []byte) (encoding.BinaryMarshaler, error) { return hllpp.Parse(d) }},
		{"fi_full_width", f, func(w *sketchpb.Sketch) {
			body := w.GetFrequentItems()
			body.TotalWeight = math.MaxInt64
			body.MaxError = math.MaxInt64 - 1
			for i := uint64(0); i < 1024; i++ {
				body.Entries = append(body.Entries, &sketchpb.FrequentItemsEntry{Hash: (1 << 63) + i, Estimate: math.MaxInt64, Error: math.MaxInt64 - 1})
			}
		}, func(d []byte) (encoding.BinaryMarshaler, error) { return frequentitems.Parse(d) }},
		{"bloom_full_width", b, func(w *sketchpb.Sketch) {
			body := w.GetBloom()
			body.InsertedCount = math.MaxUint64
			for i := range body.Bitset {
				body.Bitset[i] = 0xff
			}
			if r := w.Metadata.GetBloomBitCount() % 8; r != 0 {
				body.Bitset[len(body.Bitset)-1] = byte((1 << r) - 1)
			}
		}, func(d []byte) (encoding.BinaryMarshaler, error) { return bloom.Parse(d) }},
		{"minhash_k256_full_width", m, func(w *sketchpb.Sketch) { w.GetMinhash().PopulatedCount = math.MaxUint64 }, func(d []byte) (encoding.BinaryMarshaler, error) { return minhash.Parse(d) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.value.MarshalBinary()
			if err != nil {
				t.Fatal(err)
			}
			w := new(sketchpb.Sketch)
			if err := proto.Unmarshal(data, w); err != nil {
				t.Fatal(err)
			}
			c.mutate(w)
			data, err = proto.MarshalOptions{Deterministic: true}.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := c.parse(data)
			if err != nil {
				t.Fatal(err)
			}
			out, err := parsed.MarshalBinary()
			if err != nil || !bytes.Equal(out, data) {
				t.Fatalf("not canonical: %v", err)
			}
			t.Logf("canonical bytes=%d", len(data))
		})
	}
}
