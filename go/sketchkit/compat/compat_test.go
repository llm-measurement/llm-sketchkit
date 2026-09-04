// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package compat_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
)

type manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Release       string    `json:"release"`
	Fixtures      []fixture `json:"fixtures"`
}

type fixture struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Source     string   `json:"source"`
	WireSHA256 string   `json:"wire_sha256"`
	Expected   expected `json:"expected"`
}

type expected struct {
	SparseCount       *int    `json:"sparse_count"`
	DenseNonzeroCount *int    `json:"dense_nonzero_count"`
	TotalWeight       *int64  `json:"total_weight"`
	MaxError          *int64  `json:"max_error"`
	ItemCount         *int    `json:"item_count"`
	InsertedCount     *uint64 `json:"inserted_count"`
	SetBitCount       *uint64 `json:"set_bit_count"`
	PopulatedCount    *uint64 `json:"populated_count"`
	SignatureLength   *int    `json:"signature_length"`
}

type vector struct {
	Expected struct {
		SerializedHex string `json:"serialized_hex"`
	} `json:"expected"`
}

type wireValue interface {
	MarshalBinary() ([]byte, error)
}

func TestV010SerializedStateCompatibility(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	manifestPath := filepath.Join(root, "vectors", "compat", "v0.1.0.json")
	manifestValue := readJSON[manifest](t, manifestPath)
	if manifestValue.SchemaVersion != 1 || manifestValue.Release != "v0.1.0" {
		t.Fatalf("unexpected compatibility manifest identity: %#v", manifestValue)
	}
	if len(manifestValue.Fixtures) != 5 {
		t.Fatalf("fixture count = %d, want 5", len(manifestValue.Fixtures))
	}

	seen := make(map[string]struct{}, len(manifestValue.Fixtures))
	for _, current := range manifestValue.Fixtures {
		t.Run(current.Name, func(t *testing.T) {
			if _, ok := seen[current.Name]; ok {
				t.Fatalf("duplicate fixture name %q", current.Name)
			}
			seen[current.Name] = struct{}{}

			vectorValue := readJSON[vector](t, filepath.Join(root, current.Source))
			encoded, err := hex.DecodeString(vectorValue.Expected.SerializedHex)
			if err != nil {
				t.Fatalf("decode serialized state: %v", err)
			}
			digest := sha256.Sum256(encoded)
			if got := hex.EncodeToString(digest[:]); got != current.WireSHA256 {
				t.Fatalf("wire digest = %s, want %s", got, current.WireSHA256)
			}

			switch current.Kind {
			case "HLLPP":
				assertHLLPP(t, encoded, current.Expected)
			case "FREQUENT_ITEMS":
				assertFrequentItems(t, encoded, current.Expected)
			case "BLOOM":
				assertBloom(t, encoded, current.Expected)
			case "MINHASH":
				assertMinHash(t, encoded, current.Expected)
			default:
				t.Fatalf("unsupported fixture kind %q", current.Kind)
			}
		})
	}
}

func assertHLLPP(t *testing.T, encoded []byte, want expected) {
	t.Helper()
	parsed, err := hllpp.Parse(encoded)
	if err != nil {
		t.Fatalf("parse HLL++: %v", err)
	}
	assertStable(t, encoded, parsed)
	if want.SparseCount == nil || parsed.SparseCount() != *want.SparseCount {
		t.Fatalf("sparse count = %d, want %v", parsed.SparseCount(), want.SparseCount)
	}
	if want.DenseNonzeroCount == nil || parsed.DenseNonZeroCount() != *want.DenseNonzeroCount {
		t.Fatalf("dense nonzero count = %d, want %v", parsed.DenseNonZeroCount(), want.DenseNonzeroCount)
	}
}

func assertFrequentItems(t *testing.T, encoded []byte, want expected) {
	t.Helper()
	parsed, err := frequentitems.Parse(encoded)
	if err != nil {
		t.Fatalf("parse frequent-items: %v", err)
	}
	assertStable(t, encoded, parsed)
	if want.TotalWeight == nil || parsed.TotalWeight() != *want.TotalWeight {
		t.Fatalf("total weight = %d, want %v", parsed.TotalWeight(), want.TotalWeight)
	}
	if want.MaxError == nil || parsed.MaxError() != *want.MaxError {
		t.Fatalf("max error = %d, want %v", parsed.MaxError(), want.MaxError)
	}
	if want.ItemCount == nil || parsed.Len() != *want.ItemCount {
		t.Fatalf("item count = %d, want %v", parsed.Len(), want.ItemCount)
	}
}

func assertBloom(t *testing.T, encoded []byte, want expected) {
	t.Helper()
	parsed, err := bloom.Parse(encoded)
	if err != nil {
		t.Fatalf("parse Bloom: %v", err)
	}
	assertStable(t, encoded, parsed)
	if want.InsertedCount == nil || parsed.InsertedCount() != *want.InsertedCount {
		t.Fatalf("inserted count = %d, want %v", parsed.InsertedCount(), want.InsertedCount)
	}
	if want.SetBitCount == nil || parsed.SetBitCount() != *want.SetBitCount {
		t.Fatalf("set bit count = %d, want %v", parsed.SetBitCount(), want.SetBitCount)
	}
}

func assertMinHash(t *testing.T, encoded []byte, want expected) {
	t.Helper()
	parsed, err := minhash.Parse(encoded)
	if err != nil {
		t.Fatalf("parse MinHash: %v", err)
	}
	assertStable(t, encoded, parsed)
	if want.PopulatedCount == nil || parsed.PopulatedCount() != *want.PopulatedCount {
		t.Fatalf("populated count = %d, want %v", parsed.PopulatedCount(), want.PopulatedCount)
	}
	if want.SignatureLength == nil || parsed.SignatureLength() != *want.SignatureLength {
		t.Fatalf("signature length = %d, want %v", parsed.SignatureLength(), want.SignatureLength)
	}
}

func assertStable(t *testing.T, encoded []byte, parsed wireValue) {
	t.Helper()
	reencoded, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal parsed state: %v", err)
	}
	if !bytes.Equal(encoded, reencoded) {
		t.Fatal("serialized state changed after parse")
	}
}

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}
