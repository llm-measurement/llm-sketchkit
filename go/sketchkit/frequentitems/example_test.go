// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package frequentitems_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func Example() {
	// Public test fixture only. Production callers supply a protected secret.
	if err := os.Setenv("FI_EXAMPLE_SECRET", "example-only-not-a-deployment-secret"); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Unsetenv("FI_EXAMPLE_SECRET"); err != nil {
			panic(err)
		}
	}()
	secret, err := sketchhash.SecretFromEnv("FI_EXAMPLE_SECRET")
	if err != nil {
		panic(err)
	}
	sketch, err := frequentitems.New(frequentitems.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		panic(err)
	}
	for _, tokens := range []int64{1240, 980} {
		canonical, err := canon.CanonicalizeString(canon.TextV1, "support")
		if err != nil {
			panic(err)
		}
		digest, err := sketchhash.Hash64(secret, sketchhash.UserV1, canonical)
		if err != nil {
			panic(err)
		}
		if err := sketch.AddHash(digest, tokens); err != nil {
			panic(err)
		}
	}
	items, err := sketch.FrequentItems(frequentitems.NoFalseNegatives)
	if err != nil {
		panic(err)
	}
	for _, item := range items {
		fmt.Printf("tokens=%d bounds=[%d,%d]\n", item.Estimate, item.LowerBound, item.UpperBound)
	}
	// Output: tokens=2220 bounds=[2220,2220]
}

func TestFrequentItemsOrder(t *testing.T) {
	sketch, err := frequentitems.New(frequentitems.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		hash   uint64
		weight int64
	}{{3, 20}, {2, 30}, {1, 20}} {
		if err := sketch.AddHash(input.hash, input.weight); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []frequentitems.QueryMode{frequentitems.NoFalseNegatives, frequentitems.NoFalsePositives} {
		items, err := sketch.FrequentItems(mode)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 3 || items[0].Hash != 2 || items[1].Hash != 1 || items[2].Hash != 3 {
			t.Fatalf("unexpected query order: %v", items)
		}
	}
}
