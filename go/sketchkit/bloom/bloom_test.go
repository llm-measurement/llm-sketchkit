// Code authors: Vijay Erramilli and Codex
package bloom

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

type sketchVector struct {
	Name     string `json:"name"`
	Metadata struct {
		Kind               string `json:"kind"`
		Profile            string `json:"profile"`
		HashDomain         string `json:"hash_domain"`
		HashAlgo           string `json:"hash_algo"`
		RepresentationMode string `json:"representation_mode"`
		BloomBitCount      uint64 `json:"bloom_bit_count"`
		BloomHashCount     uint32 `json:"bloom_hash_count"`
	} `json:"metadata"`
	Operations []struct {
		Op      string `json:"op"`
		HashHex string `json:"hash_hex"`
		Source  string `json:"source"`
	} `json:"operations"`
	Expected struct {
		Body struct {
			InsertedCount         uint64   `json:"inserted_count"`
			SetBits               []uint64 `json:"set_bits"`
			BitsetHex             string   `json:"bitset_hex"`
			MayContain            []string `json:"may_contain"`
			MayNotContain         []string `json:"may_not_contain"`
			FalsePositiveEstimate float64  `json:"false_positive_estimate"`
		} `json:"body"`
	} `json:"expected"`
}

func TestSketchVectors(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "sketches", "bloom_*.json"))
	if err != nil {
		t.Fatalf("glob sketch vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no Bloom vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			vector := readSketchVector(t, path)
			got := buildVectorSketch(t, vector)
			assertVectorExpected(t, got, vector)
			assertStableReserialization(t, got)
		})
	}
}

func TestZeroFalseNegativesAndFalsePositiveRate(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		profile Profile
		queries int
	}{
		{profile: ProfileMicro, queries: 200_000},
		{profile: ProfileSmall, queries: 200_000},
		{profile: ProfileDefault, queries: 1_000_000},
	}
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(string(scenario.profile), func(t *testing.T) {
			t.Parallel()

			sketch := newTestSketch(t, scenario.profile)
			for i := uint64(0); i < sketch.RatedInsertions(); i++ {
				mustAdd(t, sketch, splitmix64(0xb1000000+i))
			}
			for i := uint64(0); i < sketch.RatedInsertions(); i++ {
				hash := splitmix64(0xb1000000 + i)
				if !sketch.MayContainHash(hash) {
					t.Fatalf("false negative for inserted hash %016x", hash)
				}
			}

			falsePositives := 0
			for i := 0; i < scenario.queries; i++ {
				hash := splitmix64(0xd1000000 + uint64(i))
				if sketch.MayContainHash(hash) {
					falsePositives++
				}
			}
			empirical := float64(falsePositives) / float64(scenario.queries)
			limit := 1.5 * sketch.TargetFPR()
			if empirical > limit {
				t.Fatalf("profile=%s empirical FPR=%f false_positives=%d queries=%d target=%f limit=%f estimate=%f",
					scenario.profile, empirical, falsePositives, scenario.queries,
					sketch.TargetFPR(), limit, sketch.FalsePositiveEstimate())
			}
			t.Logf("bloom fpr profile=%s rated_insertions=%d bit_count=%d hash_count=%d queries=%d false_positives=%d empirical_fpr=%.8f target_fpr=%.8f limit=%.8f estimate=%.8f",
				scenario.profile, sketch.RatedInsertions(), sketch.BitCount(), sketch.HashCount(),
				scenario.queries, falsePositives, empirical, sketch.TargetFPR(), limit, sketch.FalsePositiveEstimate())
		})
	}
}

func TestMergeEqualsUnion(t *testing.T) {
	t.Parallel()

	left := newTestSketch(t, ProfileMicro)
	right := newTestSketch(t, ProfileMicro)
	union := newTestSketch(t, ProfileMicro)
	for i := uint64(0); i < 1000; i++ {
		hash := splitmix64(0x9910 + i)
		if i%2 == 0 {
			mustAdd(t, left, hash)
		} else {
			mustAdd(t, right, hash)
		}
		mustAdd(t, union, hash)
	}
	merged := left.Clone()
	if err := merged.Merge(right); err != nil {
		t.Fatalf("Merge(): %v", err)
	}
	if hex.EncodeToString(merged.bitset) != hex.EncodeToString(union.bitset) {
		t.Fatal("merged bitset differs from union bitset")
	}
}

func TestParseRejectsNonProfileShape(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileMicro)
	encoded, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}

	var message sketchpb.Sketch
	if err := proto.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("unmarshal sketch: %v", err)
	}
	wrongBitCount := message.GetMetadata().GetBloomBitCount() + 1
	message.GetMetadata().BloomBitCount = &wrongBitCount

	mutated, err := proto.MarshalOptions{Deterministic: true}.Marshal(&message)
	if err != nil {
		t.Fatalf("marshal mutated sketch: %v", err)
	}
	_, err = Parse(mutated)
	if !errors.Is(err, ErrInvalidShape) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidShape)
	}
}

func readSketchVector(t *testing.T, path string) sketchVector {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vector %s: %v", path, err)
	}

	var vector sketchVector
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatalf("decode vector %s: %v", path, err)
	}

	return vector
}

func buildVectorSketch(t *testing.T, vector sketchVector) *Sketch {
	t.Helper()

	if vector.Metadata.Kind != "BLOOM" {
		t.Fatalf("unsupported vector kind %q", vector.Metadata.Kind)
	}
	if vector.Metadata.HashAlgo != string(sketchhash.HMACSHA25664) {
		t.Fatalf("unsupported hash algorithm %q", vector.Metadata.HashAlgo)
	}
	profile := Profile(vector.Metadata.Profile)
	config := profileConfigs[profile]
	if vector.Metadata.BloomBitCount != config.bitCount || vector.Metadata.BloomHashCount != config.hashCount {
		t.Fatalf("vector shape bit_count=%d hash_count=%d, want bit_count=%d hash_count=%d",
			vector.Metadata.BloomBitCount, vector.Metadata.BloomHashCount, config.bitCount, config.hashCount)
	}

	bySource := map[string]*Sketch{}
	sourceNames := make([]string, 0)
	for _, operation := range vector.Operations {
		if operation.Op != "add_hash" {
			t.Fatalf("unsupported operation %q", operation.Op)
		}
		source := operation.Source
		if source == "" {
			source = "default"
		}
		if bySource[source] == nil {
			bySource[source] = newTestSketch(t, profile)
			sourceNames = append(sourceNames, source)
		}
		mustAdd(t, bySource[source], parseHashHex(t, operation.HashHex))
	}
	if len(bySource) == 0 {
		return newTestSketch(t, profile)
	}

	sort.Strings(sourceNames)
	merged := bySource[sourceNames[0]].Clone()
	for _, source := range sourceNames[1:] {
		if err := merged.Merge(bySource[source]); err != nil {
			t.Fatalf("merge source %s: %v", source, err)
		}
	}

	return merged
}

func assertVectorExpected(t *testing.T, got *Sketch, vector sketchVector) {
	t.Helper()

	if got.InsertedCount() != vector.Expected.Body.InsertedCount {
		t.Fatalf("inserted count = %d, want %d", got.InsertedCount(), vector.Expected.Body.InsertedCount)
	}
	if vector.Expected.Body.SetBits != nil {
		assertSetBits(t, got, vector.Expected.Body.SetBits)
	}
	if vector.Expected.Body.BitsetHex != "" {
		assertBitsetHex(t, got, vector.Expected.Body.BitsetHex)
	}
	if estimate := got.FalsePositiveEstimate(); math.Abs(estimate-vector.Expected.Body.FalsePositiveEstimate) > 1e-15 {
		t.Fatalf("false positive estimate = %.18f, want %.18f", estimate, vector.Expected.Body.FalsePositiveEstimate)
	}
	for _, hashHex := range vector.Expected.Body.MayContain {
		hash := parseHashHex(t, hashHex)
		if !got.MayContainHash(hash) {
			t.Fatalf("MayContainHash(%s) = false, want true", hashHex)
		}
	}
	for _, hashHex := range vector.Expected.Body.MayNotContain {
		hash := parseHashHex(t, hashHex)
		if got.MayContainHash(hash) {
			t.Fatalf("MayContainHash(%s) = true, want false", hashHex)
		}
	}
}

func assertBitsetHex(t *testing.T, got *Sketch, want string) {
	t.Helper()

	if bitsetHex := hex.EncodeToString(got.bitset); bitsetHex != want {
		t.Fatalf("bitset hex = %s, want %s", bitsetHex, want)
	}
}

func assertSetBits(t *testing.T, got *Sketch, want []uint64) {
	t.Helper()

	gotBits := make([]uint64, 0)
	for byteIndex, b := range got.bitset {
		for bit := 0; bit < 8; bit++ {
			if b&(1<<bit) != 0 {
				gotBits = append(gotBits, uint64(byteIndex*8+bit))
			}
		}
	}
	if len(gotBits) != len(want) {
		t.Fatalf("set bit count = %d, want %d: got %v want %v", len(gotBits), len(want), gotBits, want)
	}
	for i := range gotBits {
		if gotBits[i] != want[i] {
			t.Fatalf("set_bits[%d] = %d, want %d", i, gotBits[i], want[i])
		}
	}
}

func assertStableReserialization(t *testing.T, sketch *Sketch) {
	t.Helper()

	first, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	parsed, err := Parse(first)
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	second, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() after parse: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("reserialization changed bytes:\nfirst=%s\nsecond=%s", hex.EncodeToString(first), hex.EncodeToString(second))
	}
}

func newTestSketch(t *testing.T, profile Profile) *Sketch {
	t.Helper()

	sketch, err := New(profile, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatalf("New(%s): %v", profile, err)
	}

	return sketch
}

func mustAdd(t *testing.T, sketch *Sketch, hash uint64) {
	t.Helper()

	if err := sketch.AddHash(hash); err != nil {
		t.Fatalf("AddHash(%016x): %v", hash, err)
	}
}

func parseHashHex(t *testing.T, value string) uint64 {
	t.Helper()

	bytes, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode hash %q: %v", value, err)
	}
	if len(bytes) != 8 {
		t.Fatalf("hash %q decoded to %d bytes, want 8", value, len(bytes))
	}

	var out uint64
	for _, b := range bytes {
		out = (out << 8) | uint64(b)
	}

	return out
}

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
