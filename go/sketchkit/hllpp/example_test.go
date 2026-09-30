// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package hllpp_test

import (
	"fmt"
	"os"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
)

func Example() {
	// Public test fixture only. Production callers supply a protected secret.
	if err := os.Setenv("HLLPP_EXAMPLE_SECRET", "example-only-not-a-deployment-secret"); err != nil {
		panic(err)
	}
	defer func() {
		if err := os.Unsetenv("HLLPP_EXAMPLE_SECRET"); err != nil {
			panic(err)
		}
	}()
	secret, err := sketchhash.SecretFromEnv("HLLPP_EXAMPLE_SECRET")
	if err != nil {
		panic(err)
	}
	sketch, err := hllpp.New(hllpp.ProfileSmall, sketchhash.UserV1, sketchhash.HMACSHA25664)
	if err != nil {
		panic(err)
	}
	for _, user := range []string{"alice", "bob", "alice"} {
		canonical, err := canon.CanonicalizeString(canon.TextV1, user)
		if err != nil {
			panic(err)
		}
		digest, err := sketchhash.Hash64(secret, sketchhash.UserV1, canonical)
		if err != nil {
			panic(err)
		}
		sketch.AddHash(digest)
	}
	fmt.Printf("distinct users: %.0f\n", sketch.Estimate())
	// Output: distinct users: 2
}
