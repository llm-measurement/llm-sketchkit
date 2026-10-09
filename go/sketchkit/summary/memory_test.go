// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package summary

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestCombineRetainedStateMemory(t *testing.T) {
	docs := costBatch(t)[:128]
	before, err := json.Marshal(docs)
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	done, peak := make(chan struct{}), make(chan uint64)
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		maximum := baseline.HeapAlloc
		for {
			var current runtime.MemStats
			runtime.ReadMemStats(&current)
			maximum = max(maximum, current.HeapAlloc)
			select {
			case <-done:
				peak <- maximum
				return
			case <-ticker.C:
			}
		}
	}()
	result, err := Combine(docs, []string{"a"})
	close(done)
	retained := <-peak - baseline.HeapAlloc
	if err != nil || result.Counters["requests"] != 128 {
		t.Fatalf("combine: %v", err)
	}
	// The former per-document state slice retains over 200 MiB for this batch.
	if retained > 64<<20 {
		t.Fatalf("peak additional heap %d exceeds 64 MiB", retained)
	}
	after, err := json.Marshal(docs)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("inputs changed: %v", err)
	}
	t.Logf("peak additional heap: %d bytes", retained)
}

func TestCombineDeferredAccumulationErrors(t *testing.T) {
	for _, later := range []string{"none", "version", "wire", "contract", "counter"} {
		t.Run(later, func(t *testing.T) {
			docs := costBatch(t)[:4]
			for i := range docs {
				s, err := frequentitems.New("micro", sketchhash.PromptV1, sketchhash.HMACSHA25664)
				if err != nil {
					t.Fatal(err)
				}
				weight := int64(1)
				if i == 0 {
					weight = math.MaxInt64
				}
				if err := s.AddHash(1, weight); err != nil {
					t.Fatal(err)
				}
				data, err := s.MarshalBinary()
				if err != nil {
					t.Fatal(err)
				}
				docs[i].Sketches = map[string]Payload{"fi": {Kind: "frequent_items", Data: data}}
			}
			want := ""
			switch later {
			case "version":
				docs[3].Version = 0
				want = "invalid summary version or sequence"
			case "wire":
				docs[3].Sketches["fi"] = Payload{Kind: "frequent_items", Data: []byte{0xff}}
				want = "invalid summary sketch state"
			case "contract":
				docs[3].KeyID = "other"
				want = "incompatible summary measurement contract"
			case "counter":
				docs[3].Counters["requests"] = math.MaxUint64
				want = "invalid summary counter"
			}
			before, err := json.Marshal(docs)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Combine(docs, []string{"a"})
			if later == "none" {
				if !errors.Is(err, frequentitems.ErrWeightOverflow) {
					t.Fatalf("want sketch overflow, got %v", err)
				}
			} else if err == nil || err.Error() != want {
				t.Fatalf("want %q, got %v", want, err)
			}
			after, marshalErr := json.Marshal(docs)
			if marshalErr != nil || !bytes.Equal(before, after) {
				t.Fatalf("failed combine changed inputs: %v", marshalErr)
			}
		})
	}
}
