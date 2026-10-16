// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package canon_test

import (
	"encoding/hex"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestLegacy02SupplementaryIdentity(t *testing.T) {
	t.Setenv("SKETCHKIT_LEGACY_TEST_KEY", "backport-identity-review-synthetic-v1")
	secret, err := sketchhash.SecretFromEnv("SKETCHKIT_LEGACY_TEST_KEY")
	if err != nil {
		t.Fatal(err)
	}
	// These are frozen v0.2.3 outputs, including its normalization quirks.
	for _, tc := range []struct{ name, input, canonical, digest string }{
		{"supplementary-recomposition", "\U00011f41\u0301", "e1bd85", "1c14b04f2d6942e9"},
		{"supplementary-alias", "\U00010041\u0300", "c380", "5f29069fa987098d"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canon.CanonicalizeString(canon.TextV1, tc.input)
			if err != nil || hex.EncodeToString(got) != tc.canonical {
				t.Fatalf("legacy canonical bytes = %x, want %s: %v", got, tc.canonical, err)
			}
			bytes, err := canon.Canonicalize(canon.TextV1, []byte(tc.input))
			if err != nil || string(bytes) != string(got) {
				t.Fatalf("byte/string API mismatch: %v", err)
			}
			digest, err := sketchhash.Digest64Hex(secret, sketchhash.PromptV1, got)
			if err != nil || digest != tc.digest {
				t.Fatalf("legacy digest = %s, want %s: %v", digest, tc.digest, err)
			}
		})
	}
}
