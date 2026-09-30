// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

type event struct {
	key    string
	tokens int64
}

func main() {
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_SECRET")
	if err != nil {
		log.Fatal(err)
	}
	// Two workers own disjoint requests in one completed window.
	workers := [][]event{
		{{"support", 1240}, {"research", 8900}},
		{{"support", 980}, {"coding", 3600}},
	}
	if err := runWindow(secret, workers, []string{"support", "research", "coding"}, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// runWindow owns one pair of sketches. Call it separately for each closed window.
// Worker count and the existing authorized key catalog are application-bounded.
func runWindow(secret sketchhash.Secret, workers [][]event, knownKeys []string, out io.Writer) error {
	distinct, err := hllpp.New(hllpp.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		return err
	}
	top, err := frequentitems.New(frequentitems.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	observe := func(e event) error {
		digest, err := hashKey(secret, e.key)
		if err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if err := top.AddHash(digest, e.tokens); err != nil {
			return err
		}
		distinct.AddHash(digest)
		return nil
	}

	// Any failed update invalidates this window; never publish partial results.
	errors := make(chan error, len(workers))
	for _, batch := range workers {
		go func() {
			for _, e := range batch {
				if err := observe(e); err != nil {
					errors <- err
					return
				}
			}
			errors <- nil
		}()
	}
	var firstErr error
	for range workers {
		if err := <-errors; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	mu.Lock()
	estimate := distinct.Estimate()
	items, err := top.FrequentItems(frequentitems.NoFalseNegatives)
	mu.Unlock()
	if err != nil {
		return err
	}
	items = items[:min(10, len(items))]
	// Re-hash an existing authorized catalog, retaining only the top-10 matches.
	names := make(map[uint64]string, len(items))
	for _, key := range knownKeys {
		digest, err := hashKey(secret, key)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Hash == digest {
				names[digest] = key
			}
		}
	}
	if _, err := fmt.Fprintf(out, "distinct keys: %.0f\n", estimate); err != nil {
		return err
	}
	for _, item := range items {
		name, ok := names[item.Hash]
		if !ok {
			name = fmt.Sprintf("hash:%016x", item.Hash)
		}
		if _, err := fmt.Fprintf(out, "%s tokens=%d bounds=[%d,%d]\n",
			name, item.Estimate, item.LowerBound, item.UpperBound); err != nil {
			return err
		}
	}
	return nil
}

func hashKey(secret sketchhash.Secret, key string) (uint64, error) {
	canonical, err := canon.CanonicalizeString(canon.TextV1, key)
	if err != nil {
		return 0, err
	}
	return sketchhash.Hash64(secret, sketchhash.UserV1, canonical)
}
