// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay and Codex

package canon_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

func TestUnicode15IdentityVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "vectors", "identity", "text_v1_unicode15.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Schema  int    `json:"schema_version"`
		Unicode string `json:"unicode_version"`
		Secret  string `json:"secret"`
		Cases   []struct {
			Name      string `json:"name"`
			Input     string `json:"input"`
			Canonical string `json:"canonical_hex"`
			Digest    string `json:"digest_hex"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	if vector.Schema != 1 || vector.Unicode != "15.0.0" || len(vector.Cases) == 0 {
		t.Fatal("invalid identity vector header")
	}
	t.Setenv("LLM_SKETCHKIT_IDENTITY_VECTOR_SECRET", vector.Secret)
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_IDENTITY_VECTOR_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range vector.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := canon.CanonicalizeString(canon.TextV1, c.Input)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(got) != c.Canonical {
				t.Fatalf("canonical bytes = %x, want %s", got, c.Canonical)
			}
			again, err := canon.Canonicalize(canon.TextV1, got)
			if err != nil || string(again) != string(got) {
				t.Fatalf("idempotence failed: %v", err)
			}
			digest, err := sketchhash.Digest64Hex(secret, sketchhash.PromptV1, got)
			if err != nil || digest != c.Digest {
				t.Fatalf("digest = %s, want %s: %v", digest, c.Digest, err)
			}
		})
	}
}
