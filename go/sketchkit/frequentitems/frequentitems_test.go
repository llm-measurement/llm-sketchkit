// Code authors: Vijay Erramilli and Codex
package frequentitems

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/rand"
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
		Kind                 string `json:"kind"`
		Profile              string `json:"profile"`
		HashDomain           string `json:"hash_domain"`
		HashAlgo             string `json:"hash_algo"`
		RepresentationMode   string `json:"representation_mode"`
		FrequentItemsMapSize int    `json:"frequent_items_map_size"`
	} `json:"metadata"`
	Operations []struct {
		Op      string `json:"op"`
		HashHex string `json:"hash_hex"`
		Weight  int64  `json:"weight"`
		Source  string `json:"source"`
	} `json:"operations"`
	Expected struct {
		SerializedHex string `json:"serialized_hex"`
		Body          struct {
			TotalWeight      int64    `json:"total_weight"`
			MaxError         int64    `json:"max_error"`
			NoFalseNegatives []string `json:"no_false_negatives"`
			NoFalsePositives []string `json:"no_false_positives"`
			Entries          []struct {
				HashHex    string `json:"hash_hex"`
				Estimate   int64  `json:"estimate"`
				Error      int64  `json:"error"`
				LowerBound int64  `json:"lower_bound"`
				UpperBound int64  `json:"upper_bound"`
			} `json:"entries"`
		} `json:"body"`
	} `json:"expected"`
}

func TestSketchVectors(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "vectors", "sketches", "frequent_items_*.json"))
	if err != nil {
		t.Fatalf("glob sketch vectors: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no frequent-items vectors found")
	}

	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			vector := readSketchVector(t, path)
			got := buildVectorSketch(t, vector)
			assertVectorExpected(t, got, vector)
			assertStableReserialization(t, got)
			assertSerializedHex(t, got, vector.Expected.SerializedHex)
		})
	}
}

func TestWeightedUpdatesAndQueryModes(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileMicro)
	mustAdd(t, sketch, 0x10, 5)
	mustAdd(t, sketch, 0x10, 7)
	mustAdd(t, sketch, 0x20, 3)

	if sketch.TotalWeight() != 15 {
		t.Fatalf("total weight = %d, want 15", sketch.TotalWeight())
	}
	if sketch.MaxError() != 0 {
		t.Fatalf("max error = %d, want 0", sketch.MaxError())
	}
	assertBounds(t, sketch, map[uint64]int64{0x10: 12, 0x20: 3})

	noFalseNegatives, err := sketch.FrequentItems(NoFalseNegatives)
	if err != nil {
		t.Fatalf("FrequentItems(NO_FN): %v", err)
	}
	noFalsePositives, err := sketch.FrequentItems(NoFalsePositives)
	if err != nil {
		t.Fatalf("FrequentItems(NO_FP): %v", err)
	}
	assertHashes(t, noFalseNegatives, []uint64{0x10, 0x20})
	assertHashes(t, noFalsePositives, []uint64{0x10, 0x20})
}

func TestRejectsNegativeWeight(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileMicro)
	if err := sketch.AddHash(0x10, -1); !errors.Is(err, ErrNegativeWeight) {
		t.Fatalf("AddHash() error = %v, want %v", err, ErrNegativeWeight)
	}
}

func TestParseRejectsNonProfileMapSize(t *testing.T) {
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
	nonProfileMapSize := uint32(511)
	message.GetMetadata().FrequentItemsMapSize = &nonProfileMapSize

	mutated, err := proto.MarshalOptions{Deterministic: true}.Marshal(&message)
	if err != nil {
		t.Fatalf("marshal mutated sketch: %v", err)
	}
	_, err = Parse(mutated)
	if !errors.Is(err, ErrInvalidMapSize) {
		t.Fatalf("Parse() error = %v, want %v", err, ErrInvalidMapSize)
	}
}

func TestSerializationRoundTripStable(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileMicro)
	for i := uint64(0); i < 300; i++ {
		mustAdd(t, sketch, splitmix64(0x9000+i), int64(i%7+1))
	}

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

func TestBoundsBracketTrueCountsRandom(t *testing.T) {
	t.Parallel()

	sketch := newTestSketch(t, ProfileMicro)
	trueCounts := make(map[uint64]int64)
	for i := uint64(0); i < 20000; i++ {
		hash := splitmix64(i) % 2000
		weight := int64(splitmix64(0xabc000+i)%7 + 1)
		mustAdd(t, sketch, hash, weight)
		trueCounts[hash] += weight
	}

	assertBounds(t, sketch, trueCounts)
	assertObservedUntrackedWithinMaxError(t, sketch, trueCounts)
}

func TestAdversarialPartitionMerge(t *testing.T) {
	t.Parallel()

	const partitions = 5
	parts := make([]*Sketch, partitions)
	trueCounts := make(map[uint64]int64)
	for i := range parts {
		parts[i] = newTestSketch(t, ProfileSmall)
	}

	topHashes := make([]uint64, 20)
	for rank := range topHashes {
		topHashes[rank] = splitmix64(0x4400 + uint64(rank))
	}
	for partition, sketch := range parts {
		for rank, hash := range topHashes {
			weight := int64(1000 + (20-rank)*10 + partition)
			mustAdd(t, sketch, hash, weight)
			trueCounts[hash] += weight
		}
		for i := 0; i < 493; i++ {
			hash := splitmix64(uint64(partition)<<32 + uint64(i))
			mustAdd(t, sketch, hash, 1)
			trueCounts[hash]++
		}
	}

	sumPartErrors := int64(0)
	merged := parts[0].Clone()
	sumPartErrors += parts[0].MaxError()
	for _, part := range parts[1:] {
		sumPartErrors += part.MaxError()
		if err := merged.Merge(part); err != nil {
			t.Fatalf("Merge(): %v", err)
		}
	}
	if merged.MaxError() > sumPartErrors {
		t.Fatalf("merged max error = %d, want <= sum part errors %d", merged.MaxError(), sumPartErrors)
	}
	t.Logf("adversarial merge partitions=%d sum_part_max_error=%d merged_max_error=%d bound_width=%d",
		partitions, sumPartErrors, merged.MaxError(), merged.MaxError())
	assertBounds(t, merged, trueCounts)
	assertObservedUntrackedWithinMaxError(t, merged, trueCounts)

	noFalseNegatives := mustFrequentItems(t, merged, NoFalseNegatives)
	noFalseNegativeSet := itemSet(noFalseNegatives)
	for _, hash := range topHashes {
		if !noFalseNegativeSet[hash] {
			t.Fatalf("NO_FN missing top hash %016x", hash)
		}
	}
	assertNoFalseNegativesAboveThreshold(t, noFalseNegatives, trueCounts, merged.MaxError())

	noFalsePositives := mustFrequentItems(t, merged, NoFalsePositives)
	assertNoFalsePositivesAboveThreshold(t, noFalsePositives, trueCounts, merged.MaxError())
	t.Logf("adversarial query no_fn_items=%d no_fp_items=%d true_top_items=%d", len(noFalseNegatives), len(noFalsePositives), len(topHashes))
}

func TestZipfTop20(t *testing.T) {
	t.Parallel()

	const (
		updates = 1_000_000
		keys    = 100_000
	)
	sketch := newTestSketch(t, ProfileSmall)
	trueCounts := make([]int64, keys)
	zipf := rand.NewZipf(rand.New(rand.NewSource(0x5eed)), 1.1, 1, keys-1)
	for i := 0; i < updates; i++ {
		hash := uint64(zipf.Uint64())
		mustAdd(t, sketch, hash, 1)
		trueCounts[hash]++
	}

	assertBoundsSlice(t, sketch, trueCounts)
	assertUntrackedCountsWithinMaxErrorSlice(t, sketch, trueCounts)

	top20 := topHashes(trueCounts, 20)
	minTop20 := trueCounts[top20[len(top20)-1]]
	noFalseNegatives := mustFrequentItems(t, sketch, NoFalseNegatives)
	noFalseNegativeSet := itemSet(noFalseNegatives)
	for _, hash := range top20 {
		if !noFalseNegativeSet[hash] {
			t.Fatalf("NO_FN missing true top-20 hash %016x", hash)
		}
	}
	assertNoFalseNegativesAboveThresholdSlice(t, noFalseNegatives, trueCounts, sketch.MaxError())

	noFalsePositives := mustFrequentItems(t, sketch, NoFalsePositives)
	assertNoFalsePositivesAboveThresholdSlice(t, noFalsePositives, trueCounts, sketch.MaxError())

	worstCaseError := float64(updates) / float64(sketch.MapSize()+1)
	t.Logf("zipf updates=%d keys=%d max_error=%d worst_case_error=%.2f error_ratio=%.2f no_fn_items=%d no_fp_items=%d min_top20_count=%d",
		updates, keys, sketch.MaxError(), worstCaseError, float64(sketch.MaxError())/worstCaseError,
		len(noFalseNegatives), len(noFalsePositives), minTop20)
}

func TestZipfPartitionMergeOrderGuarantees(t *testing.T) {
	t.Parallel()

	const (
		updates    = 1_000_000
		keys       = 100_000
		partitions = 5
	)
	parts := make([]*Sketch, partitions)
	for i := range parts {
		parts[i] = newTestSketch(t, ProfileSmall)
	}
	trueCounts := make([]int64, keys)
	zipf := rand.NewZipf(rand.New(rand.NewSource(0x5eed)), 1.1, 1, keys-1)
	for i := 0; i < updates; i++ {
		hash := uint64(zipf.Uint64())
		partition := int(hash % partitions)
		mustAdd(t, parts[partition], hash, 1)
		trueCounts[hash]++
	}

	sumPartErrors := int64(0)
	var minPartError int64
	var maxPartError int64
	for i, part := range parts {
		partError := part.MaxError()
		if partError < 100 {
			t.Fatalf("part[%d] max error = %d, want heavily pruned partition", i, partError)
		}
		sumPartErrors += partError
		if i == 0 || partError < minPartError {
			minPartError = partError
		}
		if partError > maxPartError {
			maxPartError = partError
		}
	}

	top20 := topHashes(trueCounts, 20)
	minTop20 := trueCounts[top20[len(top20)-1]]
	orders := permutations(partitions)
	uniqueEncodings := map[string]struct{}{}
	globalWorstError := int64((updates + parts[0].MapSize()) / (parts[0].MapSize() + 1))
	var minMergedError int64
	var maxMergedError int64
	var maxSumErrorOverrun int64
	var minNoFalseNegatives int
	var maxNoFalseNegatives int
	var minNoFalsePositives int
	var maxNoFalsePositives int
	for orderIndex, order := range orders {
		merged := mergeFrequentItemsInOrder(t, parts, order)
		mergedError := merged.MaxError()
		if mergedError > globalWorstError {
			t.Fatalf("order %v merged max error = %d, want <= global MG worst-case error %d",
				order, mergedError, globalWorstError)
		}
		maxSumErrorOverrun = maxInt64(maxSumErrorOverrun, mergedError-sumPartErrors)
		assertBoundsSlice(t, merged, trueCounts)
		assertUntrackedCountsWithinMaxErrorSlice(t, merged, trueCounts)

		noFalseNegatives := mustFrequentItems(t, merged, NoFalseNegatives)
		noFalseNegativeSet := itemSet(noFalseNegatives)
		for _, hash := range top20 {
			if !noFalseNegativeSet[hash] {
				t.Fatalf("order %v NO_FN missing true top-20 hash %016x", order, hash)
			}
		}
		assertNoFalseNegativesAboveThresholdSlice(t, noFalseNegatives, trueCounts, mergedError)

		noFalsePositives := mustFrequentItems(t, merged, NoFalsePositives)
		assertNoFalsePositivesAboveThresholdSlice(t, noFalsePositives, trueCounts, mergedError)

		encoded, err := merged.MarshalBinary()
		if err != nil {
			t.Fatalf("order %v MarshalBinary(): %v", order, err)
		}
		uniqueEncodings[hex.EncodeToString(encoded)] = struct{}{}

		if orderIndex == 0 || mergedError < minMergedError {
			minMergedError = mergedError
		}
		if mergedError > maxMergedError {
			maxMergedError = mergedError
		}
		if orderIndex == 0 || len(noFalseNegatives) < minNoFalseNegatives {
			minNoFalseNegatives = len(noFalseNegatives)
		}
		if len(noFalseNegatives) > maxNoFalseNegatives {
			maxNoFalseNegatives = len(noFalseNegatives)
		}
		if orderIndex == 0 || len(noFalsePositives) < minNoFalsePositives {
			minNoFalsePositives = len(noFalsePositives)
		}
		if len(noFalsePositives) > maxNoFalsePositives {
			maxNoFalsePositives = len(noFalsePositives)
		}
	}

	t.Logf(
		"zipf partition merge updates=%d keys=%d partitions=%d orders=%d part_max_error_min=%d part_max_error_max=%d sum_part_max_error=%d merged_max_error_min=%d merged_max_error_max=%d no_fn_items_min=%d no_fn_items_max=%d no_fp_items_min=%d no_fp_items_max=%d unique_serialized_states=%d min_top20_count=%d",
		updates, keys, partitions, len(orders), minPartError, maxPartError, sumPartErrors,
		minMergedError, maxMergedError, minNoFalseNegatives, maxNoFalseNegatives,
		minNoFalsePositives, maxNoFalsePositives, len(uniqueEncodings), minTop20,
	)
	t.Logf("zipf partition merge global_worst_error=%d max_sum_error_overrun=%d", globalWorstError, maxSumErrorOverrun)
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

	if vector.Metadata.Kind != "FREQUENT_ITEMS" {
		t.Fatalf("unsupported vector kind %q", vector.Metadata.Kind)
	}
	if vector.Metadata.HashAlgo != string(sketchhash.HMACSHA25664) {
		t.Fatalf("unsupported hash algorithm %q", vector.Metadata.HashAlgo)
	}
	profile := Profile(vector.Metadata.Profile)
	if vector.Metadata.FrequentItemsMapSize != profileMapSizes[profile] {
		t.Fatalf("vector map size = %d, want %d", vector.Metadata.FrequentItemsMapSize, profileMapSizes[profile])
	}

	bySource := map[string]*Sketch{}
	sourceNames := make([]string, 0)
	for _, operation := range vector.Operations {
		if operation.Op != "add_hash_weight" {
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
		mustAdd(t, bySource[source], parseHashHex(t, operation.HashHex), operation.Weight)
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

	if got.TotalWeight() != vector.Expected.Body.TotalWeight {
		t.Fatalf("total weight = %d, want %d", got.TotalWeight(), vector.Expected.Body.TotalWeight)
	}
	if got.MaxError() != vector.Expected.Body.MaxError {
		t.Fatalf("max error = %d, want %d", got.MaxError(), vector.Expected.Body.MaxError)
	}

	items := got.itemsSortedByHash()
	if len(items) != len(vector.Expected.Body.Entries) {
		t.Fatalf("entries length = %d, want %d", len(items), len(vector.Expected.Body.Entries))
	}
	for i, want := range vector.Expected.Body.Entries {
		wantHash := parseHashHex(t, want.HashHex)
		if items[i].Hash != wantHash ||
			items[i].Estimate != want.Estimate ||
			items[i].Error != want.Error ||
			items[i].LowerBound != want.LowerBound ||
			items[i].UpperBound != want.UpperBound {
			t.Fatalf("entry[%d] = %+v, want hash=%016x estimate=%d error=%d lower=%d upper=%d",
				i, items[i], wantHash, want.Estimate, want.Error, want.LowerBound, want.UpperBound)
		}
	}
	if vector.Expected.Body.NoFalseNegatives != nil {
		noFalseNegatives, err := got.FrequentItems(NoFalseNegatives)
		if err != nil {
			t.Fatalf("FrequentItems(NO_FN): %v", err)
		}
		assertHexHashes(t, noFalseNegatives, vector.Expected.Body.NoFalseNegatives)
	}
	if vector.Expected.Body.NoFalsePositives != nil {
		noFalsePositives, err := got.FrequentItems(NoFalsePositives)
		if err != nil {
			t.Fatalf("FrequentItems(NO_FP): %v", err)
		}
		assertHexHashes(t, noFalsePositives, vector.Expected.Body.NoFalsePositives)
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

func assertBounds(t *testing.T, sketch *Sketch, trueCounts map[uint64]int64) {
	t.Helper()

	for hash, trueCount := range trueCounts {
		lower := sketch.LowerBoundHash(hash)
		upper := sketch.UpperBoundHash(hash)
		if lower > trueCount || upper < trueCount {
			t.Fatalf("hash=%016x lower=%d true=%d upper=%d max_error=%d",
				hash, lower, trueCount, upper, sketch.MaxError())
		}
	}
}

func assertBoundsSlice(t *testing.T, sketch *Sketch, trueCounts []int64) {
	t.Helper()

	for hash, trueCount := range trueCounts {
		if trueCount == 0 {
			continue
		}
		lower := sketch.LowerBoundHash(uint64(hash))
		upper := sketch.UpperBoundHash(uint64(hash))
		if lower > trueCount || upper < trueCount {
			t.Fatalf("hash=%016x lower=%d true=%d upper=%d max_error=%d",
				hash, lower, trueCount, upper, sketch.MaxError())
		}
	}
}

func assertObservedUntrackedWithinMaxError(t *testing.T, sketch *Sketch, trueCounts map[uint64]int64) {
	t.Helper()

	untracked := 0
	for hash, trueCount := range trueCounts {
		if _, ok := sketch.items[hash]; ok {
			continue
		}
		untracked++
		if trueCount > sketch.MaxError() {
			t.Fatalf("untracked hash=%016x true=%d max_error=%d", hash, trueCount, sketch.MaxError())
		}
	}
	if untracked == 0 {
		t.Fatal("expected at least one observed untracked hash")
	}
}

func assertUntrackedCountsWithinMaxErrorSlice(t *testing.T, sketch *Sketch, trueCounts []int64) {
	t.Helper()

	untracked := 0
	for hash, trueCount := range trueCounts {
		if trueCount == 0 {
			continue
		}
		if _, ok := sketch.items[uint64(hash)]; ok {
			continue
		}
		untracked++
		if trueCount > sketch.MaxError() {
			t.Fatalf("untracked hash=%016x true=%d max_error=%d", hash, trueCount, sketch.MaxError())
		}
	}
	if untracked == 0 {
		t.Fatal("expected at least one observed untracked hash")
	}
}

func assertNoFalseNegativesAboveThreshold(
	t *testing.T,
	items []Item,
	trueCounts map[uint64]int64,
	threshold int64,
) {
	t.Helper()

	returned := itemSet(items)
	for hash, trueCount := range trueCounts {
		if trueCount > threshold && !returned[hash] {
			t.Fatalf("NO_FN missing hash=%016x true=%d threshold=%d", hash, trueCount, threshold)
		}
	}
}

func assertNoFalseNegativesAboveThresholdSlice(
	t *testing.T,
	items []Item,
	trueCounts []int64,
	threshold int64,
) {
	t.Helper()

	returned := itemSet(items)
	for hash, trueCount := range trueCounts {
		if trueCount > threshold && !returned[uint64(hash)] {
			t.Fatalf("NO_FN missing hash=%016x true=%d threshold=%d", hash, trueCount, threshold)
		}
	}
}

func assertNoFalsePositivesAboveThreshold(
	t *testing.T,
	items []Item,
	trueCounts map[uint64]int64,
	threshold int64,
) {
	t.Helper()

	for _, item := range items {
		if trueCounts[item.Hash] <= threshold {
			t.Fatalf("NO_FP returned hash %016x with true count %d <= threshold %d",
				item.Hash, trueCounts[item.Hash], threshold)
		}
	}
}

func assertNoFalsePositivesAboveThresholdSlice(
	t *testing.T,
	items []Item,
	trueCounts []int64,
	threshold int64,
) {
	t.Helper()

	for _, item := range items {
		if trueCounts[item.Hash] <= threshold {
			t.Fatalf("NO_FP returned hash %016x with true count %d <= threshold %d",
				item.Hash, trueCounts[item.Hash], threshold)
		}
	}
}

func mustFrequentItems(t *testing.T, sketch *Sketch, mode QueryMode) []Item {
	t.Helper()

	items, err := sketch.FrequentItems(mode)
	if err != nil {
		t.Fatalf("FrequentItems(%d): %v", mode, err)
	}

	return items
}

func assertHexHashes(t *testing.T, items []Item, hashes []string) {
	t.Helper()

	parsed := make([]uint64, 0, len(hashes))
	for _, hash := range hashes {
		parsed = append(parsed, parseHashHex(t, hash))
	}
	assertHashes(t, items, parsed)
}

func assertHashes(t *testing.T, items []Item, hashes []uint64) {
	t.Helper()

	if len(items) != len(hashes) {
		t.Fatalf("items length = %d, want %d", len(items), len(hashes))
	}
	for i, hash := range hashes {
		if items[i].Hash != hash {
			t.Fatalf("items[%d].Hash = %016x, want %016x", i, items[i].Hash, hash)
		}
	}
}

func itemSet(items []Item) map[uint64]bool {
	out := make(map[uint64]bool, len(items))
	for _, item := range items {
		out[item.Hash] = true
	}

	return out
}

func topHashes(counts []int64, limit int) []uint64 {
	indexes := make([]int, len(counts))
	for i := range indexes {
		indexes[i] = i
	}
	sort.Slice(indexes, func(i int, j int) bool {
		if counts[indexes[i]] != counts[indexes[j]] {
			return counts[indexes[i]] > counts[indexes[j]]
		}
		return indexes[i] < indexes[j]
	})

	out := make([]uint64, 0, limit)
	for _, index := range indexes[:limit] {
		out = append(out, uint64(index))
	}

	return out
}

func newTestSketch(t *testing.T, profile Profile) *Sketch {
	t.Helper()

	sketch, err := New(profile, sketchhash.PromptV1, sketchhash.HMACSHA25664)
	if err != nil {
		t.Fatalf("New(%s): %v", profile, err)
	}

	return sketch
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

func mustAdd(t *testing.T, sketch *Sketch, hash uint64, weight int64) {
	t.Helper()

	if err := sketch.AddHash(hash, weight); err != nil {
		t.Fatalf("AddHash(%016x, %d): %v", hash, weight, err)
	}
}

func mergeFrequentItemsInOrder(t *testing.T, parts []*Sketch, order []int) *Sketch {
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

func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
