// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package hashbench

import (
	"bytes"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

var hash64Sink uint64

func BenchmarkHMACSHA25664_64B(b *testing.B) {
	benchmarkHMACSHA25664(b, 64)
}

func BenchmarkHMACSHA25664_1KB(b *testing.B) {
	benchmarkHMACSHA25664(b, 1024)
}

func benchmarkHMACSHA25664(b *testing.B, inputSize int) {
	b.Helper()
	b.Setenv("LLM_SKETCHKIT_BENCH_SECRET", "benchmark-vector-secret")
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_BENCH_SECRET")
	if err != nil {
		b.Fatalf("SecretFromEnv(): %v", err)
	}

	input := bytes.Repeat([]byte("a"), inputSize)
	if len(input) != inputSize {
		b.Fatalf("benchmark input length = %d, want %d", len(input), inputSize)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()

	var sink uint64
	for i := 0; i < b.N; i++ {
		value, err := sketchhash.Hash64(secret, sketchhash.PromptV1, input)
		if err != nil {
			b.Fatalf("Hash64(): %v", err)
		}
		sink ^= value
	}

	hash64Sink = sink
}
