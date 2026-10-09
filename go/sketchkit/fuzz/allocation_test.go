// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package fuzz_test

import (
	"bytes"
	"errors"
	"runtime"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestAllocationBounds(t *testing.T) {
	parsers := []struct {
		name   string
		parse  func([]byte) error
		target error
	}{
		{"hllpp", func(b []byte) error { _, e := hllpp.Parse(b); return e }, hllpp.ErrInvalidWireEncoding},
		{"frequent_items", func(b []byte) error { _, e := frequentitems.Parse(b); return e }, frequentitems.ErrInvalidWireEncoding},
		{"bloom", func(b []byte) error { _, e := bloom.Parse(b); return e }, bloom.ErrInvalidWireEncoding},
		{"minhash", func(b []byte) error { _, e := minhash.Parse(b); return e }, minhash.ErrInvalidWireEncoding},
	}
	for _, field := range []protowire.Number{10, 11} {
		for _, size := range []int{64 << 10, 2 << 20} {
			body := bytes.Repeat([]byte{10, 0}, size/2)
			data := protowire.AppendBytes(protowire.AppendTag(nil, field, 2), body)
			for _, p := range parsers {
				t.Run(p.name, func(t *testing.T) {
					runtime.GC()
					var before, after runtime.MemStats
					runtime.ReadMemStats(&before)
					err := p.parse(data)
					runtime.ReadMemStats(&after)
					if !errors.Is(err, p.target) {
						t.Fatalf("wrong error category: %v", err)
					}
					if n := after.TotalAlloc - before.TotalAlloc; n > 8<<20 {
						t.Fatalf("allocated %d bytes", n)
					}
				})
			}
		}
	}
}

func TestEmptyOneofAmplification(t *testing.T) {
	data := bytes.Repeat([]byte{0x52, 0, 0x5a, 0}, 20000)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := bloom.Parse(data)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, bloom.ErrInvalidWireEncoding) {
		t.Fatal(err)
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 8<<20 {
		t.Fatalf("allocated %d bytes", n)
	}
}

func TestMixedBodyAllocationBudget(t *testing.T) {
	large, err := bloom.New(bloom.ProfileDefault, "prompt:v1", "hmac_sha256_64")
	if err != nil {
		t.Fatal(err)
	}
	suffix, err := large.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{2048, 2049, 32760} {
		var data []byte
		for _, field := range []protowire.Number{10, 11} {
			data = protowire.AppendBytes(protowire.AppendTag(data, field, 2), bytes.Repeat([]byte{10, 0}, count))
		}
		data = append(data, suffix...)
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := bloom.Parse(data)
		runtime.ReadMemStats(&after)
		if (err == nil) != (count == 2048) {
			t.Fatalf("count %d: %v", count, err)
		}
		if err != nil && !errors.Is(err, bloom.ErrInvalidWireEncoding) {
			t.Fatal(err)
		}
		if n := after.TotalAlloc - before.TotalAlloc; n > 8<<20 {
			t.Fatalf("allocated %d bytes", n)
		}
	}
}
