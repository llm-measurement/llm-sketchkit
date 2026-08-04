// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package sketchbench

import (
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func BenchmarkFrequentItemsAddHash_AmortizedSkewed(b *testing.B) {
	sketch := newBenchFrequentItems(b)
	hashes := benchHashes(4096, 0x5100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sketch.AddHash(hashes[i&4095], 1); err != nil {
			b.Fatalf("AddHash(): %v", err)
		}
	}
}

func BenchmarkFrequentItemsAddHash_TrackedSteadyState(b *testing.B) {
	sketch := newBenchFrequentItems(b)
	hashes := benchHashes(512, 0x5200)
	for _, hash := range hashes {
		if err := sketch.AddHash(hash, 1); err != nil {
			b.Fatalf("warm AddHash(): %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sketch.AddHash(hashes[i&511], 1); err != nil {
			b.Fatalf("AddHash(): %v", err)
		}
	}
}

func BenchmarkFrequentItemsAddHash_DropSteadyState(b *testing.B) {
	sketch := newBenchFrequentItems(b)
	heavy := benchHashes(20, 0x5300)
	for _, hash := range heavy {
		if err := sketch.AddHash(hash, 1_000_000); err != nil {
			b.Fatalf("warm heavy AddHash(): %v", err)
		}
	}
	tails := benchHashes(493, 0x5400)
	for _, hash := range tails {
		if err := sketch.AddHash(hash, 1); err != nil {
			b.Fatalf("warm tail AddHash(): %v", err)
		}
	}
	dropped := benchHashes(4096, 0x5500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sketch.AddHash(dropped[i&4095], 1); err != nil {
			b.Fatalf("AddHash(): %v", err)
		}
	}
}

func newBenchFrequentItems(b *testing.B) *frequentitems.Sketch {
	b.Helper()

	sketch, err := frequentitems.New(frequentitems.ProfileSmall, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		b.Fatalf("new frequent-items benchmark sketch: %v", err)
	}

	return sketch
}
