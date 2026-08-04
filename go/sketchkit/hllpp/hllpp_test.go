// SPDX-License-Identifier: Apache-2.0
// Code authors: Vijay Erramilli and Codex
package hllpp

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	sketchhash "github.com/llm-measurement/llm-sketchkit/go/sketchkit/hash"
	sketchpb "github.com/llm-measurement/llm-sketchkit/go/sketchkit/internal/pb"
	"google.golang.org/protobuf/proto"
)

type sketchVector struct {
	Name     string `json:"name"`
	Metadata struct {
		Kind                 string `json:"kind"`
		Profile              string `json:"profile"`
		HashDomain           string `json:"hash_domain"`
		HashAlgo             string `json:"hash_algo"`
		RepresentationMode   string `json:"representation_mode"`
		HLLPPNormalPrecision uint8  `json:"hllpp_normal_precision"`
		HLLPPSparsePrecision uint8  `json:"hllpp_sparse_precision"`
	} `json:"metadata"`
	Operations []struct {
		Op      string `json:"op"`
		HashHex string `json:"hash_hex"`
		Source  string `json:"source"`
	} `json:"operations"`
	Expected struct {
		SerializedHex string `json:"serialized_hex"`
		Body          struct {
			RepresentationMode string `json:"representation_mode"`
			SparseCount        int    `json:"sparse_count"`
			DenseNonZeroCount  int    `json:"dense_nonzero_count"`
			SparseRegisters    []struct {
				Index uint32 `json:"index"`
				Value uint8  `json:"value"`
			} `json:"sparse_registers"`
		} `json:"body"`
	} `json:"expected"`
}

func TestSketchVectors(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "sketches", "hllpp_*.json"))
	if err != nil {
		t.Fatalf("glob sketch vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no HLL++ vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			vector := readSketchVector(t, path)
			got := buildVectorSketch(t, vector)

			assertExpectedRepresentation(t, got, vector)
			assertStableReserialization(t, got)
			assertSerializedHex(t, got, vector.Expected.SerializedHex)
		})
	}
}

func TestMergeRejectsCrossPrecision(t *testing.T) {
	t.Parallel()

	left := newTestSketch(t, ProfileMicro)
	right := newTestSketch(t, ProfileSmall)

	err := left.Merge(right)
	if !errors.Is(err, ErrPrecisionMismatch) {
		t.Fatalf("Merge() error = %v, want %v", err, ErrPrecisionMismatch)
	}
}

func TestParseRejectsNonProfilePrecision(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileSmall)
	encoded, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}

	var message sketchpb.Sketch
	if err := proto.Unmarshal(encoded, &message); err != nil {
		t.Fatalf("unmarshal sketch: %v", err)
	}
	nonProfilePrecision := uint32(13)
	message.GetMetadata().HllppNormalPrecision = &nonProfilePrecision

	mutated, err := proto.MarshalOptions{Deterministic: true}.Marshal(&message)
	if err != nil {
		t.Fatalf("marshal mutated sketch: %v", err)
	}
	_, err = Parse(mutated)
	if !errors.Is(err, ErrPrecisionMismatch) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrPrecisionMismatch)
	}
}

func TestParseRejectsPrecisionWrap(t *testing.T) {
	t.Parallel()

	message := malformedHLLPPMessage(ProfileSmall)
	normalPrecision := uint32(270)
	sparsePrecision := uint32(274)
	message.GetMetadata().HllppNormalPrecision = &normalPrecision
	message.GetMetadata().HllppSparsePrecision = &sparsePrecision
	message.GetHllpp().SparseRegisters = []*sketchpb.HllppSparseRegister{{Index: 1, Value: 1}}

	_, err := Parse(marshalSketchMessage(t, message))
	if !errors.Is(err, ErrPrecisionMismatch) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrPrecisionMismatch)
	}
}

func TestParseRejectsSparseIndexOutOfRange(t *testing.T) {
	t.Parallel()

	message := malformedHLLPPMessage(ProfileSmall)
	message.GetHllpp().SparseRegisters = []*sketchpb.HllppSparseRegister{
		{Index: 1 << profileConfigs[ProfileSmall].sp, Value: 1},
	}

	_, err := Parse(marshalSketchMessage(t, message))
	if !errors.Is(err, ErrInvalidWireEncoding) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidWireEncoding)
	}
}

func TestParseRejectsSparseCountAbovePromotionThreshold(t *testing.T) {
	t.Parallel()

	message := malformedHLLPPMessage(ProfileMicro)
	config := profileConfigs[ProfileMicro]
	registers := make([]*sketchpb.HllppSparseRegister, 0, config.promotionThreshold+1)
	for i := 0; i <= config.promotionThreshold; i++ {
		registers = append(registers, &sketchpb.HllppSparseRegister{Index: uint32(i), Value: 1})
	}
	message.GetHllpp().SparseRegisters = registers

	_, err := Parse(marshalSketchMessage(t, message))
	if !errors.Is(err, ErrInvalidWireEncoding) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidWireEncoding)
	}
}

func TestParseRejectsInvalidDenseRank(t *testing.T) {
	t.Parallel()

	message := malformedHLLPPMessage(ProfileSmall)
	config := profileConfigs[ProfileSmall]
	message.GetMetadata().RepresentationMode = sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE
	message.GetHllpp().DenseRegisters = make([]byte, 1<<config.p)
	message.GetHllpp().DenseRegisters[0] = byte(64 - config.p + 2)

	_, err := Parse(marshalSketchMessage(t, message))
	if !errors.Is(err, ErrInvalidWireEncoding) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidWireEncoding)
	}
}

func TestParseRejectsOversizedWireInput(t *testing.T) {
	t.Parallel()

	_, err := Parse(make([]byte, maxWireBytes+1))
	if !errors.Is(err, ErrInvalidWireEncoding) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidWireEncoding)
	}
}

func TestMergeProperties(t *testing.T) {
	t.Parallel()

	a := newTestSketch(t, ProfileSmall)
	b := newTestSketch(t, ProfileSmall)
	c := newTestSketch(t, ProfileSmall)
	union := newTestSketch(t, ProfileSmall)

	for i := uint64(0); i < 9000; i++ {
		value := splitmix64(0xfeedface + i)
		union.AddHash(value)
		switch i % 3 {
		case 0:
			a.AddHash(value)
		case 1:
			b.AddHash(value)
		default:
			c.AddHash(value)
		}
	}

	abThenC := a.Clone()
	if err := abThenC.Merge(b); err != nil {
		t.Fatalf("merge ab: %v", err)
	}
	if err := abThenC.Merge(c); err != nil {
		t.Fatalf("merge abc: %v", err)
	}

	bcThenA := b.Clone()
	if err := bcThenA.Merge(c); err != nil {
		t.Fatalf("merge bc: %v", err)
	}
	if err := bcThenA.Merge(a); err != nil {
		t.Fatalf("merge bca: %v", err)
	}

	if abThenC.Estimate() != bcThenA.Estimate() {
		t.Fatalf("merge estimate differs by order: %f vs %f", abThenC.Estimate(), bcThenA.Estimate())
	}

	bound := 3 * 1.04 / math.Sqrt(float64(uint64(1)<<abThenC.p))
	relativeError := math.Abs(abThenC.Estimate()-union.Estimate()) / union.Estimate()
	if relativeError > bound {
		t.Fatalf("merge estimate relative error = %f, want <= %f", relativeError, bound)
	}
}

func TestMergeOrderIndependence(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name    string
		profile Profile
		updates int
	}{
		{name: "micro_sparse", profile: ProfileMicro, updates: 128},
		{name: "micro_dense", profile: ProfileMicro, updates: 4096},
		{name: "small_sparse", profile: ProfileSmall, updates: 512},
		{name: "small_dense", profile: ProfileSmall, updates: 12000},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			values := deterministicHashes(scenario.updates, uint64(len(scenario.name))<<32)
			union := newTestSketch(t, scenario.profile)
			for _, value := range values {
				union.AddHash(value)
			}
			unionEstimate := union.Estimate()

			for partitionCount := 2; partitionCount <= 5; partitionCount++ {
				partitionCount := partitionCount
				t.Run(fmt.Sprintf("partitions_%d", partitionCount), func(t *testing.T) {
					parts := make([]*Sketch, partitionCount)
					for i := range parts {
						parts[i] = newTestSketch(t, scenario.profile)
					}
					for i, value := range values {
						parts[i%partitionCount].AddHash(value)
					}

					permutations := permutations(partitionCount)
					var baselineEstimate float64
					var baselineBytes []byte
					for orderIndex, order := range permutations {
						merged := mergeInOrder(t, parts, order)
						estimate := merged.Estimate()
						if estimate != unionEstimate {
							t.Fatalf("estimate for order %v = %f, union estimate = %f", order, estimate, unionEstimate)
						}
						encoded, err := merged.MarshalBinary()
						if err != nil {
							t.Fatalf("marshal order %v: %v", order, err)
						}
						if orderIndex == 0 {
							baselineEstimate = estimate
							baselineBytes = encoded
							continue
						}
						if estimate != baselineEstimate {
							t.Fatalf("estimate for order %v = %f, baseline = %f", order, estimate, baselineEstimate)
						}
						if string(encoded) != string(baselineBytes) {
							t.Fatalf("serialization for order %v differs from baseline", order)
						}
					}
				})
			}
		})
	}
}

func TestSparseRegisterStorageMemory(t *testing.T) {
	t.Parallel()

	sparse := newTestSketch(t, ProfileSmall)
	for i := uint64(0); i < 500; i++ {
		sparse.AddHash(splitmix64(0x500 + i))
	}
	if sparse.RepresentationMode() != sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE {
		t.Fatalf("500 updates should remain sparse, got %s", sparse.RepresentationMode())
	}

	dense := sparse.Clone()
	dense.ForceDense()

	sparseBytes := sparse.RegisterStorageBytes()
	denseBytes := dense.RegisterStorageBytes()
	t.Logf("dense register storage=%d sparse register storage=%d ratio=%.2fx",
		denseBytes, sparseBytes, float64(denseBytes)/float64(sparseBytes))

	if denseBytes < 5*sparseBytes {
		t.Fatalf("dense register storage = %d, sparse = %d, want dense at least 5x sparse", denseBytes, sparseBytes)
	}
}

func TestAccuracySmallMediumAndBiasCardinalities(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		profile         Profile
		cardinalities   []int
		expectedRegimes map[int]string
	}{
		{
			profile:       ProfileMicro,
			cardinalities: []int{10, 100, 1000, 15000, 100000},
			expectedRegimes: map[int]string{
				15000: "dense HLL++ bias-corrected p=12",
			},
		},
		{
			profile:       ProfileSmall,
			cardinalities: []int{10, 100, 1000, 60000, 100000},
			expectedRegimes: map[int]string{
				60000: "dense HLL++ bias-corrected p=14",
			},
		},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(string(scenario.profile), func(t *testing.T) {
			t.Parallel()

			for _, result := range collectAccuracyResults(t, scenario.profile, scenario.cardinalities, 10) {
				assertAccuracyResultWithinBound(t, result)
				if expectedRegime, ok := scenario.expectedRegimes[result.Cardinality]; ok {
					assertAccuracyResultRegime(t, result, expectedRegime)
				}
				t.Log(result.LogLine())
			}
		})
	}
}

type accuracyResult struct {
	Profile           Profile
	Precision         uint8
	Cardinality       int
	Seeds             int
	Regime            string
	MeanRelativeError float64
	MaxRelativeError  float64
	Bound             float64
}

func collectAccuracyResults(t *testing.T, profile Profile, cardinalities []int, seedCount int) []accuracyResult {
	t.Helper()

	if len(cardinalities) == 0 {
		return nil
	}
	sortedCardinalities := append([]int(nil), cardinalities...)
	sort.Ints(sortedCardinalities)

	type cell struct {
		sumRelativeError float64
		maxRelativeError float64
		regimes          map[string]struct{}
	}
	cells := make([]cell, len(sortedCardinalities))
	for i := range cells {
		cells[i].regimes = make(map[string]struct{})
	}

	for seed := uint64(0); seed < uint64(seedCount); seed++ {
		sketch := newTestSketch(t, profile)
		nextMilestone := 0
		for i := 0; i < sortedCardinalities[len(sortedCardinalities)-1]; i++ {
			sketch.AddHash(splitmix64(seed<<32 + uint64(i)))
			if i+1 != sortedCardinalities[nextMilestone] {
				continue
			}

			cardinality := sortedCardinalities[nextMilestone]
			estimate := sketch.Estimate()
			relativeError := math.Abs(estimate-float64(cardinality)) / float64(cardinality)
			cells[nextMilestone].sumRelativeError += relativeError
			if relativeError > cells[nextMilestone].maxRelativeError {
				cells[nextMilestone].maxRelativeError = relativeError
			}
			cells[nextMilestone].regimes[estimatorRegime(sketch)] = struct{}{}

			nextMilestone++
			if nextMilestone == len(sortedCardinalities) {
				break
			}
		}
	}

	results := make([]accuracyResult, 0, len(sortedCardinalities))
	for i, cardinality := range sortedCardinalities {
		results = append(results, accuracyResult{
			Profile:           profile,
			Precision:         profileConfigs[profile].p,
			Cardinality:       cardinality,
			Seeds:             seedCount,
			Regime:            joinedRegimes(cells[i].regimes),
			MeanRelativeError: cells[i].sumRelativeError / float64(seedCount),
			MaxRelativeError:  cells[i].maxRelativeError,
			Bound:             accuracyBound(profile),
		})
	}

	return results
}

func assertAccuracyResultWithinBound(t *testing.T, result accuracyResult) {
	t.Helper()

	if result.MaxRelativeError > result.Bound {
		t.Fatalf("profile=%s p=%d cardinality=%d max relative error=%f, want <= %f",
			result.Profile, result.Precision, result.Cardinality, result.MaxRelativeError, result.Bound)
	}
}

func assertAccuracyResultRegime(t *testing.T, result accuracyResult, expectedRegime string) {
	t.Helper()

	if result.Regime != expectedRegime {
		t.Fatalf("profile=%s p=%d cardinality=%d regime=%q, want %q",
			result.Profile, result.Precision, result.Cardinality, result.Regime, expectedRegime)
	}
}

func (r accuracyResult) LogLine() string {
	return fmt.Sprintf(
		"accuracy cell profile=%s p=%d cardinality=%d regime=%s seeds=%d mean_rel_error=%.8f max_rel_error=%.8f enforced_bound=%.8f",
		r.Profile,
		r.Precision,
		r.Cardinality,
		r.Regime,
		r.Seeds,
		r.MeanRelativeError,
		r.MaxRelativeError,
		r.Bound,
	)
}

func estimatorRegime(sketch *Sketch) string {
	if sketch.sparse != nil {
		return fmt.Sprintf("sparse linear-counting sp=%d", sketch.sp)
	}

	m := 1 << sketch.p
	sum := 0.0
	zeroCount := 0
	for _, rank := range sketch.dense {
		sum += math.Ldexp(1, -int(rank))
		if rank == 0 {
			zeroCount++
		}
	}
	raw := alpha(uint64(m)) * float64(m*m) / sum
	if zeroCount != 0 {
		lc := linearCounting(uint64(m), uint64(m-zeroCount))
		if lc <= linearCountingThreshold(sketch.p) {
			return fmt.Sprintf("dense linear-counting p=%d", sketch.p)
		}
	}
	if raw <= 5*float64(m) {
		return fmt.Sprintf("dense HLL++ bias-corrected p=%d", sketch.p)
	}

	return fmt.Sprintf("dense HLL++ raw p=%d", sketch.p)
}

func joinedRegimes(regimes map[string]struct{}) string {
	names := make([]string, 0, len(regimes))
	for name := range regimes {
		names = append(names, name)
	}
	sort.Strings(names)

	return strings.Join(names, "; ")
}

func accuracyBound(profile Profile) float64 {
	precision := profileConfigs[profile].p
	return 3 * 1.04 / math.Sqrt(float64(uint64(1)<<precision))
}

func deterministicHashes(count int, seed uint64) []uint64 {
	values := make([]uint64, count)
	for i := range values {
		values[i] = splitmix64(seed + uint64(i))
	}

	return values
}

func mergeInOrder(t *testing.T, parts []*Sketch, order []int) *Sketch {
	t.Helper()

	merged := parts[order[0]].Clone()
	for _, index := range order[1:] {
		if err := merged.Merge(parts[index]); err != nil {
			t.Fatalf("merge part %d in order %v: %v", index, order, err)
		}
	}

	return merged
}

func permutations(count int) [][]int {
	values := make([]int, count)
	for i := range values {
		values[i] = i
	}

	var out [][]int
	var generate func(int)
	generate = func(position int) {
		if position == len(values) {
			out = append(out, append([]int(nil), values...))
			return
		}
		for i := position; i < len(values); i++ {
			values[position], values[i] = values[i], values[position]
			generate(position + 1)
			values[position], values[i] = values[i], values[position]
		}
	}
	generate(0)

	return out
}

func assertExpectedRepresentation(t *testing.T, got *Sketch, vector sketchVector) {
	t.Helper()

	wantMode := representationMode(vector.Expected.Body.RepresentationMode)
	if got.RepresentationMode() != wantMode {
		t.Fatalf("representation = %s, want %s", got.RepresentationMode(), wantMode)
	}
	if got.SparseCount() != vector.Expected.Body.SparseCount {
		t.Fatalf("sparse count = %d, want %d", got.SparseCount(), vector.Expected.Body.SparseCount)
	}
	if got.DenseNonZeroCount() != vector.Expected.Body.DenseNonZeroCount {
		t.Fatalf("dense non-zero count = %d, want %d", got.DenseNonZeroCount(), vector.Expected.Body.DenseNonZeroCount)
	}
	if len(vector.Expected.Body.SparseRegisters) != 0 {
		for _, want := range vector.Expected.Body.SparseRegisters {
			gotValue, ok := got.sparse.Get(want.Index)
			if !ok {
				t.Fatalf("missing sparse register %d", want.Index)
			}
			if gotValue != want.Value {
				t.Fatalf("sparse register %d = %d, want %d", want.Index, gotValue, want.Value)
			}
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

func assertSerializedHex(t *testing.T, sketch *Sketch, want string) {
	t.Helper()

	if want == "" {
		return
	}
	encoded, err := sketch.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary(): %v", err)
	}
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("serialized hex = %s, want %s", got, want)
	}
	decoded, err := hex.DecodeString(want)
	if err != nil {
		t.Fatalf("decode serialized hex: %v", err)
	}
	parsed, err := Parse(decoded)
	if err != nil {
		t.Fatalf("Parse(serialized_hex): %v", err)
	}
	reencoded, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary() after serialized_hex parse: %v", err)
	}
	if hex.EncodeToString(reencoded) != want {
		t.Fatal("serialized_hex parse/reencode changed bytes")
	}
}

func buildVectorSketch(t *testing.T, vector sketchVector) *Sketch {
	t.Helper()

	if vector.Metadata.Kind != "HLLPP" {
		t.Fatalf("unsupported vector kind %q", vector.Metadata.Kind)
	}
	if vector.Metadata.HashAlgo != string(sketchhash.HMACSHA25664) {
		t.Fatalf("unsupported hash algorithm %q", vector.Metadata.HashAlgo)
	}
	if vector.Metadata.HLLPPNormalPrecision != profileConfigs[Profile(vector.Metadata.Profile)].p ||
		vector.Metadata.HLLPPSparsePrecision != profileConfigs[Profile(vector.Metadata.Profile)].sp {
		t.Fatalf("vector p/sp does not match profile")
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
			bySource[source] = newTestSketch(t, Profile(vector.Metadata.Profile))
			sourceNames = append(sourceNames, source)
		}
		bySource[source].AddHash(parseHashHex(t, operation.HashHex))
	}

	if len(bySource) == 0 {
		return newTestSketch(t, Profile(vector.Metadata.Profile))
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

func representationMode(name string) sketchpb.RepresentationMode {
	switch name {
	case "HLLPP_SPARSE":
		return sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE
	case "HLLPP_DENSE":
		return sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_DENSE
	default:
		return sketchpb.RepresentationMode_REPRESENTATION_MODE_UNSPECIFIED
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

func newTestSketch(t *testing.T, profile Profile) *Sketch {
	t.Helper()

	sketch, err := New(profile, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatalf("New(%s): %v", profile, err)
	}

	return sketch
}

func malformedHLLPPMessage(profile Profile) *sketchpb.Sketch {
	config := profileConfigs[profile]
	normalPrecision := uint32(config.p)
	sparsePrecision := uint32(config.sp)
	return &sketchpb.Sketch{
		Metadata: &sketchpb.SketchMetadata{
			Kind:                 sketchpb.SketchKind_SKETCH_KIND_HLLPP,
			WireVersion:          wireVersion,
			Profile:              string(profile),
			HashDomain:           string(sketchhash.PromptV1),
			HashAlgo:             sketchpb.HashAlgorithm_HASH_ALGORITHM_HMAC_SHA256_64,
			RepresentationMode:   sketchpb.RepresentationMode_REPRESENTATION_MODE_HLLPP_SPARSE,
			HllppNormalPrecision: &normalPrecision,
			HllppSparsePrecision: &sparsePrecision,
		},
		Body: &sketchpb.Sketch_Hllpp{Hllpp: &sketchpb.HllppSketch{}},
	}
}

func marshalSketchMessage(t *testing.T, message *sketchpb.Sketch) []byte {
	t.Helper()

	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(message)
	if err != nil {
		t.Fatalf("marshal sketch message: %v", err)
	}
	return data
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
