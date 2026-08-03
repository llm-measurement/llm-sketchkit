// Code authors: Vijay Erramilli and Codex
package hash_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/canon"
	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
)

type hashVector struct {
	Name             string `json:"name"`
	Canonicalization string `json:"canonicalization"`
	Input            struct {
		Encoding string `json:"encoding"`
		Value    string `json:"value"`
	} `json:"input"`
	CanonicalBytesHex string `json:"canonical_bytes_hex"`
	Domain            string `json:"domain"`
	HashAlgo          string `json:"hash_algo"`
	Secret            struct {
		UTF8 string `json:"utf8"`
		Hex  string `json:"hex"`
	} `json:"secret"`
	Expected struct {
		Digest64Hex  string `json:"digest64_hex"`
		Digest64Uint uint64 `json:"digest64_uint"`
	} `json:"expected"`
}

func TestHashVectors(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "hash", "*.json"))
	if err != nil {
		t.Fatalf("glob hash vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no hash vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			vector := readHashVector(t, path)
			if vector.Input.Encoding != "utf8" {
				t.Fatalf("unsupported input encoding %q", vector.Input.Encoding)
			}
			if vector.HashAlgo != string(sketchhash.HMACSHA25664) {
				t.Fatalf("unsupported hash algorithm %q", vector.HashAlgo)
			}

			canonicalBytes, err := canon.CanonicalizeString(
				canon.Profile(vector.Canonicalization),
				vector.Input.Value,
			)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got := hex.EncodeToString(canonicalBytes); got != vector.CanonicalBytesHex {
				t.Fatalf("canonical bytes = %s, want %s", got, vector.CanonicalBytesHex)
			}

			envName := "LLM_SKETCHKIT_HASH_VECTOR_SECRET"
			secretValue := vector.Secret.UTF8
			if secretValue == "" {
				t.Fatalf("hash vector %q does not use env-compatible utf8 secret", vector.Name)
			}
			t.Setenv(envName, secretValue)
			secret, err := sketchhash.SecretFromEnv(envName)
			if err != nil {
				t.Fatalf("SecretFromEnv(): %v", err)
			}

			digestHex, err := sketchhash.Digest64Hex(
				secret,
				sketchhash.Domain(vector.Domain),
				canonicalBytes,
			)
			if err != nil {
				t.Fatalf("Digest64Hex(): %v", err)
			}
			if digestHex != vector.Expected.Digest64Hex {
				t.Fatalf("digest hex = %s, want %s", digestHex, vector.Expected.Digest64Hex)
			}

			hash64, err := sketchhash.Hash64(
				secret,
				sketchhash.Domain(vector.Domain),
				canonicalBytes,
			)
			if err != nil {
				t.Fatalf("Hash64(): %v", err)
			}
			if hash64 != vector.Expected.Digest64Uint {
				t.Fatalf("hash uint = %d, want %d", hash64, vector.Expected.Digest64Uint)
			}
		})
	}
}

func TestDomainSeparation(t *testing.T) {
	t.Setenv("LLM_SKETCHKIT_HASH_TEST_SECRET", "domain-vector-secret")
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_HASH_TEST_SECRET")
	if err != nil {
		t.Fatalf("SecretFromEnv(): %v", err)
	}

	canonicalBytes, err := canon.CanonicalizeString(canon.TextV1, "same value")
	if err != nil {
		t.Fatalf("CanonicalizeString(): %v", err)
	}

	promptHash, err := sketchhash.Hash64(secret, sketchhash.PromptV1, canonicalBytes)
	if err != nil {
		t.Fatalf("Hash64(prompt): %v", err)
	}
	userHash, err := sketchhash.Hash64(secret, sketchhash.UserV1, canonicalBytes)
	if err != nil {
		t.Fatalf("Hash64(user): %v", err)
	}
	if promptHash == userHash {
		t.Fatal("same input and secret produced identical hashes across domains")
	}
}

func TestSecretFromEnv(t *testing.T) {
	t.Setenv("LLM_SKETCHKIT_HASH_TEST_SECRET", "super-secret-value")

	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_HASH_TEST_SECRET")
	if err != nil {
		t.Fatalf("SecretFromEnv(): %v", err)
	}

	for _, formatted := range []string{
		fmt.Sprint(secret),
		fmt.Sprintf("%#v", secret),
	} {
		if strings.Contains(formatted, "super-secret-value") {
			t.Fatalf("formatted secret leaked value: %q", formatted)
		}
	}
}

func TestSecretFromEnvRejectsMissing(t *testing.T) {
	_, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_HASH_TEST_MISSING")
	if !errors.Is(err, sketchhash.ErrEmptySecret) {
		t.Fatalf("SecretFromEnv() error = %v, want %v", err, sketchhash.ErrEmptySecret)
	}
}

func TestSecretFromEnvRejectsWeakValues(t *testing.T) {
	for _, value := range []string{"short", "demo-secret-change-me", "dev-secret-change-me"} {
		t.Setenv("LLM_SKETCHKIT_HASH_TEST_WEAK_SECRET", value)
		_, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_HASH_TEST_WEAK_SECRET")
		if !errors.Is(err, sketchhash.ErrWeakSecret) {
			t.Fatalf("SecretFromEnv(%q) error = %v, want %v", value, err, sketchhash.ErrWeakSecret)
		}
	}
}

func TestHash64RejectsUnregisteredDomain(t *testing.T) {
	t.Setenv("LLM_SKETCHKIT_HASH_TEST_SECRET", "domain-test-secret-32-bytes")
	secret, err := sketchhash.SecretFromEnv("LLM_SKETCHKIT_HASH_TEST_SECRET")
	if err != nil {
		t.Fatalf("SecretFromEnv(): %v", err)
	}

	_, err = sketchhash.Hash64(secret, sketchhash.Domain("unknown:v1"), []byte("value"))
	if !errors.Is(err, sketchhash.ErrUnregisteredDomain) {
		t.Fatalf("Hash64() error = %v, want %v", err, sketchhash.ErrUnregisteredDomain)
	}
}

func readHashVector(t *testing.T, path string) hashVector {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vector %s: %v", path, err)
	}

	var vector hashVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode vector %s: %v", path, err)
	}

	return vector
}
