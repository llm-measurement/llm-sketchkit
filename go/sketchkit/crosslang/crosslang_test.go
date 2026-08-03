// Code authors: Vijay Erramilli and Codex
package crosslang_test

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/bloom"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/frequentitems"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/hllpp"
	"github.com/llm-measurement/llm-sketchkit/go/sketchkit/minhash"
)

type crossLanguageVector struct {
	Name     string `json:"name"`
	Metadata struct {
		Kind string `json:"kind"`
	} `json:"metadata"`
	Expected struct {
		SerializedHex string `json:"serialized_hex"`
		Body          struct {
			SourceSerializedHex   map[string]string `json:"source_serialized_hex"`
			MergedSerializedHex   string            `json:"merged_serialized_hex"`
			RepresentationMode    string            `json:"representation_mode"`
			DenseNonZeroCount     int               `json:"dense_nonzero_count"`
			InsertedCount         uint64            `json:"inserted_count"`
			SetBitCount           uint64            `json:"set_bit_count"`
			FalsePositiveEstimate float64           `json:"false_positive_estimate"`
			PopulatedCount        uint64            `json:"populated_count"`
			TotalWeight           int64             `json:"total_weight"`
			MaxError              int64             `json:"max_error"`
			NoFalseNegatives      []string          `json:"no_false_negatives"`
			NoFalsePositives      []string          `json:"no_false_positives"`
			Entries               []struct {
				HashHex    string `json:"hash_hex"`
				Estimate   int64  `json:"estimate"`
				Error      int64  `json:"error"`
				LowerBound int64  `json:"lower_bound"`
				UpperBound int64  `json:"upper_bound"`
			} `json:"entries"`
		} `json:"body"`
	} `json:"expected"`
}

func TestCrossLanguageMergeVectors(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "sketches", "cross_language_*.json"))
	if err != nil {
		t.Fatalf("glob cross-language vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no cross-language vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			vector := readCrossLanguageVector(t, path)
			switch vector.Metadata.Kind {
			case "HLLPP":
				assertHLLPPMerge(t, vector)
			case "BLOOM":
				assertBloomMerge(t, vector)
			case "MINHASH":
				assertMinHashMerge(t, vector)
			case "FREQUENT_ITEMS":
				assertFrequentItemsMerge(t, vector)
			default:
				t.Fatalf("unsupported kind %q", vector.Metadata.Kind)
			}
		})
	}
}

func assertHLLPPMerge(t *testing.T, vector crossLanguageVector) {
	t.Helper()

	left, err := hllpp.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["left"]))
	if err != nil {
		t.Fatalf("parse left: %v", err)
	}
	right, err := hllpp.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["right"]))
	if err != nil {
		t.Fatalf("parse right: %v", err)
	}
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := mustMarshalHex(t, merged); got != vector.Expected.SerializedHex {
		t.Fatalf("merged hex = %s, want %s", got, vector.Expected.SerializedHex)
	}
	if merged.DenseNonZeroCount() != vector.Expected.Body.DenseNonZeroCount {
		t.Fatalf("dense non-zero count = %d, want %d", merged.DenseNonZeroCount(), vector.Expected.Body.DenseNonZeroCount)
	}
	assertHLLPPStableParse(t, vector.Expected.Body.MergedSerializedHex)
}

func assertBloomMerge(t *testing.T, vector crossLanguageVector) {
	t.Helper()

	left, err := bloom.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["left"]))
	if err != nil {
		t.Fatalf("parse left: %v", err)
	}
	right, err := bloom.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["right"]))
	if err != nil {
		t.Fatalf("parse right: %v", err)
	}
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := mustMarshalHex(t, merged); got != vector.Expected.SerializedHex {
		t.Fatalf("merged hex = %s, want %s", got, vector.Expected.SerializedHex)
	}
	if merged.InsertedCount() != vector.Expected.Body.InsertedCount {
		t.Fatalf("inserted count = %d, want %d", merged.InsertedCount(), vector.Expected.Body.InsertedCount)
	}
	if merged.SetBitCount() != vector.Expected.Body.SetBitCount {
		t.Fatalf("set bit count = %d, want %d", merged.SetBitCount(), vector.Expected.Body.SetBitCount)
	}
	if math.Abs(merged.FalsePositiveEstimate()-vector.Expected.Body.FalsePositiveEstimate) > 1e-15 {
		t.Fatalf("FPR estimate = %.18f, want %.18f", merged.FalsePositiveEstimate(), vector.Expected.Body.FalsePositiveEstimate)
	}
	assertBloomStableParse(t, vector.Expected.Body.MergedSerializedHex)
}

func assertMinHashMerge(t *testing.T, vector crossLanguageVector) {
	t.Helper()

	left, err := minhash.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["left"]))
	if err != nil {
		t.Fatalf("parse left: %v", err)
	}
	right, err := minhash.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["right"]))
	if err != nil {
		t.Fatalf("parse right: %v", err)
	}
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := mustMarshalHex(t, merged); got != vector.Expected.SerializedHex {
		t.Fatalf("merged hex = %s, want %s", got, vector.Expected.SerializedHex)
	}
	if merged.PopulatedCount() != vector.Expected.Body.PopulatedCount {
		t.Fatalf("populated count = %d, want %d", merged.PopulatedCount(), vector.Expected.Body.PopulatedCount)
	}
	assertMinHashStableParse(t, vector.Expected.Body.MergedSerializedHex)
}

func assertFrequentItemsMerge(t *testing.T, vector crossLanguageVector) {
	t.Helper()

	left, err := frequentitems.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["left"]))
	if err != nil {
		t.Fatalf("parse left: %v", err)
	}
	right, err := frequentitems.Parse(decodeHex(t, vector.Expected.Body.SourceSerializedHex["right"]))
	if err != nil {
		t.Fatalf("parse right: %v", err)
	}
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("merge: %v", err)
	}
	expected, err := frequentitems.Parse(decodeHex(t, vector.Expected.Body.MergedSerializedHex))
	if err != nil {
		t.Fatalf("parse expected: %v", err)
	}
	assertFrequentItemsSemantics(t, merged, expected, vector)
}

func assertFrequentItemsSemantics(t *testing.T, got *frequentitems.Sketch, expected *frequentitems.Sketch, vector crossLanguageVector) {
	t.Helper()

	if got.TotalWeight() != expected.TotalWeight() || got.TotalWeight() != vector.Expected.Body.TotalWeight {
		t.Fatalf("total weight = %d, expected fixture = %d, vector = %d",
			got.TotalWeight(), expected.TotalWeight(), vector.Expected.Body.TotalWeight)
	}
	if got.MaxError() != expected.MaxError() || got.MaxError() != vector.Expected.Body.MaxError {
		t.Fatalf("max error = %d, expected fixture = %d, vector = %d",
			got.MaxError(), expected.MaxError(), vector.Expected.Body.MaxError)
	}
	for _, entry := range vector.Expected.Body.Entries {
		hashValue := parseHashHex(t, entry.HashHex)
		if got.EstimateHash(hashValue) != expected.EstimateHash(hashValue) ||
			got.EstimateHash(hashValue) != entry.Estimate {
			t.Fatalf("estimate for %s = %d, expected fixture = %d, vector = %d",
				entry.HashHex, got.EstimateHash(hashValue), expected.EstimateHash(hashValue), entry.Estimate)
		}
		if got.LowerBoundHash(hashValue) != expected.LowerBoundHash(hashValue) ||
			got.LowerBoundHash(hashValue) != entry.LowerBound {
			t.Fatalf("lower bound for %s = %d, expected fixture = %d, vector = %d",
				entry.HashHex, got.LowerBoundHash(hashValue), expected.LowerBoundHash(hashValue), entry.LowerBound)
		}
		if got.UpperBoundHash(hashValue) != expected.UpperBoundHash(hashValue) ||
			got.UpperBoundHash(hashValue) != entry.UpperBound {
			t.Fatalf("upper bound for %s = %d, expected fixture = %d, vector = %d",
				entry.HashHex, got.UpperBoundHash(hashValue), expected.UpperBoundHash(hashValue), entry.UpperBound)
		}
	}
	assertFrequentItemHexes(t, got, frequentitems.NoFalseNegatives, vector.Expected.Body.NoFalseNegatives)
	assertFrequentItemHexes(t, got, frequentitems.NoFalsePositives, vector.Expected.Body.NoFalsePositives)
	assertFrequentItemsStableParse(t, vector.Expected.Body.MergedSerializedHex)
}

func assertFrequentItemHexes(t *testing.T, sketch *frequentitems.Sketch, mode frequentitems.QueryMode, want []string) {
	t.Helper()

	items, err := sketch.FrequentItems(mode)
	if err != nil {
		t.Fatalf("FrequentItems(%d): %v", mode, err)
	}
	if len(items) != len(want) {
		t.Fatalf("items length = %d, want %d", len(items), len(want))
	}
	for i, item := range items {
		got := hex.EncodeToString(uint64Bytes(item.Hash))
		if got != want[i] {
			t.Fatalf("items[%d] = %s, want %s", i, got, want[i])
		}
	}
}

func assertHLLPPStableParse(t *testing.T, value string) {
	t.Helper()
	parsed, err := hllpp.Parse(decodeHex(t, value))
	if err != nil {
		t.Fatalf("parse merged HLL++: %v", err)
	}
	if got := mustMarshalHex(t, parsed); got != value {
		t.Fatalf("HLL++ parse/rewrite = %s, want %s", got, value)
	}
}

func assertBloomStableParse(t *testing.T, value string) {
	t.Helper()
	parsed, err := bloom.Parse(decodeHex(t, value))
	if err != nil {
		t.Fatalf("parse merged Bloom: %v", err)
	}
	if got := mustMarshalHex(t, parsed); got != value {
		t.Fatalf("Bloom parse/rewrite = %s, want %s", got, value)
	}
}

func assertMinHashStableParse(t *testing.T, value string) {
	t.Helper()
	parsed, err := minhash.Parse(decodeHex(t, value))
	if err != nil {
		t.Fatalf("parse merged MinHash: %v", err)
	}
	if got := mustMarshalHex(t, parsed); got != value {
		t.Fatalf("MinHash parse/rewrite = %s, want %s", got, value)
	}
}

func assertFrequentItemsStableParse(t *testing.T, value string) {
	t.Helper()
	parsed, err := frequentitems.Parse(decodeHex(t, value))
	if err != nil {
		t.Fatalf("parse merged frequent-items: %v", err)
	}
	if got := mustMarshalHex(t, parsed); got != value {
		t.Fatalf("frequent-items parse/rewrite = %s, want %s", got, value)
	}
}

func mustMarshalHex(t *testing.T, value interface{ MarshalBinary() ([]byte, error) }) string {
	t.Helper()
	data, err := value.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	return hex.EncodeToString(data)
}

func decodeHex(t *testing.T, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	return data
}

func parseHashHex(t *testing.T, value string) uint64 {
	t.Helper()
	data := decodeHex(t, value)
	if len(data) != 8 {
		t.Fatalf("hash hex length = %d, want 8 bytes", len(data))
	}
	var out uint64
	for _, b := range data {
		out = (out << 8) | uint64(b)
	}
	return out
}

func uint64Bytes(value uint64) []byte {
	return []byte{
		byte(value >> 56),
		byte(value >> 48),
		byte(value >> 40),
		byte(value >> 32),
		byte(value >> 24),
		byte(value >> 16),
		byte(value >> 8),
		byte(value),
	}
}

func readCrossLanguageVector(t *testing.T, path string) crossLanguageVector {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vector %s: %v", path, err)
	}
	var vector crossLanguageVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode vector %s: %v", path, err)
	}
	return vector
}
