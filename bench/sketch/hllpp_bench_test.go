// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package sketchbench

import (
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

const hllppBenchSampleSize = 4096

func BenchmarkHLLPPAddHash_AmortizedFromEmpty(b *testing.B) {
	sketch := newBenchHLLPP(b)
	hashes := benchHashes(hllppBenchSampleSize, 0x0100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sketch.AddHash(hashes[i&(hllppBenchSampleSize-1)])
	}
}

func BenchmarkHLLPPAddHash_SparseSteadyState(b *testing.B) {
	sketch := newBenchHLLPP(b)
	hashes := benchHashes(hllppBenchSampleSize, 0x1200)
	for i := 0; i < 128; i++ {
		sketch.AddHash(hashes[i])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sketch.AddHash(hashes[i&127])
	}
}

func BenchmarkHLLPPAddHash_DenseSteadyState(b *testing.B) {
	sketch := newBenchHLLPP(b)
	hashes := benchHashes(hllppBenchSampleSize, 0x3400)
	for _, hash := range hashes {
		sketch.AddHash(hash)
	}
	sketch.ForceDense()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sketch.AddHash(hashes[i&(hllppBenchSampleSize-1)])
	}
}

func newBenchHLLPP(b *testing.B) *hllpp.Sketch {
	b.Helper()

	sketch, err := hllpp.New(hllpp.ProfileSmall, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		b.Fatalf("new HLL++ benchmark sketch: %v", err)
	}

	return sketch
}

func benchHashes(count int, seed uint64) []uint64 {
	values := make([]uint64, count)
	for i := range values {
		values[i] = benchSplitmix64(seed + uint64(i))
	}

	return values
}

func benchSplitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
