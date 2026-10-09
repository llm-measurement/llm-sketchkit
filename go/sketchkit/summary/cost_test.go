// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"fmt"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func costBatch(t testing.TB) []Envelope {
	t.Helper()
	s, err := frequentitems.New("default", sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	for i := uint64(1); i <= 1024; i++ {
		if err := s.AddHash(i*0x9e3779b97f4a7c15, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	full, err := s.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	docs := make([]Envelope, 1024)
	for i := range docs {
		data := empty
		if i == 0 {
			data = full
		}
		docs[i] = Envelope{AccountingID: "test", Counters: map[string]uint64{"requests": 1},
			EmittedAt: 1024, Epoch: fmt.Sprintf("e%04d", i), KeyID: "test-key",
			ObservedStart: int64(i), ObservedEnd: int64(i + 1), ProducerID: "a",
			ScopeID: "test", Sequence: 1, Sketches: map[string]Payload{}, Version: 1, WindowDuration: 1024}
		for j := 0; j < 16; j++ {
			docs[i].Sketches[fmt.Sprintf("s%02d", j)] = Payload{Data: data, Kind: "frequent_items"}
		}
	}
	return docs
}

func TestCombineCost(t *testing.T) {
	docs := costBatch(t)
	// A deterministic allocation budget catches the former repeated parse and
	// serialization of the first full snapshot without relying on runner speed.
	allocs := testing.AllocsPerRun(1, func() {
		result, err := Combine(docs, []string{"a"})
		if err != nil || result.Counters["requests"] != 1024 {
			t.Fatalf("combine: %v", err)
		}
	})
	if allocs > 1500000 {
		t.Fatalf("allocation count %.0f exceeds budget", allocs)
	}
	t.Logf("allocations: %.0f", allocs)
}

func BenchmarkCombine1024(b *testing.B) {
	docs := costBatch(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := Combine(docs, []string{"a"}); err != nil {
			b.Fatal(err)
		}
	}
}
